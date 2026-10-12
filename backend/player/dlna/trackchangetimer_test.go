package dlna

import (
	"sync"
	"testing"
	"time"
)

// TestTrackChangeTimer_ConcurrentReset reproduces the scenario that used to
// deadlock: many goroutines calling Reset(0) (cancel, e.g. from Stop/Pause)
// concurrently with others calling Reset(nonzero) (re-arm, e.g. from a
// delayed background position sync). With the old unbuffered-channel-based
// implementation, a Reset(0) landing first could cause the timer's
// dispatcher goroutine to exit while a concurrent Reset(nonzero) call was
// still blocked sending to it, hanging that caller (and anything serialized
// behind it) forever.
func TestTrackChangeTimer_ConcurrentReset(t *testing.T) {
	timer := newTrackChangeTimer(func() {})
	timer.Reset(50 * time.Millisecond)

	const n = 100
	var wg sync.WaitGroup
	wg.Add(2 * n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			timer.Reset(0)
		}()
		go func() {
			defer wg.Done()
			timer.Reset(10 * time.Millisecond)
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// all Reset calls returned - no deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("Reset calls did not return in time - possible deadlock")
	}
}

// TestTrackChangeTimer_FiresAndCancels checks the basic contract: a
// scheduled fire actually invokes the callback, and Reset(0) suppresses a
// fire that hasn't happened yet.
func TestTrackChangeTimer_FiresAndCancels(t *testing.T) {
	fired := make(chan struct{}, 1)
	timer := newTrackChangeTimer(func() {
		select {
		case fired <- struct{}{}:
		default:
		}
	})

	timer.Reset(20 * time.Millisecond)
	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("callback did not fire")
	}

	timer.Reset(50 * time.Millisecond)
	timer.Reset(0)
	select {
	case <-fired:
		t.Fatal("callback fired after being cancelled")
	case <-time.After(150 * time.Millisecond):
		// expected: no fire
	}
}
