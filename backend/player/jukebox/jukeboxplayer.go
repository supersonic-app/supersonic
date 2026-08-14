package jukebox

import (
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

type JukeboxPlayer struct {
	player.BasePlayerCallbackImpl

	provider mediaprovider.JukeboxProvider

	state   int // stopped, playing, paused
	volume  int
	seeking bool

	// start playback position in seconds of the last seek/time sync
	lastStartTime int
	// how long the track has been playing since last time sync
	stopwatch util.Stopwatch

	trackChangeTimer common.TrackChangeTimer

	curTrack         int
	queueLength      int
	curTrackDuration float64
}

func NewJukeboxPlayer(provider mediaprovider.JukeboxProvider) (*JukeboxPlayer, error) {
	j := &JukeboxPlayer{provider: provider}
	j.trackChangeTimer = common.NewTrackChangeTimer(j.handleOnTrackChange)
	return j, nil
}

func (j *JukeboxPlayer) SetVolume(vol int) error {
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
	if j.state == playing {
		return nil
	}
	if err := j.provider.JukeboxStart(); err != nil {
		return err
	}

	j.state = playing
	j.stopwatch.Start()
	j.InvokeOnPlaying()
	return nil
}

func (j *JukeboxPlayer) Pause() error {
	if j.state != playing {
		return nil
	}
	if err := j.provider.JukeboxStop(); err != nil {
		return err
	}
	j.stopwatch.Stop()
	j.state = paused
	j.InvokeOnPaused()
	return nil
}

func (j *JukeboxPlayer) Stop(_ bool) error {
	if j.state == stopped {
		return nil
	}
	if err := j.provider.JukeboxStop(); err != nil {
		return err
	}
	j.state = stopped
	j.lastStartTime = 0
	j.stopwatch.Reset()
	j.InvokeOnStopped()
	return nil
}

func (j *JukeboxPlayer) PlayTrack(track *mediaprovider.Track, startTime float64) error {
	if err := j.provider.JukeboxSet(track.ID); err != nil {
		return err
	}
	if err := j.provider.JukeboxStart(); err != nil {
		return err
	}

	j.curTrack = 0
	j.queueLength = 1
	j.curTrackDuration = track.Duration.Seconds()

	if startTime > 0 {
		if err := j.provider.JukeboxSeek(j.curTrack, int(startTime)); err != nil {
			return err
		}
	}
	j.lastStartTime = int(startTime)
	j.stopwatch.Reset()
	j.stopwatch.Start()
	j.state = playing
	j.InvokeOnPlaying()

	return nil
}

func (j *JukeboxPlayer) SetNextTrack(track *mediaprovider.Track) error {
	// we need to replace the last track in the queue, remove it first
	if j.curTrack < j.queueLength-1 {
		if err := j.provider.JukeboxRemove(j.curTrack + 1); err != nil {
			return err
		}
		j.queueLength -= 1
	}
	// append the new track to the queue
	if err := j.provider.JukeboxAdd(track.ID); err != nil {
		return err
	}
	j.queueLength += 1
	return nil
}

func (j *JukeboxPlayer) SeekSeconds(secs float64) error {
	j.seeking = true
	err := j.provider.JukeboxSeek(j.curTrack, int(secs))
	j.seeking = false
	j.InvokeOnSeek()
	return err
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

func (j *JukeboxPlayer) Destroy() {}

func (j *JukeboxPlayer) handleOnTrackChange() {
	stopping := false
	// TODO: is playing next track?

	if stopping {
		j.lastStartTime = 0
		j.stopwatch.Reset()
	} else {
		j.lastStartTime = 0
		j.stopwatch.Reset()
		j.stopwatch.Start()
		j.trackChangeTimer.Reset(0 /*TODO: next track time*/)
		j.InvokeOnTrackChange()
	}
}
