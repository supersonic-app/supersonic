package widgets

import (
	"sync/atomic"

	"fyne.io/fyne/v2"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

// Component that manages lazily loading more tracks into a Tracklist
// as the user scrolls near the bottom.
type TracklistLoader struct {
	disposed atomic.Bool

	tracklist *Tracklist
	iter      mediaprovider.TrackIterator

	trackBuffer  []*mediaprovider.Track
	fetching     bool
	done         bool
	len          int
	highestShown int
}

func NewTracklistLoader(tracklist *Tracklist, iter mediaprovider.TrackIterator) *TracklistLoader {
	t := TracklistLoader{
		tracklist: tracklist,
		iter:      iter,
	}
	t.tracklist.OnTrackShown = t.onTrackShown
	t.fetching = true
	t.tracklist.SetLoading(true)
	go t.loadMoreTracks(25)
	return &t
}

// Cancels all asynchronous loads so that they will no longer modify the tracklist.
func (t *TracklistLoader) Dispose() {
	t.tracklist.SetLoading(false)
	t.disposed.Store(true)
	t.tracklist.OnTrackShown = nil
}

func (t *TracklistLoader) onTrackShown(tracknum int) {
	if tracknum > t.highestShown {
		t.highestShown = tracknum
	}
	if t.highestShown >= t.len-25 && !t.fetching && !t.done && !t.disposed.Load() {
		t.fetching = true
		go t.loadMoreTracks(25)
	}
}

func (t *TracklistLoader) loadMoreTracks(num int) {
	// repeat fetch task as long as user has scrolled near bottom
	for !t.done && t.highestShown >= t.len-25 {
		if t.trackBuffer == nil {
			t.trackBuffer = make([]*mediaprovider.Track, 0, num)
		}
		t.trackBuffer = t.trackBuffer[:0]
		for range num {
			tr := t.iter.Next()
			if tr == nil {
				t.done = true
				break
			}
			t.trackBuffer = append(t.trackBuffer, tr)
			if t.disposed.Load() {
				break
			}
		}
		if t.disposed.Load() {
			return
		}
		fyne.DoAndWait(func() {
			t.tracklist.SetLoading(false)
			t.tracklist.AppendTracks(t.trackBuffer)
			t.len += len(t.trackBuffer)
		})
	}
	if t.done {
		t.trackBuffer = nil
	}
	t.fetching = false
}
