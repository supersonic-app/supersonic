package jukebox

import (
	"time"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/player"
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

	curTrack           int
	queueLength        int
	curTrackDuration   float64
	startTrackTime     float64
	startedAtUnixMilli int64
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
	if err := j.startAndUpdateTime(); err != nil {
		return err
	}

	j.state = playing
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
	// TODO: calculate paused at time
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
	j.InvokeOnStopped()
	return nil
}

func (j *JukeboxPlayer) PlayTrack(track *mediaprovider.Track, _ float64) error {
	if err := j.provider.JukeboxSet(track.ID); err != nil {
		return err
	}
	j.startTrackTime = 0
	if err := j.startAndUpdateTime(); err != nil {
		return err
	}

	j.curTrack = 0
	j.queueLength = 1
	j.curTrackDuration = track.Duration.Seconds()

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
	if j.state == playing {
		state = player.Playing
	} else if j.state == paused {
		state = player.Paused
	}

	// TODO - the rest

	return player.Status{
		State: state,
	}
}

func (j *JukeboxPlayer) Destroy() {}

func (j *JukeboxPlayer) startAndUpdateTime() error {
	beforeStart := time.Now()
	if err := j.provider.JukeboxStart(); err != nil {
		return err
	}
	afterStart := time.Now()

	// assume track started playing at (ie has been playing for)
	// half the round-trip latency
	j.startedAtUnixMilli = time.Now().Add(-afterStart.Sub(beforeStart)).UnixMilli()
	return nil
}
