package common

import (
	"sync"
	"time"
)

// TrackChangeTimer is a resettable one-shot timer used by remote player
// implementations (DLNAPlayer, JukeboxPlayer) to estimate when the current
// track will end, since neither DLNA nor the Subsonic jukebox API pushes
// playback events to the client.
//
// Reset is safe to call concurrently from multiple goroutines (e.g. a
// user-initiated Stop() racing a delayed background position-sync), which a
// previous implementation based on an unbuffered channel handoff was not:
// two concurrent Reset calls could race such that one delivers a "cancel"
// message that causes the timer's dispatcher goroutine to exit while a
// second, already in-flight send is still waiting for a receiver, blocking
// that caller (and anything serialized behind it, such as the playback
// command queue) forever.
type TrackChangeTimer struct {
	mu    sync.Mutex
	timer *time.Timer

	onHandleTrackChange func()
}

func NewTrackChangeTimer(onHandleTrackChange func()) TrackChangeTimer {
	return TrackChangeTimer{onHandleTrackChange: onHandleTrackChange}
}

// Reset (re)arms the timer to fire onHandleTrackChange after dur, replacing
// any previously scheduled fire. A dur of exactly 0 cancels any pending fire
// without scheduling a new one; a negative dur fires (almost) immediately,
// same as a plain time.Timer.
func (d *TrackChangeTimer) Reset(dur time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	if dur == 0 {
		return
	}
	d.timer = time.AfterFunc(dur, d.onHandleTrackChange)
}
