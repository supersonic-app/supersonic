package dlna

import (
	"sync"
	"time"
)

// trackChangeTimer is a resettable one-shot timer used to estimate when the
// current track will end, since DLNA/UPnP has no push mechanism for
// transport-state changes.
//
// Reset is safe to call concurrently from multiple goroutines (e.g. a
// user-initiated Stop() racing the delayed background position sync kicked
// off after PlayFile/SeekSeconds/handleOnTrackChange), which the previous
// implementation based on an unbuffered channel handoff to a dispatcher
// goroutine was not: two concurrent Reset calls could race such that one
// delivers a "cancel" that causes the dispatcher to exit while the other,
// already in-flight send, is still waiting for a receiver - blocking that
// caller (and anything serialized behind it, such as the playback command
// queue) forever.
type trackChangeTimer struct {
	mu    sync.Mutex
	timer *time.Timer

	onFire func()
}

func newTrackChangeTimer(onFire func()) *trackChangeTimer {
	return &trackChangeTimer{onFire: onFire}
}

// Reset (re)arms the timer to fire onFire after dur, replacing any
// previously scheduled fire. A dur of exactly 0 cancels any pending fire
// without scheduling a new one.
func (t *trackChangeTimer) Reset(dur time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	if dur == 0 {
		return
	}
	t.timer = time.AfterFunc(dur, t.onFire)
}
