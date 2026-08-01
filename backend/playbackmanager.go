package backend

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/charlievieth/strcase"
	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/backend/player"
	"github.com/dweymouth/supersonic/backend/player/dlna"
	"github.com/dweymouth/supersonic/backend/player/mpv"
	"github.com/dweymouth/supersonic/sharedutil"
	"github.com/supersonic-app/go-upnpcast/device"
	"github.com/supersonic-app/go-upnpcast/services"
)

// A high-level MediaProvider-aware playback engine, serves as an
// intermediary between the frontend and various Player backends.
type PlaybackManager struct {
	engine   *playbackEngine
	wfmGen   *WaveformImageGenerator
	cache    *AudioCache
	cmdQueue *playbackCommandQueue
	appCfg   *AppConfig
	cfg      *PlaybackConfig

	// CoverArtPathFn returns a local filesystem path to the cached cover
	// art image for the given CoverArtID. Used by DLNA cast so the
	// renderer can fetch album art via the local proxy. Set externally
	// (typically wired to ImageManager.GetCoverArtPath).
	CoverArtPathFn func(coverArtID string) (string, error)

	localPlayer         player.BasePlayer
	remotePlayersLock   sync.Mutex
	remotePlayers       []RemotePlaybackDevice
	currentRemotePlayer *RemotePlaybackDevice

	onWaveformImgUpdate []func(*WaveformImage)
	onPlayerChange      []func()

	lastPlayTime         float64
	lastPlayingID        string
	wfmUpdateImageCancel context.CancelFunc
	wfmImageJobs         [3]*WaveformImageJob

	// whether autoplay tracks are currently being fetched/enqueued
	pendingAutoplay    bool
	wasLoadTrackPaused bool

	// current radio metadata
	radioStationName string
	radioIcyTitle    string
	radioIcyArtist   string
}

type RemotePlaybackDevice struct {
	Name     string
	URL      string
	Protocol string
	new      func() (player.BasePlayer, error)
}

func NewPlaybackManager(
	ctx context.Context,
	s *ServerManager,
	c *AudioCache,
	p player.BasePlayer,
	playbackCfg *PlaybackConfig,
	scrobbleCfg *ScrobbleConfig,
	transcodeCfg *TranscodingConfig,
	appCfg *AppConfig,
) *PlaybackManager {
	e := NewPlaybackEngine(ctx, s, c, p, playbackCfg, scrobbleCfg, transcodeCfg)
	q := NewCommandQueue()
	pm := &PlaybackManager{
		engine:      e,
		cmdQueue:    q,
		appCfg:      appCfg,
		cfg:         playbackCfg,
		localPlayer: p,
		cache:       c,
	}
	if c != nil {
		pm.wfmGen = NewWaveformImageGenerator(c)
	}
	pm.addOnTrackChangeHook()
	go pm.runCmdQueue(ctx)
	return pm
}

func (p *PlaybackManager) findWfmImageJob(id string, uncanceledOnly bool) (*WaveformImageJob, bool) {
	for _, j := range p.wfmImageJobs {
		if j != nil && j.ItemID == id && (!uncanceledOnly || !j.Canceled()) {
			return j, true
		}
	}
	return nil, false
}

func (p *PlaybackManager) addWfmImageJob(job *WaveformImageJob) {
	p.wfmImageJobs[0].Cancel()
	p.wfmImageJobs[0] = p.wfmImageJobs[1]
	p.wfmImageJobs[1] = p.wfmImageJobs[2]
	p.wfmImageJobs[2] = job
}

func (p *PlaybackManager) addOnTrackChangeHook() {
	// See https://github.com/dweymouth/supersonic/issues/483
	// On Windows, MPV sometimes fails to start playback when switching to a track
	// with a different sample rate than the previous. If this is detected,
	// send a command to the MPV player to force restart playback.
	p.OnPlayTimeUpdate(func(curTime, totalTime float64, _ bool) {
		p.lastPlayTime = curTime

		// enqueue autoplay tracks if enabled and nearing end of queue
		if p.cfg.Autoplay && !p.pendingAutoplay && totalTime-curTime < 10.0 &&
			p.NowPlayingIndex() == p.engine.getPlayQueueLength()-1 {
			p.enqueueAutoplayTracks()
		}
	})

	p.engine.onBeforeSongChange = append(p.engine.onBeforeSongChange, func(item mediaprovider.MediaItem) {
		if item == nil || !p.engine.playbackCfg.UseWaveformSeekbar {
			return
		}
		if p.wfmGen != nil && item.Metadata().Type == mediaprovider.MediaItemTypeTrack {
			if _, ok := p.findWfmImageJob(item.Metadata().ID, true); !ok {
				// start generating waveform image for next-up track
				p.addWfmImageJob(p.wfmGen.StartWaveformGeneration(item.(*mediaprovider.Track)))
			}
		}
		if p.isLoadTrackPaused() {
			// we need to call handleWaveformImageSongChange to ensure the waveform image is updated
			// for the track that is loaded paused when starting the app
			p.handleWaveformImageSongChange(item)
			p.wasLoadTrackPaused = true
		}
	})

	p.OnSongChange(func(item mediaprovider.MediaItem, _ *mediaprovider.Track) {
		if p.wasLoadTrackPaused {
			// if the song change was triggered by LoadTrackPaused when starting the app,
			// we already called handleWaveformImageSongChange in the onBeforeSongChange hook above
			p.wasLoadTrackPaused = false
		} else {
			p.handleWaveformImageSongChange(item)
		}

		if runtime.GOOS != "windows" {
			return
		}
		// workaround for https://github.com/dweymouth/supersonic/issues/483 (see above comment)
		if p.NowPlayingIndex() != p.engine.getPlayQueueLength() && p.PlaybackStatus().State == player.Playing {
			p.lastPlayTime = 0
			go func() {
				time.Sleep(300 * time.Millisecond)
				if p.lastPlayTime == 0 {
					log.Println("Play stall detected!")
					p.cmdQueue.addCommand(playbackCommand{Type: cmdForceRestartPlayback})
				}
			}()
		}
	})
	p.OnRadioMetadataChange(func(radioName, title, artist string) {
		p.radioStationName = radioName
		p.radioIcyTitle = title
		p.radioIcyArtist = artist
	})
}

func (p *PlaybackManager) handleWaveformImageSongChange(item mediaprovider.MediaItem) {
	if p.wfmUpdateImageCancel != nil {
		p.wfmUpdateImageCancel()
	}
	if !p.engine.playbackCfg.UseWaveformSeekbar {
		return
	}

	updateUnfinishedJob := func(job *WaveformImageJob) {
		ctx, c := context.WithCancel(p.cache.rootCtx)
		p.wfmUpdateImageCancel = c
		go func(ctx context.Context, job *WaveformImageJob) {
			for {
				time.Sleep(333 * time.Millisecond)
				select {
				case <-ctx.Done():
					return
				default:
					img := job.Get()
					for _, cb := range p.onWaveformImgUpdate {
						cb(img)
					}
					if job.Done() {
						return
					}
				}
			}
		}(ctx, job)
	}

	if item != nil {
		// cancel possible waveform generation job for previous track
		if p.lastPlayingID != item.Metadata().ID {
			if old, ok := p.findWfmImageJob(p.lastPlayingID, false); ok {
				old.Cancel()
			}
			p.lastPlayingID = item.Metadata().ID
		}

		var job *WaveformImageJob
		if j, ok := p.findWfmImageJob(item.Metadata().ID, true); ok {
			job = j
		} else if tr, ok := item.(*mediaprovider.Track); ok {
			job = p.wfmGen.StartWaveformGeneration(tr)
			p.addWfmImageJob(job)
		}
		if job != nil {
			img := job.Get()
			for _, cb := range p.onWaveformImgUpdate {
				cb(img)
			}
			if !job.done && job != nil {
				updateUnfinishedJob(job)
			}
		}
	}

	if item == nil || item.Metadata().Type != mediaprovider.MediaItemTypeTrack {
		// set a zero waveform image when we're not playing anything
		// or playing a media type we can't derive a waveform from (e.g. radio)
		img := NewWaveformImage()
		for _, cb := range p.onWaveformImgUpdate {
			cb(img)
		}
	}
}

func (p *PlaybackManager) ScanRemotePlayers(ctx context.Context, fastScan bool) {
	if fastScan {
		p.scanRemotePlayers(ctx, 1 /*waitSec*/)
		// continue to slow scan to detect players that take longer to respond
	}
	p.scanRemotePlayers(ctx, 10 /*waitSec*/)
}

func (p *PlaybackManager) scanRemotePlayers(ctx context.Context, waitSec int) {
	devices, _ := device.SearchMediaRenderers(ctx, waitSec, services.AVTransport, services.RenderingControl)

	coverArtPathFn := p.CoverArtPathFn
	var discovered []RemotePlaybackDevice
	for _, d := range devices {
		rp := RemotePlaybackDevice{
			Name:     d.FriendlyName,
			URL:      d.URL,
			Protocol: "DLNA",
			new: func() (player.BasePlayer, error) {
				return dlna.NewDLNAPlayer(d, coverArtPathFn)
			},
		}
		discovered = append(discovered, rp)
	}

	p.remotePlayersLock.Lock()
	p.remotePlayers = discovered
	p.remotePlayersLock.Unlock()
}

func (p *PlaybackManager) RemotePlayers() []RemotePlaybackDevice {
	p.remotePlayersLock.Lock()
	players := p.remotePlayers
	p.remotePlayersLock.Unlock()
	return players
}

func (p *PlaybackManager) CurrentRemotePlayer() *RemotePlaybackDevice {
	return p.currentRemotePlayer
}

func (p *PlaybackManager) SetRemotePlayer(rp *RemotePlaybackDevice) error {
	// Even in case of failure, call onPlayerChange callbacks to update UI
	// (such as enabling/disabling cast button)
	defer func() {
		for _, cb := range p.onPlayerChange {
			cb()
		}
	}()

	p.cmdQueue.Clear()
	if rp == nil {
		if err := p.engine.SetPlayer(p.localPlayer); err != nil {
			return err
		}
		p.currentRemotePlayer = nil
		return nil
	}

	player, err := rp.new()
	if err != nil {
		return err
	}
	if err := p.engine.SetPlayer(player); err != nil {
		return err
	}

	p.currentRemotePlayer = rp
	return nil
}

func (p *PlaybackManager) CurrentPlayer() player.BasePlayer {
	return p.engine.CurrentPlayer()
}

func (p *PlaybackManager) OnPlayerChange(cb func()) {
	p.onPlayerChange = append(p.onPlayerChange, cb)
}

func (p *PlaybackManager) OnWaveformImgUpdate(cb func(*WaveformImage)) {
	p.onWaveformImgUpdate = append(p.onWaveformImgUpdate, cb)
}

func (p *PlaybackManager) IsSeeking() bool {
	return p.engine.IsSeeking()
}

// Should only be called before quitting.
// Disables playback state callbacks being sent
func (p *PlaybackManager) DisableCallbacks() {
	p.engine.callbacksDisabled = true
}

// Gets the now playing media item, if any.
func (p *PlaybackManager) NowPlaying() mediaprovider.MediaItem {
	item := p.engine.NowPlaying()
	if station, ok := item.(*mediaprovider.RadioStation); ok {
		station = station.Copy().(*mediaprovider.RadioStation)
		station.StationName = p.radioStationName
		station.Title = p.radioIcyTitle
		station.Artists = []string{p.radioIcyArtist}
		item = station
	}
	return item
}

func (p *PlaybackManager) NowPlayingIndex() int {
	return p.engine.NowPlayingIndex()
}

// Sets a callback that is notified whenever a new song begins playing.
func (p *PlaybackManager) OnSongChange(cb func(nowPlaying mediaprovider.MediaItem, justScrobbledIfAny *mediaprovider.Track)) {
	p.engine.onSongChange = append(p.engine.onSongChange, cb)
}

// Sets a callback that is notified whenever the Icy radio metadata changes.
func (p *PlaybackManager) OnRadioMetadataChange(cb func(radioName, title, artist string)) {
	p.engine.onRadioMetadataChange = append(p.engine.onRadioMetadataChange, cb)
}

// Registers a callback that is notified whenever the play time should be updated.
func (p *PlaybackManager) OnPlayTimeUpdate(cb func(curTime float64, totalTime float64, seeked bool)) {
	p.engine.onPlayTimeUpdate = append(p.engine.onPlayTimeUpdate, cb)
}

// Registers a callback that is notified whenever the loop mode changes.
func (p *PlaybackManager) OnLoopModeChange(cb func(LoopMode)) {
	p.engine.onLoopModeChange = append(p.engine.onLoopModeChange, cb)
}

// Registers a callback that is notified whenever the shuffle state changes.
func (p *PlaybackManager) OnShuffleChange(cb func(bool)) {
	p.engine.onShuffleChange = append(p.engine.onShuffleChange, cb)
}

// Registers a callback that is notified whenever the volume changes.
func (p *PlaybackManager) OnVolumeChange(cb func(int)) {
	p.engine.onVolumeChange = append(p.engine.onVolumeChange, cb)
}

// Registers a callback that is notified whenever the play queue changes.
func (p *PlaybackManager) OnQueueChange(cb func()) {
	p.engine.onQueueChange = append(p.engine.onQueueChange, cb)
}

// TriggerQueueChangeCallback notifies all queue change listeners.
func (p *PlaybackManager) TriggerQueueChangeCallback() {
	for _, cb := range p.engine.onQueueChange {
		cb()
	}
}

// Registers a callback that is notified whenever the player has been seeked.
func (p *PlaybackManager) OnSeek(cb func()) {
	p.engine.onSeek = append(p.engine.onSeek, cb)
}

// Registers a callback that is notified whenever the player has been paused.
func (p *PlaybackManager) OnPaused(cb func()) {
	p.engine.onPaused = append(p.engine.onPaused, cb)
}

// Registers a callback that is notified whenever the player is stopped.
func (p *PlaybackManager) OnStopped(cb func()) {
	p.engine.onStopped = append(p.engine.onStopped, cb)
}

// Registers a callback that is notified whenever the player begins playing.
func (p *PlaybackManager) OnPlaying(cb func()) {
	p.engine.onPlaying = append(p.engine.onPlaying, cb)
}

// Loads the specified album into the play queue.
func (p *PlaybackManager) LoadAlbum(albumID string, insertQueueMode InsertQueueMode, shuffle bool) error {
	album, err := p.engine.sm.Server.GetAlbum(albumID)
	if err != nil {
		return err
	}
	p.LoadTracks(album.Tracks, insertQueueMode, shuffle)
	return nil
}

func (p *PlaybackManager) IsShuffle() bool {
	return p.cfg.Shuffle
}

// Loads the specified playlist into the play queue.
func (p *PlaybackManager) LoadPlaylist(playlistID string, insertQueueMode InsertQueueMode, shuffle bool) error {
	playlist, err := p.engine.sm.Server.GetPlaylist(playlistID)
	if err != nil {
		return err
	}
	p.LoadTracks(playlist.Tracks, insertQueueMode, shuffle)
	return nil
}

// Load tracks into the play queue.
// If replacing the current queue (!appendToQueue), playback will be stopped.
func (p *PlaybackManager) LoadTracks(tracks []*mediaprovider.Track, insertQueueMode InsertQueueMode, shuffle bool) {
	items := sharedutil.CopyTrackSliceToMediaItemSlice(tracks)
	p.cmdQueue.LoadItems(items, insertQueueMode, shuffle)
}

// Replaces the playQueue with tracks and moves the track at idx to position 0
func (p *PlaybackManager) LoadTracksAndPlayAtIdx(tracks []*mediaprovider.Track, shuffle bool, idx int) {
	items := sharedutil.CopyTrackSliceToMediaItemSlice(tracks)
	p.cmdQueue.LoadItemsAndPlayAtIdx(items, shuffle, idx)
}

// Load items into the currently active queue. (shuffledPlayQueue/playQueue)
// If replacing the current queue (!appendToQueue), playback will be stopped.
// Loading items into the shuffledPlayQueue may also modify the playQueue
func (p *PlaybackManager) LoadItems(items []mediaprovider.MediaItem, insertQueueMode InsertQueueMode, shuffle bool) {
	p.cmdQueue.LoadItems(items, insertQueueMode, shuffle)
}

// Replaces the specified queue (PlayQueue/ShuffledPlayQueue) with the given tracks.
// This is used when starting supersonic to load the queue state and directly overrides any previous data.
// For replacing the queue while supersonic is running, use LoadTracks
func (p *PlaybackManager) SetQueueState(tracks []*mediaprovider.Track, queueType QueueType) {
	p.cmdQueue.SetQueueState(tracks, queueType)
}

// Replaces the play queue with the given set of tracks.
// Does not stop playback if the currently playing track is in the new queue,
// but updates the now playing index to point to the first instance of the track in the new queue.
func (p *PlaybackManager) UpdatePlayQueue(items []mediaprovider.MediaItem) {
	p.cmdQueue.UpdatePlayQueue(items)
}

func (p *PlaybackManager) PlayAlbum(albumID string, firstTrack int, shuffle bool) error {
	if err := p.LoadAlbum(albumID, Replace, shuffle); err != nil {
		return err
	}
	if p.engine.replayGainCfg.Mode == ReplayGainAuto {
		p.SetReplayGainMode(player.ReplayGainAlbum)
	}
	p.PlayTrackAt(firstTrack)
	return nil
}

func (p *PlaybackManager) PlayPlaylist(playlistID string, firstTrack int, shuffle bool) error {
	if err := p.LoadPlaylist(playlistID, Replace, shuffle); err != nil {
		return err
	}
	if p.engine.replayGainCfg.Mode == ReplayGainAuto {
		p.SetReplayGainMode(player.ReplayGainTrack)
	}
	p.PlayTrackAt(firstTrack)
	return nil
}

func (p *PlaybackManager) PlayTrack(trackID string) error {
	tr, err := p.engine.sm.Server.GetTrack(trackID)
	if err != nil {
		return err
	}
	p.LoadTracks([]*mediaprovider.Track{tr}, Replace, false)
	if p.engine.replayGainCfg.Mode == ReplayGainAuto {
		p.SetReplayGainMode(player.ReplayGainTrack)
	}
	p.PlayFromBeginning()
	return nil
}

func (p *PlaybackManager) ShuffleArtistAlbums(artistID string) {
	artist, err := p.engine.sm.Server.GetArtist(artistID)
	if err != nil {
		log.Printf("failed to get artist: %v\n", err)
		return
	}
	if len(artist.Albums) == 0 {
		return
	}

	rand.Shuffle(len(artist.Albums), func(i, j int) {
		artist.Albums[i], artist.Albums[j] = artist.Albums[j], artist.Albums[i]
	})
	p.StopAndClearPlayQueue()
	for _, al := range artist.Albums {
		p.LoadAlbum(al.ID, Append, false)
	}

	if p.engine.replayGainCfg.Mode == ReplayGainAuto {
		p.SetReplayGainMode(player.ReplayGainAlbum)
	}
	p.PlayFromBeginning()
}

func (p *PlaybackManager) PlayArtistDiscography(artistID string, shuffleTracks bool) {
	tr, err := p.engine.sm.Server.GetArtistTracks(artistID)
	if err != nil {
		log.Printf("failed to get artist tracks: %v\n", err)
		return
	}
	p.LoadTracks(tr, Replace, shuffleTracks)
	if p.engine.replayGainCfg.Mode == ReplayGainAuto {
		if shuffleTracks {
			p.SetReplayGainMode(player.ReplayGainTrack)
		} else {
			p.SetReplayGainMode(player.ReplayGainAlbum)
		}
	}
	p.PlayFromBeginning()
}

func (p *PlaybackManager) PlayFromBeginning() {
	p.cmdQueue.PlayTrackAt(0)
}

func (p *PlaybackManager) PlayTrackAt(idx int) {
	p.cmdQueue.PlayTrackAt(idx)
}

// LoadTrackPaused sets up engine state as if the track at idx is loaded and
// paused at startTime, updating the UI and OS media integrations, without
// starting MPV. Call Continue (or PlayPause) to begin actual playback.
func (p *PlaybackManager) LoadTrackPaused(idx int, startTime float64) {
	p.cmdQueue.LoadTrackPaused(idx, startTime)
}

func (p *PlaybackManager) isLoadTrackPaused() bool {
	return p.engine.pendingLoadPaused
}

func (p *PlaybackManager) PlayRandomSongs(genreName string) error {
	return p.fetchAndPlayTracks(func() ([]*mediaprovider.Track, error) {
		tr, err := p.engine.sm.Server.GetRandomTracks(genreName, p.appCfg.EnqueueBatchSize)
		if err != nil {
			return nil, err
		}
		return sharedutil.FilterSlice(tr, func(t *mediaprovider.Track) bool {
			skipKwd := p.cfg.SkipKeywordWhenShuffling
			include :=
				(skipKwd == "" || !strcase.Contains(t.Title, skipKwd)) &&
					(!p.cfg.SkipOneStarWhenShuffling || t.Rating != 1)
			return include
		}), nil
	})
}

func (p *PlaybackManager) PlaySimilarSongs(id string) error {
	return p.fetchAndPlayTracks(func() ([]*mediaprovider.Track, error) {
		return p.engine.sm.Server.GetSimilarTracks(id, p.appCfg.EnqueueBatchSize)
	})
}

func (p *PlaybackManager) PlayRandomAlbums(genreName string) error {
	mp := p.engine.sm.GetServer()
	if mp == nil {
		return errors.New("logged out")
	}

	if p.engine.replayGainCfg.Mode == ReplayGainAuto {
		p.SetReplayGainMode(player.ReplayGainAlbum)
	}

	var options mediaprovider.AlbumFilterOptions
	if genreName != "" {
		options = mediaprovider.AlbumFilterOptions{
			Genres: []string{genreName},
		}
	}
	iter := mp.IterateAlbums(mediaprovider.AlbumSortRandom, mediaprovider.NewAlbumFilter(options))
	insertMode := Replace
	for i := range 20 {
		al := iter.Next()
		if al == nil {
			break
		}
		if al, err := mp.GetAlbum(al.ID); err == nil {
			p.LoadTracks(al.Tracks, insertMode, false)
			if i == 0 {
				p.PlayFromBeginning()
				insertMode = Append
			}
		}
	}

	return nil
}

func (p *PlaybackManager) LoadRadioStation(station *mediaprovider.RadioStation, queueMode InsertQueueMode) {
	p.cmdQueue.LoadRadioStation(station, queueMode)
}

func (p *PlaybackManager) PlayRadioStation(station *mediaprovider.RadioStation) {
	p.LoadRadioStation(station, Replace)
	p.PlayFromBeginning()
}

func (p *PlaybackManager) fetchAndPlayTracks(fetchFn func() ([]*mediaprovider.Track, error)) error {
	if songs, err := fetchFn(); err != nil {
		return err
	} else {
		p.LoadTracks(songs, Replace, false)
		if p.engine.replayGainCfg.Mode == ReplayGainAuto {
			p.SetReplayGainMode(player.ReplayGainTrack)
		}
		p.PlayFromBeginning()
		return nil
	}
}

func (p *PlaybackManager) GetActivePlayQueue() []mediaprovider.MediaItem {
	return p.engine.GetActivePlayQueueDeepCopy()
}

func (p *PlaybackManager) GetPlayQueue() []mediaprovider.MediaItem {
	return p.engine.GetPlayQueueDeepCopy()
}

func (p *PlaybackManager) GetShuffledPlayQueue() []mediaprovider.MediaItem {
	return p.engine.GetShuffledPlayQueueDeepCopy()
}

// Any time the user changes the favorite status of a track elsewhere in the app,
// this should be called to ensure the in-memory track model is updated.
func (p *PlaybackManager) OnTrackFavoriteStatusChanged(id string, fav bool) {
	p.engine.OnTrackFavoriteStatusChanged(id, fav)
}

// Any time the user changes the rating of a track elsewhere in the app,
// this should be called to ensure the in-memory track model is updated.
func (p *PlaybackManager) OnTrackRatingChanged(id string, rating int) {
	p.engine.OnTrackRatingChanged(id, rating)
}

func (p *PlaybackManager) RemoveTracksFromQueue(idxs []int) {
	p.cmdQueue.RemoveItemsFromQueue(idxs)
}

// Stop playback and clear the play queue.
func (p *PlaybackManager) StopAndClearPlayQueue() {
	p.cmdQueue.StopAndClearPlayQueue()
}

func (p *PlaybackManager) SetReplayGainOptions(config ReplayGainConfig) {
	p.engine.SetReplayGainOptions(config)
}

func (p *PlaybackManager) SetReplayGainMode(mode player.ReplayGainMode) {
	p.engine.SetReplayGainMode(mode)
}

// Changes the loop mode of the player to the next one.
// Useful for toggling UI elements, to change modes without knowing the current player mode.
func (p *PlaybackManager) SetNextLoopMode() {
	switch p.engine.loopMode {
	case LoopNone:
		p.cmdQueue.SetLoopMode(LoopAll)
	case LoopAll:
		p.cmdQueue.SetLoopMode(LoopOne)
	case LoopOne:
		p.cmdQueue.SetLoopMode(LoopNone)
	}
}

func (p *PlaybackManager) SetLoopMode(loopMode LoopMode) {
	p.cmdQueue.SetLoopMode(loopMode)
}

func (p *PlaybackManager) GetLoopMode() LoopMode {
	return p.engine.loopMode
}

func (p *PlaybackManager) IsAutoplay() bool {
	return p.cfg.Autoplay
}

func (p *PlaybackManager) PlaybackStatus() PlaybackStatus {
	return p.engine.PlaybackStatus()
}

func (p *PlaybackManager) SetVolume(vol int) {
	p.cmdQueue.SetVolume(vol)
}

func (p *PlaybackManager) SetAutoplay(autoplay bool) {
	p.cfg.Autoplay = autoplay
	if autoplay && p.NowPlayingIndex() == p.engine.getPlayQueueLength()-1 {
		p.enqueueAutoplayTracks()
	}
}

func (p *PlaybackManager) SetShuffle(shuffle bool) {
	p.cfg.Shuffle = shuffle
	p.engine.SetShuffle(shuffle)
}

func (p *PlaybackManager) Volume() int {
	return p.engine.CurrentPlayer().GetVolume()
}

func (p *PlaybackManager) SeekNext() {
	p.cmdQueue.SeekNext()
}

func (p *PlaybackManager) SeekBackOrPrevious() {
	p.cmdQueue.SeekBackOrPrevious()
}

// Seek to given absolute position in the current track by seconds.
func (p *PlaybackManager) SeekSeconds(sec float64) {
	p.cmdQueue.SeekSeconds(sec)
}

// Seek by given relative position in the current track by seconds.
func (p *PlaybackManager) SeekBySeconds(sec float64) {
	status := p.engine.PlaybackStatus()
	target := status.TimePos + sec
	if target < 0 {
		target = 0
	} else if target > status.Duration {
		target = status.Duration
	}
	p.cmdQueue.SeekSeconds(target)
}

// Seek to a fractional position in the current track [0..1]
func (p *PlaybackManager) SeekFraction(fraction float64) {
	if fraction < 0 {
		fraction = 0
	} else if fraction > 1 {
		fraction = 1
	}
	target := p.engine.curTrackDuration * fraction
	p.cmdQueue.SeekSeconds(target)
}

func (p *PlaybackManager) Stop() {
	p.cmdQueue.Stop()
}

func (p *PlaybackManager) Shutdown() {
	p.cmdQueue.StopAndWait()
}

func (p *PlaybackManager) Pause() {
	p.cmdQueue.Pause()
}

func (p *PlaybackManager) Continue() {
	p.cmdQueue.Continue()
}

func (p *PlaybackManager) PlayPause() {
	switch p.engine.PlaybackStatus().State {
	case player.Playing:
		p.Pause()
	case player.Paused:
		p.Continue()
	case player.Stopped:
		p.PlayTrackAt(0)
	}
}

func (p *PlaybackManager) SetPauseAfterCurrent(pauseAfterCurrent bool) {
	p.engine.SetPauseAfterCurrent(pauseAfterCurrent)
}

func (p *PlaybackManager) IsPauseAfterCurrent() bool {
	return p.engine.pauseAfterCurrent
}

func (p *PlaybackManager) enqueueAutoplayTracks() {
	nowPlaying := p.NowPlaying()
	if nowPlaying == nil {
		return
	}
	if nowPlaying.Metadata().Type == mediaprovider.MediaItemTypeRadioStation {
		return
	}

	s := p.engine.sm.Server
	if s == nil {
		return
	}

	// last 500 played items
	queue := p.GetActivePlayQueue()
	if l := len(queue); l > 500 {
		queue = queue[l-500:]
	}

	// tracks we will enqueue
	var tracks []*mediaprovider.Track

	filterAutoplayTracks := func(tracks []*mediaprovider.Track) []*mediaprovider.Track {
		return sharedutil.FilterSlice(tracks, func(t *mediaprovider.Track) bool {
			shouldSkip :=
				(p.cfg.SkipOneStarWhenShuffling && t.Rating == 1) ||
					(p.cfg.SkipKeywordWhenShuffling != "" && strcase.Contains(t.Title, p.cfg.SkipKeywordWhenShuffling))
			recentlyPlayed := slices.ContainsFunc(queue, func(i mediaprovider.MediaItem) bool {
				return i.Metadata().Type == mediaprovider.MediaItemTypeTrack && i.Metadata().ID == t.ID
			})
			return !shouldSkip && !recentlyPlayed
		})
	}

	// since this func is invoked in a callback from the playback engine,
	// need to do the rest async as it may take time and block other callbacks
	p.pendingAutoplay = true
	go func() {
		defer func() { p.pendingAutoplay = false }()

		// first 2 strategies - similar by artist, and similar by genres - only work for tracks
		if nowPlaying.Metadata().Type == mediaprovider.MediaItemTypeTrack {
			tr := nowPlaying.(*mediaprovider.Track)

			// similar tracks by artist
			if len(tr.ArtistIDs) > 0 {
				similar, err := s.GetSimilarTracks(tr.ArtistIDs[0], p.appCfg.EnqueueBatchSize)
				if err != nil {
					log.Printf("autoplay error: failed to get similar tracks: %v", err)
				}
				tracks = filterAutoplayTracks(similar)
			}

			// fallback to random tracks from genre
			if len(tracks) == 0 {
				for _, g := range tr.Genres {
					if g == "" {
						continue
					}
					byGenre, err := s.GetRandomTracks(g, p.appCfg.EnqueueBatchSize)
					if err != nil {
						log.Printf("autoplay error: failed to get tracks by genre: %v", err)
					}
					tracks = filterAutoplayTracks(byGenre)
					if len(tracks) > 0 {
						break
					}
				}
			}
		}

		// random tracks works regardless of the type of the last playing media
		if len(tracks) == 0 {
			// fallback to random tracks
			random, err := s.GetRandomTracks("", p.appCfg.EnqueueBatchSize)
			if err != nil {
				log.Printf("autoplay error: failed to get random tracks: %v", err)
			}
			tracks = filterAutoplayTracks(random)
		}

		if len(tracks) > 0 {
			p.LoadTracks(tracks, Append, false /*no need to shuffle, already random*/)
		}
	}()
}

func (p *PlaybackManager) runCmdQueue(ctx context.Context) {
	logIfErr := func(action string, err error) {
		if err != nil {
			log.Printf("Playback error (%s): %v", action, err)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-p.cmdQueue.C():
			switch c.Type {
			case cmdStop:
				logIfErr("Stop", p.engine.Stop())
			case cmdContinue:
				logIfErr("Continue", p.engine.Continue())
			case cmdPause:
				logIfErr("Pause", p.engine.Pause())
			case cmdPlayTrackAt:
				logIfErr("PlayTrackAt", p.engine.PlayTrackAt(c.Arg.(int)))
			case cmdSeekSeconds:
				logIfErr("SeekSeconds", p.engine.SeekSeconds(c.Arg.(float64)))
			case cmdSeekFwdBackN:
				action := fmt.Sprintf("SeekFwdBack[%d]", c.Arg.(int))
				logIfErr(action, p.engine.SeekFwdBackN(c.Arg.(int)))
			case cmdVolume:
				logIfErr("Volume", p.engine.SetVolume(c.Arg.(int)))
			case cmdLoopMode:
				p.engine.SetLoopMode(c.Arg.(LoopMode))
			case cmdStopAndClearPlayQueue:
				p.engine.StopAndClearPlayQueue()
			case cmdUpdatePlayQueue:
				logIfErr("UpdatePlayQueue", p.engine.UpdatePlayQueue(c.Arg.([]mediaprovider.MediaItem)))
			case cmdRemoveTracksFromQueue:
				p.engine.RemoveTracksFromQueue(c.Arg.([]int))
			case cmdLoadItems:
				err := p.engine.LoadItems(
					c.Arg.([]mediaprovider.MediaItem),
					c.Arg2.(InsertQueueMode),
					c.Arg3.(bool),
				)
				logIfErr("LoadItems", err)
			case cmdLoadItemsAndPlayAtIdx:
				err := p.engine.LoadItemsAndPlayAtIdx(
					c.Arg.([]mediaprovider.MediaItem),
					c.Arg2.(bool),
					c.Arg3.(int),
				)
				logIfErr("LoadItemsAndPlayAtIdx", err)
			case cmdSetQueueState:
				err := p.engine.SetQueueState(
					c.Arg.([]*mediaprovider.Track),
					c.Arg2.(QueueType),
				)
				logIfErr("SetQueueState", err)
			case cmdLoadRadioStation:
				p.engine.LoadRadioStation(
					c.Arg.(*mediaprovider.RadioStation),
					c.Arg2.(InsertQueueMode),
				)
			case cmdLoadTrackPaused:
				logIfErr("LoadTrackPaused", p.engine.loadTrackPaused(c.Arg.(int), c.Arg2.(float64)))
			case cmdForceRestartPlayback:
				if mpv, ok := p.engine.CurrentPlayer().(*mpv.Player); ok {
					log.Println("Force-restarting MPV playback")

					// restart player, but perserve the state
					isPaused := false
					stat := p.engine.CurrentPlayer().GetStatus()
					if stat.State == player.Paused {
						isPaused = true
					}
					mpv.ForceRestartPlayback(isPaused)
				}
			}
			if c.OnDone != nil {
				c.OnDone()
			}
		}
	}
}
