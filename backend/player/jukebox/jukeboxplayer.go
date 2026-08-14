package jukebox

import (
	"log"
	"time"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/player"
	"github.com/supersonic-app/supersonic/backend/player/common"
	"github.com/supersonic-app/supersonic/backend/util"
)

const (
	stopped = 0
	playing = 1
	paused  = 2
)

// syncSettleDelay is how long to wait after issuing a command that changes
// server-side playback (start/seek) before polling JukeboxGetStatus to
// reconcile our locally-estimated position against the server's authoritative
// one. Mirrors the equivalent settle delays in DLNAPlayer.
const syncSettleDelay = 2 * time.Second

// retryDelay is how soon to retry after a failed JukeboxGetStatus call
// triggered by the track change timer firing.
const retryDelay = 2 * time.Second

type JukeboxPlayer struct {
	player.BasePlayerCallbackImpl

	provider  mediaprovider.JukeboxProvider
	destroyed bool

	state   int // stopped, playing, paused
	volume  int
	seeking bool

	// start playback position in seconds of the last seek/time sync
	lastStartTime int
	// how long the track has been playing since last time sync
	stopwatch util.Stopwatch

	trackChangeTimer common.TrackChangeTimer

	// index into the server-side jukebox queue of the currently playing track
	curTrack int
	// number of tracks currently in the server-side jukebox queue
	queueLength      int
	curTrackDuration float64

	// duration of the track queued via SetNextTrack, used to know the new
	// track's duration once the server reports it's started playing.
	// Only meaningful when hasNextTrack is true.
	hasNextTrack      bool
	nextTrackDuration float64
}

func NewJukeboxPlayer(provider mediaprovider.JukeboxProvider) (*JukeboxPlayer, error) {
	j := &JukeboxPlayer{provider: provider}
	j.trackChangeTimer = common.NewTrackChangeTimer(j.handleOnTrackChange)
	return j, nil
}

func (j *JukeboxPlayer) SetVolume(vol int) error {
	if j.destroyed {
		return nil
	}
	if err := j.provider.JukeboxSetVolume(vol); err != nil {
		return err
	}
	j.volume = vol
	return nil
}

func (j *JukeboxPlayer) GetVolume() int {
	return j.volume
}

func (j *JukeboxPlayer) Continue() error {
	if j.destroyed || j.state == playing {
		return nil
	}
	if err := j.provider.JukeboxStart(); err != nil {
		return err
	}

	j.state = playing
	j.stopwatch.Start()
	j.trackChangeTimer.Reset(j.remainingTrackTime())
	j.InvokeOnPlaying()

	go j.scheduleSync(syncSettleDelay)
	return nil
}

func (j *JukeboxPlayer) Pause() error {
	if j.destroyed || j.state != playing {
		return nil
	}
	if err := j.provider.JukeboxStop(); err != nil {
		return err
	}
	j.trackChangeTimer.Reset(0)
	j.stopwatch.Stop()
	j.state = paused
	j.InvokeOnPaused()
	return nil
}

func (j *JukeboxPlayer) Stop(_ bool) error {
	if j.destroyed || j.state == stopped {
		return nil
	}
	if err := j.provider.JukeboxStop(); err != nil {
		return err
	}
	j.trackChangeTimer.Reset(0)
	j.state = stopped
	j.lastStartTime = 0
	j.stopwatch.Reset()
	j.InvokeOnStopped()
	return nil
}

func (j *JukeboxPlayer) PlayTrack(track *mediaprovider.Track, startTime float64) error {
	if j.destroyed {
		return nil
	}
	if err := j.provider.JukeboxSet(track.ID); err != nil {
		return err
	}
	if err := j.provider.JukeboxStart(); err != nil {
		return err
	}

	j.curTrack = 0
	j.queueLength = 1
	j.curTrackDuration = track.Duration.Seconds()
	j.hasNextTrack = false

	if startTime > 0 {
		if err := j.provider.JukeboxSeek(j.curTrack, int(startTime)); err != nil {
			return err
		}
	}
	j.lastStartTime = int(startTime)
	j.stopwatch.Reset()
	j.stopwatch.Start()
	j.state = playing
	j.trackChangeTimer.Reset(j.remainingTrackTime())
	j.InvokeOnPlaying()
	// PlayTrack is called both for user-initiated track changes (e.g.
	// skip/select) and when the engine transfers an already-playing track
	// to this player (e.g. switching cast devices mid-song). Either way,
	// the engine only advances its own now-playing index/UI in response to
	// this callback - matches DLNAPlayer.PlayFile.
	j.InvokeOnTrackChange()
	if startTime > 0 {
		j.InvokeOnSeek()
	}

	go j.scheduleSync(syncSettleDelay)
	return nil
}

// SetNextTrack queues track to play after the current one, replacing any
// previously queued next track. A nil track clears the queued next track,
// without affecting the currently playing one.
func (j *JukeboxPlayer) SetNextTrack(track *mediaprovider.Track) error {
	if j.destroyed {
		return nil
	}

	// we need to replace the last track in the queue, remove it first
	if j.curTrack < j.queueLength-1 {
		if err := j.provider.JukeboxRemove(j.curTrack + 1); err != nil {
			return err
		}
		j.queueLength -= 1
		j.hasNextTrack = false
	}

	if track == nil {
		return nil
	}

	// append the new track to the queue
	if err := j.provider.JukeboxAdd(track.ID); err != nil {
		return err
	}
	j.queueLength += 1
	j.hasNextTrack = true
	j.nextTrackDuration = track.Duration.Seconds()
	return nil
}

func (j *JukeboxPlayer) SeekSeconds(secs float64) error {
	if j.destroyed {
		return nil
	}

	j.seeking = true
	err := j.provider.JukeboxSeek(j.curTrack, int(secs))
	j.seeking = false
	if err != nil {
		return err
	}

	j.lastStartTime = int(secs)
	j.stopwatch.Reset()
	if j.state == playing {
		j.stopwatch.Start()
	}
	j.trackChangeTimer.Reset(j.remainingTrackTime())
	j.InvokeOnSeek()

	go j.scheduleSync(syncSettleDelay)
	return nil
}

func (j *JukeboxPlayer) IsSeeking() bool {
	return j.seeking
}

func (j *JukeboxPlayer) GetStatus() player.Status {
	state := player.Stopped
	switch j.state {
	case playing:
		state = player.Playing
	case paused:
		state = player.Paused
	}

	return player.Status{
		State:    state,
		TimePos:  j.curPlayPos().Seconds(),
		Duration: j.curTrackDuration,
	}
}

func (j *JukeboxPlayer) curPlayPos() time.Duration {
	return time.Duration(j.lastStartTime)*time.Second + j.stopwatch.Elapsed()
}

// remainingTrackTime is how long until the current track is expected to end,
// based on our local bookkeeping of its duration and current position
// (including time elapsed since the last seek/time sync, not just the
// position as of then).
func (j *JukeboxPlayer) remainingTrackTime() time.Duration {
	return time.Duration(j.curTrackDuration*float64(time.Second)) - j.curPlayPos()
}

func (j *JukeboxPlayer) Destroy() {
	j.destroyed = true
	j.trackChangeTimer.Reset(0)
}

// handleOnTrackChange fires when the local trackChangeTimer estimates the
// current track has finished. Since the server-side jukebox doesn't push
// playback events, this always reconciles against the server's authoritative
// JukeboxGetStatus before updating any state.
func (j *JukeboxPlayer) handleOnTrackChange() {
	if j.destroyed {
		return
	}

	stat, err := j.provider.JukeboxGetStatus()
	if err != nil {
		log.Printf("jukebox: failed to get status: %v", err)
		if !j.destroyed {
			j.trackChangeTimer.Reset(retryDelay)
		}
		return
	}

	if !stat.Playing {
		// the server-side queue is exhausted (or was stopped by another
		// client) - nothing left to advance to
		j.state = stopped
		j.lastStartTime = 0
		j.stopwatch.Reset()
		j.InvokeOnStopped()
		return
	}

	if stat.CurrentTrack == j.curTrack {
		// our local timer fired early due to clock drift - the server
		// hasn't advanced tracks yet. Resync and re-arm for the
		// remaining time rather than treating this as a track change.
		j.resyncFromStatus(stat)
		return
	}

	// the server has advanced to the next track in the queue
	j.curTrack = stat.CurrentTrack
	if j.hasNextTrack {
		j.curTrackDuration = j.nextTrackDuration
		j.hasNextTrack = false
	}
	j.lastStartTime = int(stat.PositionSeconds)
	j.stopwatch.Reset()
	j.stopwatch.Start()
	j.trackChangeTimer.Reset(j.remainingTrackTime())
	j.InvokeOnTrackChange()
}

// scheduleSync waits the given delay and then reconciles local playback
// position/state against the server's authoritative JukeboxGetStatus. Used
// after commands (start/seek) that change server-side playback, to correct
// for network round-trip latency and any drift in the local estimate.
func (j *JukeboxPlayer) scheduleSync(delay time.Duration) {
	time.Sleep(delay)
	if j.destroyed {
		return
	}
	stat, err := j.provider.JukeboxGetStatus()
	if err != nil {
		log.Printf("jukebox: failed to sync status: %v", err)
		return
	}
	if j.destroyed || stat.CurrentTrack != j.curTrack {
		// player was destroyed, or the track has already changed again
		// (e.g. handleOnTrackChange already reconciled it) - stale reply
		return
	}
	j.resyncFromStatus(stat)
}

// resyncFromStatus reconciles local position tracking against an
// authoritative status response for the currently known track (does not
// handle the track having changed - see handleOnTrackChange for that).
func (j *JukeboxPlayer) resyncFromStatus(stat *mediaprovider.JukeboxStatus) {
	j.lastStartTime = int(stat.PositionSeconds)
	j.stopwatch.Reset()
	if j.state == playing {
		j.stopwatch.Start()
	}
	j.trackChangeTimer.Reset(j.remainingTrackTime())
	j.InvokeOnSeek()
}
