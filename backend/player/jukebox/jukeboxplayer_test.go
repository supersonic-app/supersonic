package jukebox

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

// fakeJukeboxProvider is a minimal, concurrency-safe stand-in for a real
// Subsonic server, used to exercise JukeboxPlayer from multiple goroutines
// at once under the race detector.
type fakeJukeboxProvider struct {
	mu       sync.Mutex
	queue    []string
	curTrack int
	playing  bool
}

func (f *fakeJukeboxProvider) JukeboxStart() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing = len(f.queue) > 0
	return nil
}

func (f *fakeJukeboxProvider) JukeboxStop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing = false
	return nil
}

func (f *fakeJukeboxProvider) JukeboxSeek(idx, seconds int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.curTrack = idx
	return nil
}

func (f *fakeJukeboxProvider) JukeboxClear() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = nil
	f.curTrack = 0
	f.playing = false
	return nil
}

func (f *fakeJukeboxProvider) JukeboxAdd(trackID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, trackID)
	return nil
}

func (f *fakeJukeboxProvider) JukeboxRemove(idx int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if idx < 0 || idx >= len(f.queue) {
		return errIndexOutOfRange
	}
	f.queue = append(f.queue[:idx], f.queue[idx+1:]...)
	return nil
}

func (f *fakeJukeboxProvider) JukeboxGetStatus() (*mediaprovider.JukeboxStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &mediaprovider.JukeboxStatus{
		CurrentTrack: f.curTrack,
		Playing:      f.playing,
	}, nil
}

func (f *fakeJukeboxProvider) JukeboxSet(trackID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = []string{trackID}
	f.curTrack = 0
	f.playing = true
	return nil
}

func (f *fakeJukeboxProvider) JukeboxSetVolume(vol int) error { return nil }
func (f *fakeJukeboxProvider) JukeboxSupported() bool         { return true }

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

const errIndexOutOfRange = fakeErr("index out of range")

// TestJukeboxPlayer_ConcurrentAccess drives PlayTrack, SetNextTrack,
// SeekSeconds and the internal track-change/status-sync goroutines from many
// goroutines at once, mirroring how playbackEngine actually calls
// JukeboxPlayer: PlayTrack/SeekSeconds from the serialized command queue,
// SetNextTrack from an independent poll ticker, and handleOnTrackChange from
// trackChangeTimer's own dispatcher goroutine. This is a regression test for
// a data race in JukeboxPlayer's unsynchronized state (curTrack,
// hasNextTrack, ...) that could let concurrent requests reach the server out
// of order and desync local bookkeeping from the server's actual queue.
func TestJukeboxPlayer_ConcurrentAccess(t *testing.T) {
	provider := &fakeJukeboxProvider{}
	j, err := NewJukeboxPlayer(provider)
	if err != nil {
		t.Fatalf("NewJukeboxPlayer: %v", err)
	}
	var trackChanges int64
	j.OnTrackChange(func() { atomic.AddInt64(&trackChanges, 1) })

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n * 3)
	for i := 0; i < n; i++ {
		track := &mediaprovider.Track{}
		track.ID = "track-a"
		track.Duration = 3 * time.Second

		next := &mediaprovider.Track{}
		next.ID = "track-b"
		next.Duration = 3 * time.Second

		go func() {
			defer wg.Done()
			_ = j.PlayTrack(track, 0)
		}()
		go func() {
			defer wg.Done()
			_ = j.SetNextTrack(next)
		}()
		go func() {
			defer wg.Done()
			_ = j.SeekSeconds(1)
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent calls did not return in time")
	}

	j.Destroy()
}
