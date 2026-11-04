package widgets

import (
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
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

// setQueue sets the queue with the hide-played filter on or off
func setQueue(p *PlayQueueList, queue []mediaprovider.MediaItem, nowPlayingIdx int, hidePlayed bool) {
	p.SetHidePlayed(hidePlayed)
	p.SetQueue(queue, nowPlayingIdx)
}

func TestPlayQueueListHidePlayed(t *testing.T) {
	p := newTestPlayQueueList(t)
	queue := testQueue(10)

	// with the filter off, everything is displayed and indexes pass through
	setQueue(p, queue, 5, false)
	if got := p.lenTracks(); got != 10 {
		t.Errorf("displayed items = %d, want 10", got)
	}
	if got := p.hiddenItemCount(); got != 0 {
		t.Errorf("hidden = %d, want 0", got)
	}

	// with it on, the 5 already-played items are hidden
	setQueue(p, queue, 5, true)
	if got := p.lenTracks(); got != 5 {
		t.Errorf("displayed items = %d, want 5", got)
	}
	if got := p.hiddenItemCount(); got != 5 {
		t.Errorf("hidden = %d, want 5", got)
	}

	// Queue() must still report the FULL queue, since that is what the
	// indexes handed to OnReorderItems/OnRemoveFromQueue index into
	if got := ids(p.Queue()); len(got) != 10 {
		t.Errorf("Queue() = %v, want all 10 items", got)
	}
}

func TestPlayQueueListSelectedIdxsAreQueueIdxs(t *testing.T) {
	p := newTestPlayQueueList(t)
	setQueue(p, testQueue(10), 5, true)

	// select the first and last visible rows (display 0 and 4)
	p.selectAddOrRemove(0)
	p.selectAddOrRemove(4)

	// must be translated into full-queue indexes 5 and 9, not 0 and 4
	if idxs := p.selectedQueueIdxs(); !slices.Equal(idxs, []int{5, 9}) {
		t.Errorf("selectedQueueIdxs = %v, want [5 9]", idxs)
	}
}

func TestPlayQueueListTrackChangeKeepsSelection(t *testing.T) {
	for _, hide := range []bool{false, true} {
		p := newTestPlayQueueList(t)
		setQueue(p, testQueue(10), 5, hide)
		p.selectAddOrRemove(3) // queue 3 with the filter off, queue 8 with it on
		want := p.selectedQueueIdxs()

		p.SetNowPlayingIndex(6)
		if idxs := p.selectedQueueIdxs(); !slices.Equal(idxs, want) {
			t.Errorf("hide=%v: selection = %v, want %v to survive the track change", hide, idxs, want)
		}
	}
}

func TestPlayQueueListHiddenItemsAreUnselected(t *testing.T) {
	p := newTestPlayQueueList(t)
	setQueue(p, testQueue(10), 5, true)
	p.selectAddOrRemove(0) // queue 5, the playing track

	p.SetNowPlayingIndex(6) // queue 5 is now hidden
	p.SetHidePlayed(false)
	if idxs := p.selectedQueueIdxs(); len(idxs) != 0 {
		t.Errorf("selection = %v, want the hidden item to have been unselected", idxs)
	}
}

func TestPlayQueueListHidePlayedEdgeCases(t *testing.T) {
	p := newTestPlayQueueList(t)
	queue := testQueue(3)

	// nothing playing: nothing is hidden
	setQueue(p, queue, -1, true)
	if got := p.hiddenItemCount(); got != 0 {
		t.Errorf("hidden with no now playing = %d, want 0", got)
	}
	if got := p.lenTracks(); got != 3 {
		t.Errorf("displayed items = %d, want 3", got)
	}

	// playing the first track: nothing has been played yet
	p.SetQueue(queue, 0)
	if got := p.hiddenItemCount(); got != 0 {
		t.Errorf("hidden on first track = %d, want 0", got)
	}

	// index past the end of the queue must not slice out of range
	p.SetQueue(queue, 99)
	if got := p.hiddenItemCount(); got != 0 {
		t.Errorf("hidden for out-of-range index = %d, want 0", got)
	}
	if got := p.lenTracks(); got != 3 {
		t.Errorf("displayed items = %d, want 3", got)
	}

	// empty queue
	p.SetQueue(nil, 0)
	if got := p.lenTracks(); got != 0 {
		t.Errorf("displayed items = %d, want 0", got)
	}
}

// the displayed track numbers must stay the tracks' positions in the full
// queue, rather than restarting from 1 once the played items are hidden
func TestPlayQueueListDisplayTrackNum(t *testing.T) {
	p := newTestPlayQueueList(t)
	rowNum := func(itemID int) string {
		row := p.list.CreateItem()
		p.list.UpdateItem(itemID, row)
		return row.(*PlayQueueListRow).num.Text
	}

	setQueue(p, testQueue(10), 5, false)
	if got := rowNum(0); got != "1" {
		t.Errorf("first row numbered %s, want 1", got)
	}

	p.SetHidePlayed(true)
	if got := rowNum(0); got != "6" {
		t.Errorf("first visible row numbered %s, want 6", got)
	}
	if got := rowNum(4); got != "10" {
		t.Errorf("last visible row numbered %s, want 10", got)
	}
}

func TestPlayQueueListPlayTrackAtUsesQueueIndex(t *testing.T) {
	p := newTestPlayQueueList(t)
	got := -1
	p.OnPlayItemAt = func(idx int) { got = idx }

	setQueue(p, testQueue(10), 5, true)
	p.onPlayTrackAt(4)
	if got != 9 {
		t.Errorf("playing last visible row reported index %d, want 9", got)
	}

	// with the filter off the display index is already the queue index
	p.SetHidePlayed(false)
	p.onPlayTrackAt(0)
	if got != 0 {
		t.Errorf("with filter off, first row reported index %d, want 0", got)
	}
}

// the indexes and insert position the list reports must compose with Queue()
// into a correct reordering of the FULL queue
func TestPlayQueueListReorder(t *testing.T) {
	for _, tt := range []struct {
		name          string
		hide          bool
		drag, dropPos int
		want          string
	}{
		// display f..j: a drop between rows lands at the same place in the queue
		{"hidden", true, 4, 2, "abcdefgjhi"},
		// a drop above the playing track would make the dragged track
		// "already played" and so vanish; it lands just after it instead
		{"hidden, above playing", true, 4, 0, "abcdefjghi"},
		// without hiding, dropping above the playing track is allowed
		{"shown, above playing", false, 9, 5, "abcdejfghi"},
	} {
		p := newTestPlayQueueList(t)
		p.Reorderable = true
		setQueue(p, testQueue(10), 5, tt.hide)

		var gotIdxs []int
		var gotInsertPos int
		p.OnReorderItems = func(idxs []int, insertPos int) {
			gotIdxs, gotInsertPos = idxs, insertPos
		}
		p.selectAddOrRemove(tt.drag)
		p.list.OnDragEnd(tt.drag, tt.dropPos)

		// this is what ConnectPlayQueuelistActions does with the reported values
		got := strings.Join(ids(sharedutil.ReorderItems(p.Queue(), gotIdxs, gotInsertPos)), "")
		if got != tt.want {
			t.Errorf("%s: reordered queue = %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestPlayQueueListSetHidePlayed(t *testing.T) {
	p := newTestPlayQueueList(t)
	setQueue(p, testQueue(10), 4, false)

	p.SetHidePlayed(true)
	if got := p.lenTracks(); got != 6 {
		t.Errorf("displayed items = %d, want 6 after hiding", got)
	}

	p.SetHidePlayed(false)
	if got := p.lenTracks(); got != 10 {
		t.Errorf("displayed items = %d, want 10 after unhiding", got)
	}
}

func TestPlayQueueListHidingKeepsScrollPosition(t *testing.T) {
	p := newTestPlayQueueList(t)
	w := test.NewWindow(p)
	defer w.Close()
	w.Resize(fyne.NewSize(400, 300))
	setQueue(p, testQueue(20), 5, true)
	pitch := p.rowPitch()

	// scrolled down: the same tracks stay in view as a played one is hidden
	p.list.ScrollToOffset(3 * pitch)
	p.SetNowPlayingIndex(6)
	if got := p.list.GetScrollOffset(); got != 2*pitch {
		t.Errorf("offset = %v, want %v", got, 2*pitch)
	}

	// at the top: stays at the top, showing the new playing track first
	p.list.ScrollToOffset(0)
	p.SetNowPlayingIndex(7)
	if got := p.list.GetScrollOffset(); got != 0 {
		t.Errorf("offset = %v, want 0", got)
	}

	// revealing the played tracks keeps the playing track at the top
	p.SetHidePlayed(false)
	if got := p.list.GetScrollOffset(); got != 7*pitch {
		t.Errorf("offset = %v, want %v", got, 7*pitch)
	}
}
