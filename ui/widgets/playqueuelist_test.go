package widgets

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
)

func testQueue(n int) []mediaprovider.MediaItem {
	items := make([]mediaprovider.MediaItem, n)
	for i := range items {
		items[i] = &mediaprovider.Track{ID: string(rune('a' + i))}
	}
	return items
}

func ids(items []mediaprovider.MediaItem) []string {
	s := make([]string, len(items))
	for i, it := range items {
		s[i] = it.Metadata().ID
	}
	return s
}

func newTestPlayQueueList(t *testing.T) *PlayQueueList {
	t.Helper()
	app := test.NewTempApp(t)
	// the app theme, so that the widgets' custom theme sizes resolve
	app.Settings().SetTheme(myTheme.NewMyTheme(&backend.ThemeConfig{}, t.TempDir()))
	return NewPlayQueueList(nil, false)
}

func TestPlayQueueListHidePlayed(t *testing.T) {
	p := newTestPlayQueueList(t)
	queue := testQueue(10)

	// with the filter off, everything is displayed and indexes pass through
	p.SetQueue(queue, 5, false)
	if got := p.lenTracks(); got != 10 {
		t.Errorf("displayed items = %d, want 10", got)
	}
	if got := p.queueIdxOffset(); got != 0 {
		t.Errorf("offset = %d, want 0", got)
	}

	// with it on, the 5 already-played items are hidden
	p.SetQueue(queue, 5, true)
	if got := p.lenTracks(); got != 5 {
		t.Errorf("displayed items = %d, want 5", got)
	}
	if got := p.queueIdxOffset(); got != 5 {
		t.Errorf("offset = %d, want 5", got)
	}

	// Queue() must still report the FULL queue, since that is what the
	// indexes handed to OnReorderItems/OnRemoveFromQueue index into
	if got := ids(p.Queue()); len(got) != 10 {
		t.Errorf("Queue() = %v, want all 10 items", got)
	}
}

func TestPlayQueueListSelectedIdxsAreQueueIdxs(t *testing.T) {
	p := newTestPlayQueueList(t)
	p.SetQueue(testQueue(10), 5, true)

	// select the first and last visible rows (display 0 and 4)
	p.selectAddOrRemove(0)
	p.selectAddOrRemove(4)

	idxs, offset := p.selectedQueueIdxs()
	if offset != 5 {
		t.Fatalf("offset = %d, want 5", offset)
	}
	// must be translated into full-queue indexes 5 and 9, not 0 and 4
	want := map[int]bool{5: true, 9: true}
	if len(idxs) != 2 {
		t.Fatalf("selectedQueueIdxs = %v, want 2 indexes", idxs)
	}
	for _, i := range idxs {
		if !want[i] {
			t.Errorf("got queue index %d, want one of 5, 9", i)
		}
	}
	// every reported index must be in range for the queue it indexes
	for _, i := range idxs {
		if i < 0 || i >= len(p.Queue()) {
			t.Errorf("queue index %d out of range for queue of %d", i, len(p.Queue()))
		}
	}
}

func TestPlayQueueListNowPlayingIndexKeepsSelection(t *testing.T) {
	p := newTestPlayQueueList(t)
	queue := testQueue(10)
	p.SetQueue(queue, 5, false)
	p.selectTrack(3)

	// with the filter off, a track change must not rebuild the items and so
	// must not drop the selection
	p.SetNowPlayingIndex(6, false)
	if idxs, _ := p.selectedQueueIdxs(); len(idxs) != 1 || idxs[0] != 3 {
		t.Errorf("selection = %v, want [3] to survive the track change", idxs)
	}

	// with the filter on, the displayed set changes, so a rebuild is expected
	p.SetQueue(queue, 5, true)
	if got := p.queueIdxOffset(); got != 5 {
		t.Fatalf("offset = %d, want 5", got)
	}
	p.SetNowPlayingIndex(6, true)
	if got := p.queueIdxOffset(); got != 6 {
		t.Errorf("offset = %d, want 6 after advancing a track", got)
	}
	if got := p.lenTracks(); got != 4 {
		t.Errorf("displayed items = %d, want 4", got)
	}
}

func TestPlayQueueListHidePlayedEdgeCases(t *testing.T) {
	p := newTestPlayQueueList(t)
	queue := testQueue(3)

	// nothing playing: nothing is hidden
	p.SetQueue(queue, -1, true)
	if got := p.queueIdxOffset(); got != 0 {
		t.Errorf("offset with no now playing = %d, want 0", got)
	}
	if got := p.lenTracks(); got != 3 {
		t.Errorf("displayed items = %d, want 3", got)
	}

	// playing the first track: nothing has been played yet
	p.SetQueue(queue, 0, true)
	if got := p.queueIdxOffset(); got != 0 {
		t.Errorf("offset on first track = %d, want 0", got)
	}

	// index past the end of the queue must not slice out of range
	p.SetQueue(queue, 99, true)
	if got := p.queueIdxOffset(); got != 0 {
		t.Errorf("offset for out-of-range index = %d, want 0", got)
	}
	if got := p.lenTracks(); got != 3 {
		t.Errorf("displayed items = %d, want 3", got)
	}

	// empty queue
	p.SetQueue(nil, 0, true)
	if got := p.lenTracks(); got != 0 {
		t.Errorf("displayed items = %d, want 0", got)
	}
}

// the displayed track numbers must stay the tracks' positions in the full
// queue, rather than restarting from 1 once the played items are hidden
func TestPlayQueueListDisplayTrackNum(t *testing.T) {
	p := newTestPlayQueueList(t)
	queue := testQueue(10)

	p.SetQueue(queue, 5, false)
	if got := p.displayTrackNum(0); got != 1 {
		t.Errorf("first row numbered %d, want 1", got)
	}
	if got := p.displayTrackNum(9); got != 10 {
		t.Errorf("last row numbered %d, want 10", got)
	}

	p.SetQueue(queue, 5, true)
	if got := p.displayTrackNum(0); got != 6 {
		t.Errorf("first visible row numbered %d, want 6", got)
	}
	if got := p.displayTrackNum(4); got != 10 {
		t.Errorf("last visible row numbered %d, want 10", got)
	}
}

func TestPlayQueueListPlayTrackAtUsesQueueIndex(t *testing.T) {
	p := newTestPlayQueueList(t)
	got := -1
	p.OnPlayItemAt = func(idx int) { got = idx }

	p.SetQueue(testQueue(10), 5, true)
	p.onPlayTrackAt(0)
	if got != 5 {
		t.Errorf("playing first visible row reported index %d, want 5", got)
	}
	p.onPlayTrackAt(4)
	if got != 9 {
		t.Errorf("playing last visible row reported index %d, want 9", got)
	}

	// with the filter off the display index is already the queue index
	p.SetQueue(testQueue(10), 5, false)
	p.onPlayTrackAt(0)
	if got != 0 {
		t.Errorf("with filter off, first row reported index %d, want 0", got)
	}
}

// the indexes and insert position the list reports must compose with Queue()
// into a correct reordering of the FULL queue - pairing them with the displayed
// items instead panics and drops the hidden tracks
func TestPlayQueueListReorderComposesWithQueue(t *testing.T) {
	p := newTestPlayQueueList(t)
	p.Reorderable = true
	p.SetQueue(testQueue(10), 5, true) // display f..j, offset 5

	var gotIdxs []int
	var gotInsertPos int
	p.OnReorderItems = func(idxs []int, insertPos int) {
		gotIdxs, gotInsertPos = idxs, insertPos
	}

	// drag the last visible row (display 4 = queue 9 = "j") to the top of the
	// visible list (display insert position 0)
	p.selectAddOrRemove(4)
	p.list.OnDragEnd(4, 0)

	if gotIdxs == nil {
		t.Fatal("OnReorderItems was not called")
	}
	// this is what ConnectPlayQueuelistActions does with the reported values
	newQueue := ids(sharedutil.ReorderItems(p.Queue(), gotIdxs, gotInsertPos))

	want := []string{"a", "b", "c", "d", "e", "j", "f", "g", "h", "i"}
	if len(newQueue) != len(want) {
		t.Fatalf("reordered queue has %d tracks, want %d (hidden tracks must not be dropped)",
			len(newQueue), len(want))
	}
	for i := range want {
		if newQueue[i] != want[i] {
			t.Fatalf("reordered queue = %v, want %v", newQueue, want)
		}
	}
}

func TestPlayQueueListSetHidePlayed(t *testing.T) {
	p := newTestPlayQueueList(t)
	p.SetQueue(testQueue(10), 4, false)
	if got := p.lenTracks(); got != 10 {
		t.Fatalf("displayed items = %d, want 10", got)
	}

	p.SetHidePlayed(true)
	if got := p.lenTracks(); got != 6 {
		t.Errorf("displayed items = %d, want 6 after hiding", got)
	}
	if got := p.queueIdxOffset(); got != 4 {
		t.Errorf("offset = %d, want 4", got)
	}

	p.SetHidePlayed(false)
	if got := p.lenTracks(); got != 10 {
		t.Errorf("displayed items = %d, want 10 after unhiding", got)
	}
	if got := p.queueIdxOffset(); got != 0 {
		t.Errorf("offset = %d, want 0", got)
	}
}
