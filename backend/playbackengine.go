package backend

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"slices"
	"strings"
	"time"

	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/backend/player"
	"github.com/dweymouth/supersonic/backend/player/mpv"
	"github.com/dweymouth/supersonic/backend/util"
	"github.com/dweymouth/supersonic/sharedutil"
)

// TODO: make thread-safe

var (
	ReplayGainNone  = player.ReplayGainNone.String()
	ReplayGainAlbum = player.ReplayGainAlbum.String()
	ReplayGainTrack = player.ReplayGainTrack.String()
	ReplayGainAuto  = "Auto"
)

type InsertQueueMode int

const (
	Replace InsertQueueMode = iota
	InsertNext
	Append
)

type QueueType int

const (
	PlayQueue QueueType = iota
	ShuffledPlayQueue
	Both
)

// The playback loop mode (LoopNone, LoopAll, LoopOne).
type LoopMode int

const (
	LoopNone LoopMode = iota
	LoopAll
	LoopOne
)

type PlaybackState = player.State

type PlaybackStatus struct {
	State    PlaybackState
	TimePos  float64
	Duration float64
}

type playbackEngine struct {
	ctx           context.Context
	cancelPollPos context.CancelFunc
	sm            *ServerManager
	audiocache    *AudioCache
	player        player.BasePlayer

	playTimeStopwatch   util.Stopwatch
	curTrackDuration    float64
	latestTrackPosition float64 // cleared by checkScrobble
	callbacksDisabled   bool

	playQueue         []mediaprovider.MediaItem
	shuffledPlayQueue []mediaprovider.MediaItem

	nowPlayingIdx int
	isRadio       bool
	loopMode      LoopMode
	shuffle       bool

	pauseAfterCurrent bool // flag to pause playback after current track ends

	// flags for handleOnTrackChange / handleOnStopped callbacks - reset to false in the callbacks
	wasStopped       bool // true iff player was stopped before handleOnTrackChange invocation
	alreadyScrobbled bool // true iff the previously-playing track was already scrobbled

	// if >= 0, track number that was requested by PlayTrackAt
	// onTrackChange callback should set nowPlayingIdx to this,
	// and reset this to -1
	pendingTrackChangeNum int

	pendingPlayerChange       bool
	pendingPlayerChangeStatus player.Status

	// set when restoring session state: track is conceptually loaded+paused
	// but MPV hasn't been touched yet; cleared on Continue or playTrackAt
	pendingLoadPaused    bool
	pendingLoadStartTime float64

	// Whether we need to set the next track on the Player
	// before the current track completes (normally when 10 seconds remain
	// in the time pos polling function)
	needToSetNextTrack bool

	// to pass to onSongChange listeners; clear once listeners have been called
	lastScrobbled *mediaprovider.Track
	playbackCfg   *PlaybackConfig
	scrobbleCfg   *ScrobbleConfig
	transcodeCfg  *TranscodingConfig
	replayGainCfg ReplayGainConfig

	// registered callbacks
	onBeforeSongChange []func(next mediaprovider.MediaItem)
	onSongChange       []func(nowPlaying mediaprovider.MediaItem, justScrobbledIfAny *mediaprovider.Track)
	onPlayTimeUpdate   []func(float64, float64, bool)
	onLoopModeChange   []func(LoopMode)
	onShuffleChange    []func(bool)
	onVolumeChange     []func(int)
	onSeek             []func()
	onPaused           []func()
	onStopped          []func()
	onPlaying          []func()
	onQueueChange      []func()

	onRadioMetadataChange []func(radioName, title, artist string)
}

func NewPlaybackEngine(
	ctx context.Context,
	s *ServerManager,
	c *AudioCache,
	p player.BasePlayer,
	playbackCfg *PlaybackConfig,
	scrobbleCfg *ScrobbleConfig,
	transcodeCfg *TranscodingConfig,
) *playbackEngine {
	// clamp to 99% to avoid any possible rounding issues
	scrobbleCfg.ThresholdPercent = clamp(scrobbleCfg.ThresholdPercent, 0, 99)
	pm := &playbackEngine{
		ctx:           ctx,
		sm:            s,
		audiocache:    c,
		player:        p,
		playbackCfg:   playbackCfg,
		scrobbleCfg:   scrobbleCfg,
		transcodeCfg:  transcodeCfg,
		nowPlayingIdx: -1,
		wasStopped:    true,
	}
	switch playbackCfg.RepeatMode {
	case "All":
		pm.loopMode = LoopAll
	case "One":
		pm.loopMode = LoopOne
	}

	pm.shuffle = playbackCfg.Shuffle

	pm.registerPlayerCallbacks(p)
	s.OnLogout(func() {
		pm.StopAndClearPlayQueue()
	})

	return pm
}

func (p *playbackEngine) registerPlayerCallbacks(pl player.BasePlayer) {
	pl.OnTrackChange(p.handleOnTrackChange)
	pl.OnSeek(func() {
		p.handleTimePosUpdate(true)
		p.invokeNoArgCallbacks(p.onSeek)
		seekState := "playing"
		if p.PlaybackStatus().State == player.Paused {
			seekState = "paused"
		}
		p.reportPlayback(seekState)
	})
	pl.OnStopped(p.handleOnStopped)
	pl.OnPaused(func() {
		p.playTimeStopwatch.Stop()
		p.stopPollTimePos()
		p.invokeNoArgCallbacks(p.onPaused)
		p.reportPlayback("paused")
	})
	pl.OnPlaying(func() {
		p.playTimeStopwatch.Start()
		p.startPollTimePos()
		p.invokeNoArgCallbacks(p.onPlaying)
		p.reportPlayback("playing")
	})
}

func (p *playbackEngine) unregisterPlayerCallbacks(pl player.BasePlayer) {
	pl.OnPaused(nil)
	pl.OnPlaying(nil)
	pl.OnStopped(nil)
	pl.OnSeek(nil)
	pl.OnTrackChange(nil)
}

func (p *playbackEngine) SetPlayer(pl player.BasePlayer) error {
	needToUnpause := false

	stat := p.CurrentPlayer().GetStatus()
	if p.pendingPlayerChange || p.pendingLoadPaused {
		stat.State = player.Paused
	}
	if p.pendingLoadPaused {
		stat.TimePos = p.pendingLoadStartTime
		stat.Duration = p.curTrackDuration
	}

	switch stat.State {
	case player.Stopped:
		// nothing
	case player.Playing:
		needToUnpause = true
		p.stopPollTimePos()
		fallthrough
	case player.Paused:
		p.pendingPlayerChangeStatus = stat
		p.pendingPlayerChange = true
	}
	p.unregisterPlayerCallbacks(p.player)
	if err := p.player.Stop(true); err != nil {
		log.Printf("failed to stop player: %v", err)
	}

	oldVol := p.player.GetVolume()
	if _, isMPV := p.player.(*mpv.Player); !isMPV {
		p.player.Destroy()
	}
	p.player = pl
	p.registerPlayerCallbacks(pl)

	if needToUnpause {
		p.playTrackAt(p.nowPlayingIdx, p.pendingPlayerChangeStatus.TimePos)
		p.pendingPlayerChange = false
	}
	vol := pl.GetVolume()
	if oldVol != vol {
		for _, cb := range p.onVolumeChange {
			cb(vol)
		}
	}
	return nil
}

// ======================= START PLAY QUEUE FUNCS ===========================

func (p *playbackEngine) getPlayQueue() []mediaprovider.MediaItem {
	return p.playQueue
}

func (p *playbackEngine) getShuffledPlayQueue() []mediaprovider.MediaItem {
	return p.shuffledPlayQueue
}

func (p *playbackEngine) getActivePlayQueue() []mediaprovider.MediaItem {
	if p.shuffle {
		return p.shuffledPlayQueue
	}
	return p.playQueue
}

func (p *playbackEngine) getPlayQueueLength() int {
	return len(p.getActivePlayQueue())
}

func (p *playbackEngine) clearPlayQueue() {
	p.checkScrobble()
	p.player.Stop(false)
	p.nowPlayingIdx = -1
	p.playQueue = nil
	p.shuffledPlayQueue = nil
}

func (p *playbackEngine) setPlayQueue(items []mediaprovider.MediaItem) {
	p.playQueue = items
}

func (p *playbackEngine) setShuffledPlayQueue(items []mediaprovider.MediaItem) {
	p.shuffledPlayQueue = items
}

func (p *playbackEngine) getPlayQueueItemAt(idx int) mediaprovider.MediaItem {
	return p.getActivePlayQueue()[idx]
}

func (p *playbackEngine) insertItemsIntoPlayQueueAt(items []mediaprovider.MediaItem, idx int, queueType QueueType) {
	switch queueType {
	case Both:
		p.playQueue = append(p.playQueue[:idx], append(items, p.playQueue[idx:]...)...)
		p.shuffledPlayQueue = append(p.shuffledPlayQueue[:idx], append(items, p.shuffledPlayQueue[idx:]...)...)
	case PlayQueue:
		p.playQueue = append(p.playQueue[:idx], append(items, p.playQueue[idx:]...)...)
	case ShuffledPlayQueue:
		p.shuffledPlayQueue = append(p.shuffledPlayQueue[:idx], append(items, p.shuffledPlayQueue[idx:]...)...)
	}
}

func (p *playbackEngine) GetPlayQueueDeepCopy() []mediaprovider.MediaItem {
	return deepCopyMediaItemSlice(p.getPlayQueue())
}

func (p *playbackEngine) GetShuffledPlayQueueDeepCopy() []mediaprovider.MediaItem {
	return deepCopyMediaItemSlice(p.getShuffledPlayQueue())
}

func (p *playbackEngine) GetActivePlayQueueDeepCopy() []mediaprovider.MediaItem {
	return deepCopyMediaItemSlice(p.getActivePlayQueue())
}

// ======================== END PLAY QUEUE FUNCS =============================

func (p *playbackEngine) PlayTrackAt(idx int) error {
	return p.playTrackAt(idx, 0)
}

// loadTrackPaused sets up engine state as if the track at idx is loaded and
// paused at startTime, firing UI/OS-integration callbacks, but does NOT touch
// the underlying player. Call Continue() to actually begin playback.
func (p *playbackEngine) loadTrackPaused(idx int, startTime float64) error {
	if l := p.getPlayQueueLength(); idx < 0 || idx >= l {
		return fmt.Errorf("track index (%d) out of range (0-%d)", idx, l)
	}
	p.nowPlayingIdx = idx
	nowPlaying := p.getPlayQueueItemAt(idx)
	_, p.isRadio = nowPlaying.(*mediaprovider.RadioStation)
	p.wasStopped = false
	p.alreadyScrobbled = false
	p.curTrackDuration = nowPlaying.Metadata().Duration.Seconds()
	p.pendingLoadPaused = true
	p.pendingLoadStartTime = startTime

	// Pre-generate the waveform image for the paused track using the same hook
	// that normally pre-generates waveforms for the upcoming track.
	for _, cb := range p.onBeforeSongChange {
		cb(nowPlaying)
	}

	p.invokeOnSongChangeCallbacks()
	p.invokeNoArgCallbacks(p.onPaused)
	p.handleTimePosUpdate(false)
	p.handleNextTrackUpdated()
	return nil
}

func (p *playbackEngine) playTrackAt(idx int, startTime float64) error {
	if l := p.getPlayQueueLength(); idx < 0 || idx >= l {
		return fmt.Errorf("track index (%d) out of range (0-%d)", idx, l)
	}
	p.pendingLoadPaused = false
	// scrobble current track if needed
	p.checkScrobble()
	p.alreadyScrobbled = true
	p.pendingTrackChangeNum = idx
	err := p.setTrack(idx, false, startTime)
	return err
}

// Gets the curently playing media item, if any.
func (p *playbackEngine) NowPlaying() mediaprovider.MediaItem {
	if p.nowPlayingIdx < 0 || p.getPlayQueueLength() == 0 {
		return nil
	}
	if !p.pendingLoadPaused && p.player.GetStatus().State == player.Stopped {
		return nil
	}
	return p.getPlayQueueItemAt(p.nowPlayingIdx)
}

func (p *playbackEngine) NowPlayingIndex() int {
	return int(p.nowPlayingIdx)
}

func (p *playbackEngine) SetLoopMode(loopMode LoopMode) {
	p.loopMode = loopMode
	if p.nowPlayingIdx >= 0 {
		// TODO - don't need when going from LoopNone to LoopAll
		// if not on last track
		p.handleNextTrackUpdated()
	}

	for _, cb := range p.onLoopModeChange {
		cb(loopMode)
	}
}

func (p *playbackEngine) GetLoopMode() LoopMode {
	return p.loopMode
}

func (p *playbackEngine) GetTrackIdxByIdFrom(items []mediaprovider.MediaItem, id string) int {
	foundIdx := -1
	for i, tr := range items {
		if tr.Metadata().ID == id {
			foundIdx = i
			break
		}
	}
	return foundIdx
}

func (p *playbackEngine) SetShuffle(shuffle bool) {
	if p.shuffle == shuffle {
		return
	}

	for _, cb := range p.onShuffleChange {
		cb(shuffle)
	}
	p.shuffle = shuffle

	// guard against changing shuffle with an empty queue
	if p.getPlayQueue() == nil || len(p.getPlayQueue()) == 0 {
		return
	}

	newNowPlayingIdx := 0
	if shuffle {
		shuffledQueue := deepCopyMediaItemSlice(p.playQueue)
		rand.Shuffle(len(shuffledQueue), func(i, j int) {
			shuffledQueue[i], shuffledQueue[j] = shuffledQueue[j], shuffledQueue[i]
		})
		if p.nowPlayingIdx >= 0 && len(p.getPlayQueue()) > p.nowPlayingIdx {
			nowPlayingID := p.getPlayQueue()[p.nowPlayingIdx].Metadata().ID
			p.setShuffledPlayQueue(sharedutil.ReorderItems(shuffledQueue, []int{p.GetTrackIdxByIdFrom(shuffledQueue, nowPlayingID)}, 0))
		} else {
			return
		}
	} else {
		if p.nowPlayingIdx >= 0 && len(p.getShuffledPlayQueue()) > p.nowPlayingIdx {
			nowPlayingID := p.getShuffledPlayQueue()[p.nowPlayingIdx].Metadata().ID
			newNowPlayingIdx = p.GetTrackIdxByIdFrom(p.playQueue, nowPlayingID)
		} else {
			return
		}
	}

	p.nowPlayingIdx = newNowPlayingIdx
	p.handleNextTrackUpdated()
	p.invokeNoArgCallbacks(p.onQueueChange)
}

func (p *playbackEngine) PlaybackStatus() PlaybackStatus {
	if p.pendingLoadPaused {
		return PlaybackStatus{
			State:    player.Paused,
			TimePos:  p.pendingLoadStartTime,
			Duration: p.curTrackDuration,
		}
	}
	stat := p.pendingPlayerChangeStatus
	if !p.pendingPlayerChange {
		stat = p.CurrentPlayer().GetStatus()
	}
	return PlaybackStatus{
		State:    stat.State,
		TimePos:  stat.TimePos,
		Duration: p.curTrackDuration,
	}
}

func (p *playbackEngine) SetVolume(vol int) error {
	vol = clamp(vol, 0, 100)
	if err := p.player.SetVolume(vol); err != nil {
		return err
	}
	for _, cb := range p.onVolumeChange {
		cb(vol)
	}
	return nil
}

func (p *playbackEngine) CurrentPlayer() player.BasePlayer {
	return p.player
}

func (p *playbackEngine) SeekNext() error {
	if p.PlaybackStatus().State == player.Stopped {
		return nil
	}
	return p.PlayTrackAt(p.nowPlayingIdx + 1)
}

func (p *playbackEngine) SeekBackOrPrevious() error {
	if p.nowPlayingIdx == 0 || p.PlaybackStatus().TimePos > 3 {
		return p.player.SeekSeconds(0)
	}
	return p.PlayTrackAt(p.nowPlayingIdx - 1)
}

func (p *playbackEngine) SeekFwdBackN(n int) error {
	idx := p.nowPlayingIdx
	if n < 0 && p.PlaybackStatus().TimePos > 3 {
		n += 1 // first seek back is just seek to beginning of current
	}
	if n == 0 || (idx == 0 && n < 0) {
		return p.player.SeekSeconds(0) // seek back in current song
	}

	lastIdx := p.getPlayQueueLength() - 1
	newIdx := min(lastIdx, max(0, idx+n))

	if idx == lastIdx && n > 0 {
		newIdx = 0
	}
	return p.PlayTrackAt(newIdx)
}

// Seek to given absolute position in the current track by seconds.
func (p *playbackEngine) SeekSeconds(sec float64) error {
	if p.isRadio {
		return nil // can't seek radio stations
	}
	return p.player.SeekSeconds(sec)
}

func (p *playbackEngine) IsSeeking() bool {
	return p.player.IsSeeking()
}

func (p *playbackEngine) Stop() error {
	if p.pendingLoadPaused {
		p.pendingLoadPaused = false
		p.pendingLoadStartTime = 0
		p.handleOnStopped()
	}
	return p.player.Stop(false)
}

func (p *playbackEngine) SetPauseAfterCurrent(pauseAfterCurrent bool) {
	p.pauseAfterCurrent = pauseAfterCurrent
}

func (p *playbackEngine) Pause() error {
	return p.player.Pause()
}

func (p *playbackEngine) Continue() error {
	if p.pendingLoadPaused {
		p.pendingLoadPaused = false
		// Use pendingTrackChangeNum so handleOnTrackChange doesn't advance nowPlayingIdx.
		// Set alreadyScrobbled so it doesn't try to scrobble the not-yet-played track.
		p.pendingTrackChangeNum = p.nowPlayingIdx
		p.alreadyScrobbled = true
		return p.setTrack(p.nowPlayingIdx, false, p.pendingLoadStartTime)
	}

	if p.pendingPlayerChange {
		p.pendingPlayerChange = false
		return p.playTrackAt(p.nowPlayingIdx, p.pendingPlayerChangeStatus.TimePos)
	}

	if p.PlaybackStatus().State == player.Stopped {
		return p.PlayTrackAt(0)
	}
	return p.player.Continue()
}

// Load items into the specified queue(s). This overrides the queue. For standard inserts or replaces use p.LoadItems
// This is used on program startup to populate both the playQueue and shuffledPlayQueue with the previously saved client state.
func (p *playbackEngine) SetQueueState(tracks []*mediaprovider.Track, queueType QueueType) error {
	newTracks := sharedutil.CopyTrackSliceToMediaItemSlice(tracks)
	switch queueType {
	case PlayQueue:
		p.setPlayQueue(newTracks)
	case ShuffledPlayQueue:
		p.setShuffledPlayQueue(newTracks)
	case Both:
		p.setPlayQueue(newTracks)
		p.setShuffledPlayQueue(newTracks)
	}
	p.invokeNoArgCallbacks(p.onQueueChange)
	return nil
}

// Load items into the play queue.
// If replacing the current queue (!appendToQueue), playback will be stopped.
func (p *playbackEngine) LoadItems(items []mediaprovider.MediaItem, insertQueueMode InsertQueueMode, shuffle bool) error {
	newItems := deepCopyMediaItemSlice(items)
	return p.doLoaditems(newItems, insertQueueMode, shuffle)
}

// Load items into the play queue and play the track at idx.
// Replaces the playQueue, shuffledPlayQueue, or both
func (p *playbackEngine) LoadItemsAndPlayAtIdx(items []mediaprovider.MediaItem, shuffle bool, idx int) error {
	newItems := deepCopyMediaItemSlice(items)

	if p.shuffle || shuffle {
		p.clearPlayQueue()
		rand.Shuffle(len(newItems), func(i, j int) {
			newItems[i], newItems[j] = newItems[j], newItems[i]
		})
		if idx < len(items) {
			nowPlayingID := items[idx].Metadata().ID
			p.setPlayQueue(deepCopyMediaItemSlice(items))
			p.setShuffledPlayQueue(sharedutil.ReorderItems(newItems, []int{p.GetTrackIdxByIdFrom(newItems, nowPlayingID)}, 0))
			p.PlayTrackAt(0)
		}
	} else {
		p.doLoaditems(newItems, Replace, shuffle)
		p.PlayTrackAt(idx)
	}

	p.handleNextTrackUpdated()
	p.invokeNoArgCallbacks(p.onQueueChange)

	return nil
}

// Load tracks into the play queue.
// If replacing the current queue (!appendToQueue), playback will be stopped.
func (p *playbackEngine) LoadTracks(tracks []*mediaprovider.Track, insertQueueMode InsertQueueMode, shuffle bool) error {
	newTracks := sharedutil.CopyTrackSliceToMediaItemSlice(tracks)
	return p.doLoaditems(newTracks, insertQueueMode, shuffle)
}

func (p *playbackEngine) doLoaditems(items []mediaprovider.MediaItem, insertQueueMode InsertQueueMode, shuffle bool) error {
	queueType := PlayQueue

	if insertQueueMode == Replace {
		p.clearPlayQueue()
	}
	if nextChanged := len(items) > 0 && (insertQueueMode != Append || (p.nowPlayingIdx == p.getPlayQueueLength()-1)); nextChanged {
		defer p.handleNextTrackUpdated()
	}

	if shuffle {
		rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	}

	insertIdx := p.getPlayQueueLength()

	if insertQueueMode == Replace {
		if p.shuffle {
			shuffledItems := deepCopyMediaItemSlice(items)
			rand.Shuffle(len(shuffledItems), func(i, j int) { shuffledItems[i], shuffledItems[j] = shuffledItems[j], shuffledItems[i] })

			p.insertItemsIntoPlayQueueAt(items, insertIdx, PlayQueue)
			p.insertItemsIntoPlayQueueAt(shuffledItems, insertIdx, ShuffledPlayQueue)
		} else {
			p.insertItemsIntoPlayQueueAt(items, insertIdx, queueType)
		}
	} else {
		if insertQueueMode == InsertNext {
			insertIdx = p.nowPlayingIdx + 1
		}
		if p.shuffle {
			queueType = Both
		}
		p.insertItemsIntoPlayQueueAt(items, insertIdx, queueType)
	}

	p.invokeNoArgCallbacks(p.onQueueChange)
	return nil
}

func (p *playbackEngine) LoadRadioStation(radio *mediaprovider.RadioStation, insertMode InsertQueueMode) {
	if insertMode == Replace {
		p.clearPlayQueue()
	}
	if nextChanged := insertMode == InsertNext || (insertMode == Append && p.nowPlayingIdx == p.getPlayQueueLength()-1); nextChanged {
		p.handleNextTrackUpdated()
	}
	insertIdx := p.getPlayQueueLength()
	if insertMode == InsertNext {
		insertIdx = p.nowPlayingIdx + 1
	}
	p.insertItemsIntoPlayQueueAt([]mediaprovider.MediaItem{radio}, insertIdx, Both)
	p.invokeNoArgCallbacks(p.onQueueChange)
}

// Stop playback and clear the play queue.
func (p *playbackEngine) StopAndClearPlayQueue() {
	changed := p.getPlayQueueLength() > 0
	p.clearPlayQueue()
	if changed {
		p.invokeNoArgCallbacks(p.onQueueChange)
	}
}

// Any time the user changes the favorite status of a track elsewhere in the app,
// this should be called to ensure the in-memory track model is updated.
func (p *playbackEngine) OnTrackFavoriteStatusChanged(id string, fav bool) {
	if item := sharedutil.FindMediaItemByID(id, p.getActivePlayQueue()); item != nil {
		if tr, ok := item.(*mediaprovider.Track); ok {
			tr.Favorite = fav
		}
	}
}

// Any time the user changes the rating of a track elsewhere in the app,
// this should be called to ensure the in-memory track model is updated.
func (p *playbackEngine) OnTrackRatingChanged(id string, rating int) {
	if item := sharedutil.FindMediaItemByID(id, p.getActivePlayQueue()); item != nil {
		if tr, ok := item.(*mediaprovider.Track); ok {
			tr.Rating = rating
		}
	}
}

// Replaces the play queue with the given set of tracks.
// Does not stop playback if the currently playing track is in the new queue,
// but updates the now playing index to point to the first instance of the track in the new queue.
func (p *playbackEngine) UpdatePlayQueue(items []mediaprovider.MediaItem) error {
	newQueue := deepCopyMediaItemSlice(items)
	newNowPlayingIdx := -1
	if p.nowPlayingIdx >= 0 {
		nowPlayingID := p.getPlayQueueItemAt(p.nowPlayingIdx).Metadata().ID
		for i, tr := range newQueue {
			if tr.Metadata().ID == nowPlayingID {
				newNowPlayingIdx = i
				break
			}
		}
	}
	if p.shuffle {
		p.setShuffledPlayQueue(newQueue)
	} else {
		p.setPlayQueue(newQueue)
	}
	if p.nowPlayingIdx >= 0 && newNowPlayingIdx == -1 {
		return p.Stop()
	}
	if p.nowPlayingIdx >= 0 {
		p.handleNextTrackUpdated()
	}
	p.nowPlayingIdx = newNowPlayingIdx

	p.invokeNoArgCallbacks(p.onQueueChange)
	return nil
}

func (p *playbackEngine) RemoveTracksFromQueue(idxs []int) {
	isPlayingTrackRemoved := false
	isNextPlayingTrackremoved := false
	nowPlaying := p.NowPlayingIndex()
	newNowPlaying := nowPlaying

	if p.shuffle {
		// remove tracks by ID from playQueue
		ids := p.GetTrackIdsFromIdx(idxs)
		newPlayQueue := make([]mediaprovider.MediaItem, 0, p.getPlayQueueLength()-len(idxs))
		for _, tr := range p.getPlayQueue() {
			if slices.Contains(ids, tr.Metadata().ID) {
				//remove id from id list, handles having the same track present multiple times in playQueue
				idx := slices.Index(ids, tr.Metadata().ID)
				ids = slices.Delete(ids, idx, idx+1)
			} else {
				// not removing this track
				newPlayQueue = append(newPlayQueue, tr)
			}
		}
		p.setPlayQueue(newPlayQueue)

		newShuffledQueue := make([]mediaprovider.MediaItem, 0, p.getPlayQueueLength()-len(idxs))
		p.RemoveTracksFromQueueByIdx(idxs, &newShuffledQueue, &newNowPlaying, &isPlayingTrackRemoved, &isNextPlayingTrackremoved)
		p.setShuffledPlayQueue(newShuffledQueue)
	} else {
		newPlayQueue := make([]mediaprovider.MediaItem, 0, p.getPlayQueueLength()-len(idxs))
		p.RemoveTracksFromQueueByIdx(idxs, &newPlayQueue, &newNowPlaying, &isPlayingTrackRemoved, &isNextPlayingTrackremoved)
		p.setPlayQueue(newPlayQueue)
	}

	p.nowPlayingIdx = newNowPlaying
	if isPlayingTrackRemoved {
		if newNowPlaying == p.getPlayQueueLength() {
			// we had been playing the last track, and removed it
			p.Stop()
		} else {
			p.nowPlayingIdx -= 1 // will be incremented in newtrack callback from player
			p.setTrack(newNowPlaying, false, 0)
		}
		// setNextTrack and onSongChange callbacks will be handled
		// when we receive new track event from player
	} else if isNextPlayingTrackremoved {
		if newNowPlaying < p.getPlayQueueLength()-1 {
			p.handleNextTrackUpdated()
		} else {
			// no next track to play
			p.setNextTrack(-1)
		}
	}

	p.invokeNoArgCallbacks(p.onQueueChange)
}

func (p *playbackEngine) RemoveTracksFromQueueByIdx(idxs []int, newQueue *[]mediaprovider.MediaItem, newNowPlaying *int, isPlayingTrackRemoved *bool, isNextPlayingTrackRemoved *bool) {
	idxSet := sharedutil.ToSet(idxs)
	nowPlaying := p.NowPlayingIndex()

	for i, tr := range p.getActivePlayQueue() {
		if _, ok := idxSet[i]; ok {
			if i < nowPlaying {
				// if removing a track earlier than the currently playing one (if any),
				// decrement new now playing index by one to account for new position in queue
				*newNowPlaying--
			} else if i == nowPlaying {
				*isPlayingTrackRemoved = true
				// If we are removing the currently playing track, we need to scrobble it
				p.checkScrobble()
				p.alreadyScrobbled = true
			} else if nowPlaying >= 0 && i == nowPlaying+1 {
				*isNextPlayingTrackRemoved = true
			}
		} else {
			// not removing this track
			*newQueue = append(*newQueue, tr)
		}
	}
}

func (p *playbackEngine) GetTrackIdsFromIdx(idx []int) []string {
	ids := make([]string, 0, len(idx))
	for _, v := range idx {
		ids = append(ids, p.getPlayQueueItemAt(v).Metadata().ID)
	}
	return ids
}

func (p *playbackEngine) SetReplayGainOptions(config ReplayGainConfig) {
	rGainPlayer, ok := p.player.(player.ReplayGainPlayer)
	if !ok {
		log.Println("Error: player doesn't support ReplayGain")
		return
	}

	p.replayGainCfg = config
	mode := player.ReplayGainNone
	switch config.Mode {
	case ReplayGainAuto:
		mode = player.ReplayGainTrack
	case ReplayGainTrack:
		mode = player.ReplayGainTrack
	case ReplayGainAlbum:
		mode = player.ReplayGainAlbum
	}

	rGainPlayer.SetReplayGainOptions(player.ReplayGainOptions{
		Mode:            mode,
		PreventClipping: config.PreventClipping,
		PreampGain:      config.PreampGainDB,
	})
}

func (p *playbackEngine) SetReplayGainMode(mode player.ReplayGainMode) {
	rGainPlayer, ok := p.player.(player.ReplayGainPlayer)
	if !ok {
		log.Println("Error: player doesn't support ReplayGain")
		return
	}
	rGainPlayer.SetReplayGainOptions(player.ReplayGainOptions{
		PreventClipping: p.replayGainCfg.PreventClipping,
		PreampGain:      p.replayGainCfg.PreampGainDB,
		Mode:            mode,
	})
}

func (p *playbackEngine) cacheNextTracks() {
	if p.audiocache != nil {
		// fetch up to the 2 next tracks in the queue to the cache
		fetch := make([]AudioCacheRequest, 0, 3)
		// if nothing is playing (index = -1), treat the beginning of the queue as
		// the "currently" playing track, since we're probably about to play it
		npI := max(p.nowPlayingIdx, 0)
		for _, idx := range [3]int{npI, npI + 1, npI + 2} {
			if idx > 0 && idx < p.getPlayQueueLength() {
				item := p.getPlayQueueItemAt(idx)
				if item.Metadata().Type == mediaprovider.MediaItemTypeTrack {
					fetch = append(fetch, AudioCacheRequest{
						ID:          p.getPlayQueueItemAt(idx).Metadata().ID,
						DownloadURL: p.getMediaURLForIdx(idx),
					})
				}
			}
		}
		id := ""
		if np := p.NowPlaying(); np != nil {
			id = np.Metadata().ID
		}
		p.audiocache.CacheOnly(id, fetch)
	}
}

func (p *playbackEngine) handleOnTrackChange() {
	// scrobble the previous song if needed
	if !p.alreadyScrobbled {
		p.checkScrobble()
	}

	if p.PlaybackStatus().State == player.Playing {
		p.playTimeStopwatch.Start()
	}
	if p.pendingTrackChangeNum < 0 && (p.wasStopped || p.loopMode != LoopOne) {
		p.nowPlayingIdx++
		if p.loopMode == LoopAll && p.nowPlayingIdx == p.getPlayQueueLength() {
			p.nowPlayingIdx = 0 // wrapped around
		}
	} else if p.pendingTrackChangeNum >= 0 {
		p.nowPlayingIdx = p.pendingTrackChangeNum
		p.pendingTrackChangeNum = -1
	}
	nowPlaying := p.getPlayQueueItemAt(p.nowPlayingIdx)
	_, isRadio := nowPlaying.(*mediaprovider.RadioStation)
	p.isRadio = isRadio

	// reset flags
	p.wasStopped = false
	p.alreadyScrobbled = false

	p.curTrackDuration = nowPlaying.Metadata().Duration.Seconds()
	p.sendNowPlayingScrobble() // Must come before invokeOnChangeCallbacks b/c track may immediately be scrobbled
	p.invokeOnSongChangeCallbacks()
	p.handleTimePosUpdate(false)
	p.handleNextTrackUpdated()

	if p.pauseAfterCurrent {
		p.Pause()
		p.SetPauseAfterCurrent(false)
	}

}

func (p *playbackEngine) handleOnStopped() {
	p.playTimeStopwatch.Stop()
	if !p.alreadyScrobbled {
		p.checkScrobble()
	}
	p.stopPollTimePos()
	p.handleTimePosUpdate(false)
	p.invokeOnSongChangeCallbacks()
	p.invokeNoArgCallbacks(p.onStopped)
	p.reportPlayback("stopped")
	p.alreadyScrobbled = false
	p.wasStopped = true
	p.nowPlayingIdx = -1
	p.pauseAfterCurrent = false
}

// to be invoked as soon as the next item in the queue that should play changes
func (p *playbackEngine) handleNextTrackUpdated() {
	p.cacheNextTracks()
	p.needToSetNextTrack = true
	for _, cb := range p.onBeforeSongChange {
		var item mediaprovider.MediaItem
		if idx := p.nextPlayingIndex(); idx >= 0 {
			item = p.getPlayQueueItemAt(idx)
		}
		cb(item)
	}
}

func (p *playbackEngine) nextPlayingIndex() int {
	switch p.loopMode {
	case LoopNone:
		if p.nowPlayingIdx >= p.getPlayQueueLength()-1 {
			return -1
		}
		return p.nowPlayingIdx + 1
	case LoopOne:
		return p.nowPlayingIdx
	case LoopAll:
		if p.nowPlayingIdx >= p.getPlayQueueLength()-1 {
			return 0
		}
		return p.nowPlayingIdx + 1
	}
	return -1 // unreached
}

func (p *playbackEngine) setTrack(idx int, next bool, startTime float64) error {
	var item mediaprovider.MediaItem
	var url string
	if idx >= 0 {
		item = p.getPlayQueueItemAt(idx)
		url = p.getMediaURLForIdx(idx)
	}
	track, isTrack := item.(*mediaprovider.Track)
	if p.audiocache != nil && isTrack {
		p.audiocache.CacheFile(item.Metadata().ID, p.getMediaURLForIdx(idx))
	}

	if urlP, ok := p.player.(player.URLPlayer); ok {
		var meta mediaprovider.MediaItemMetadata
		if idx >= 0 {
			meta = item.Metadata()
			if isTrack && p.audiocache != nil {
				if filepath := p.audiocache.PathForCachedFile(track.ID); filepath != "" {
					url = filepath
				}
			}
			if lpP, ok := p.player.(player.LocalPlayer); ok && !isTrack {
				lpP.ObserveIcyRadioTitle(func(icytitle string) {
					var title, artist string
					if s := strings.Split(icytitle, " - "); len(s) == 2 {
						title, artist = s[1], s[0]
					} else {
						title = icytitle
					}
					for _, cb := range p.onRadioMetadataChange {
						cb(meta.Name, title, artist)
					}
				})
			} else if ok {
				lpP.UnobserveIcyRadioTitle()
			}
			if url == "" {
				return errors.New("no stream URL")
			}
		}
		if next {
			return urlP.SetNextFile(url, meta)
		}
		return urlP.PlayFile(url, meta, startTime)
	} else if trP, ok := p.player.(player.TrackPlayer); ok {
		var track *mediaprovider.Track
		if idx >= 0 {
			track, ok = p.getPlayQueueItemAt(idx).(*mediaprovider.Track)
			if !ok {
				return errors.New("cannot play non-Track media item with TrackPlayer")
			}
		}
		if next {
			return trP.SetNextTrack(track)
		}
		return trP.PlayTrack(track, startTime)
	}
	panic("Unsupported player type")
}

func (p *playbackEngine) getMediaURLForIdx(idx int) string {
	var url string
	item := p.getPlayQueueItemAt(idx)
	if tr, ok := item.(*mediaprovider.Track); ok {
		var ts *mediaprovider.TranscodeSettings
		if p.transcodeCfg.RequestTranscode {
			ts = &mediaprovider.TranscodeSettings{
				Codec:       p.transcodeCfg.Codec,
				BitRateKBPS: p.transcodeCfg.MaxBitRateKBPS,
			}
		}
		url, _ = p.sm.Server.GetStreamURL(tr.ID, ts, p.transcodeCfg.ForceRawFile)
	} else {
		url = item.(*mediaprovider.RadioStation).StreamURL
	}
	return url
}

func (p *playbackEngine) setNextTrack(idx int) error {
	return p.setTrack(idx, true, 0)
}

// call BEFORE updating p.nowPlayingIdx
func (p *playbackEngine) checkScrobble() {
	if !p.scrobbleCfg.Enabled || p.getPlayQueueLength() == 0 || p.nowPlayingIdx < 0 {
		return
	}
	track, ok := p.getPlayQueueItemAt(p.nowPlayingIdx).(*mediaprovider.Track)
	if !ok {
		return // radio stations are not scrobbled
	}

	playDur := p.playTimeStopwatch.Elapsed()
	if playDur.Seconds() < 0.1 || p.curTrackDuration < 0.1 {
		return
	}
	pcnt := playDur.Seconds() / p.curTrackDuration * 100
	timeThresholdMet := p.scrobbleCfg.ThresholdTimeSeconds >= 0 &&
		playDur.Seconds() >= float64(p.scrobbleCfg.ThresholdTimeSeconds)

	var submission bool
	server := p.sm.Server
	if server.ClientDecidesScrobble() && (timeThresholdMet || pcnt >= float64(p.scrobbleCfg.ThresholdPercent)) {
		track.PlayCount += 1
		p.lastScrobbled = track
		submission = true
	}
	go server.TrackEndedPlayback(track.ID, int(p.latestTrackPosition), submission)
	p.latestTrackPosition = 0
	p.playTimeStopwatch.Reset()
}

func (p *playbackEngine) sendNowPlayingScrobble() {
	if !p.scrobbleCfg.Enabled || p.getPlayQueueLength() == 0 || p.nowPlayingIdx < 0 {
		return
	}
	track, ok := p.getPlayQueueItemAt(p.nowPlayingIdx).(*mediaprovider.Track)
	if !ok {
		return // radio stations are not scrobbled
	}

	server := p.sm.Server
	if !server.ClientDecidesScrobble() {
		// server will count track as scrobbled as soon as it starts playing
		p.lastScrobbled = track
		track.PlayCount += 1
	}
	go p.sm.Server.TrackBeganPlayback(track.ID)
	p.reportPlayback("starting")
}

// creates a deep copy of the track info so that we can maintain our own state
// (play count increases, favorite, and rating) without messing up other views' track models
func deepCopyMediaItemSlice(tracks []mediaprovider.MediaItem) []mediaprovider.MediaItem {
	newTracks := make([]mediaprovider.MediaItem, len(tracks))
	for i, tr := range tracks {
		newTracks[i] = tr.Copy()
	}
	return newTracks
}

func (p *playbackEngine) invokeOnSongChangeCallbacks() {
	if p.callbacksDisabled {
		return
	}
	for _, cb := range p.onSongChange {
		cb(p.NowPlaying(), p.lastScrobbled)
	}
	p.lastScrobbled = nil
}

func (p *playbackEngine) reportPlayback(state string) {
	reporter, ok := p.sm.Server.(mediaprovider.CanReportPlayback)
	if !ok {
		return
	}
	if p.getPlayQueueLength() == 0 || p.nowPlayingIdx < 0 {
		return
	}
	np := p.NowPlaying()
	if np == nil || np.Metadata().Type != mediaprovider.MediaItemTypeTrack {
		return
	}
	posMs := int64(p.latestTrackPosition * 1000)
	go reporter.ReportPlayback(np.Metadata().ID, posMs, state)
}

func (pm *playbackEngine) invokeNoArgCallbacks(cbs []func()) {
	if pm.callbacksDisabled {
		return
	}
	for _, cb := range cbs {
		cb()
	}
}

func (p *playbackEngine) startPollTimePos() {
	if p.cancelPollPos != nil {
		return // already polling
	}

	ctx, cancel := context.WithCancel(p.ctx)
	p.cancelPollPos = cancel
	pollFrequency := 250 * time.Millisecond
	if p.playbackCfg.UseWaveformSeekbar {
		pollFrequency = 100 * time.Millisecond
	}
	pollingTick := time.NewTicker(pollFrequency)

	go func() {
		for {
			select {
			case <-ctx.Done():
				pollingTick.Stop()
				return
			case <-pollingTick.C:
				p.handleTimePosUpdate(false)
			}
		}
	}()
}

func (p *playbackEngine) stopPollTimePos() {
	if p.cancelPollPos != nil {
		p.cancelPollPos()
		p.cancelPollPos = nil
	}
}

func (p *playbackEngine) handleTimePosUpdate(seeked bool) {
	s := p.PlaybackStatus()
	var meta mediaprovider.MediaItemMetadata
	if np := p.NowPlaying(); np != nil {
		meta = np.Metadata()
	}
	isNearEnd := meta.Type != mediaprovider.MediaItemTypeRadioStation && s.TimePos > meta.Duration.Seconds()-10
	if p.needToSetNextTrack && isNearEnd {
		p.needToSetNextTrack = false
		if nextIdx := p.nextPlayingIndex(); nextIdx >= 0 && nextIdx < len(p.playQueue) {
			p.setNextTrack(nextIdx)
		} else {
			// no next track to play, ensure player knows this
			p.setNextTrack(-1)
		}
	}
	if p.callbacksDisabled {
		return
	}
	if s.TimePos > p.latestTrackPosition {
		p.latestTrackPosition = s.TimePos
	}
	duration := s.Duration
	if p.isRadio {
		// MPV reports buffered duration - we don't want to show this
		duration = 0
	}
	for _, cb := range p.onPlayTimeUpdate {
		cb(s.TimePos, duration, seeked)
	}
}
