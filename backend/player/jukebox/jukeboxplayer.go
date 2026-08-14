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

	// index into the server-side jukebox queue of the currently playing
	// track. Since the server doesn't push playback events, this is only
	// ever updated from a JukeboxGetStatus response (see
	// reconcileWithStatus) - never assumed/incremented locally except right
	// after PlayTrack, where we know it's 0 because JukeboxSet just cleared
	// the queue.
	curTrack         int
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

// GetVolume queries the server for the jukebox's actual current volume,
// rather than relying purely on the last value set through this player
// instance. This matters right after switching to the Jukebox player: with a
// freshly constructed JukeboxPlayer, j.volume defaults to its zero value, and
// PlaybackManager reads GetVolume() immediately to decide whether to fire a
// volume-changed callback (see playbackEngine.SetPlayer) - without querying
// the server, that would always report 0 and yank the volume slider down
// regardless of the jukebox's actual (possibly nonzero) volume.
func (j *JukeboxPlayer) GetVolume() int {
	if j.destroyed {
		return j.volume
	}
	if stat, err := j.provider.JukeboxGetStatus(); err == nil {
		j.volume = stat.Volume
	}
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

	// Reconcile against the server's authoritative queue position first.
	// This is called once per track, close to its end (see
	// playbackEngine.handleTimePosUpdate's isNearEnd check), which is also
	// exactly when our own track-change-timer estimate is most likely to
	// still be stale relative to the server's true state (e.g. our
	// estimate of the current track's duration ran a bit long). If we
	// computed the index to remove/add from a stale j.curTrack, we could
	// end up operating on the currently-playing entry instead of a
	// genuinely queued-but-unplayed one - which, depending on how the
	// server's jukebox reacts to removing its active track, can disrupt
	// playback (observed: it fell back to replaying the previous track).
	if stat, err := j.provider.JukeboxGetStatus(); err == nil && stat.Playing {
		j.reconcileWithStatus(stat)
	}

	if j.hasNextTrack {
		if err := j.provider.JukeboxRemove(j.curTrack + 1); err != nil {
			return err
		}
		j.hasNextTrack = false
	}

	if track == nil {
		return nil
	}

	// append the new track to the queue
	if err := j.provider.JukeboxAdd(track.ID); err != nil {
		return err
	}
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

	j.reconcileWithStatus(stat)
}

// reconcileWithStatus updates local track-position state to match an
// authoritative JukeboxGetStatus response. If the server has moved on to a
// different track than we last knew, this performs the same bookkeeping as
// a natural track change and notifies the engine via InvokeOnTrackChange -
// so regardless of which poll (the track-change timer firing, scheduleSync,
// or SetNextTrack's own pre-check) is the one to first observe the server
// having advanced, the engine always finds out about it the same way.
// Otherwise, just resyncs the current track's position (see
// resyncFromStatus).
func (j *JukeboxPlayer) reconcileWithStatus(stat *mediaprovider.JukeboxStatus) {
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
