package browsing

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/ui/controller"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
)

type Page interface {
	fyne.CanvasObject

	Save() SavedPage
	Reload()
	Route() controller.Route
}

type SavedPage interface {
	Restore() Page
}

// Searchable pages should implement this interface so their search bar can be focused by keyboard shortcut.
type Searchable interface {
	SearchWidget() fyne.Focusable
}

// Pages with selection should implement this interface to receive Ctrl+A events
type CanSelectAll interface {
	SelectAll()
	UnselectAll()
}

// Pages that have one main scrollable view should implement this interface
// to receive callbacks from window-level keyboard scrolling (up/down)
type Scrollable interface {
	Scroll(amount float32)
}

type CanShowNowPlaying interface {
	OnSongChange(playing mediaprovider.MediaItem, lastScrobbledIfAny *mediaprovider.Track)
}

type CanShowPlayTime interface {
	OnPlayTimeUpdate(curTime, totalTime float64, seeked bool)
}

type CanShowPlayQueue interface {
	OnPlayQueueChange()
}

type BrowsingPane struct {
	widget.BaseWidget

	OnHistoryChanged func()

	playbackManager *backend.PlaybackManager

	curPage    Page
	history    []SavedPage
	historyIdx int

	pageContainer *fyne.Container
}

func NewBrowsingPane(app *backend.PlaybackManager, contr *controller.Controller, onGoHome func()) *BrowsingPane {
	b := &BrowsingPane{playbackManager: app}
	b.ExtendBaseWidget(b)
	b.playbackManager.OnSongChange(b.onSongChange)
	b.playbackManager.OnPlayTimeUpdate(b.onPlayTimeUpdate)
	b.playbackManager.OnQueueChange(b.onQueueChange)
	bkgrnd := myTheme.NewThemedRectangle(myTheme.ColorNamePageBackground)
	b.pageContainer = container.NewStack(bkgrnd, layout.NewSpacer())
	return b
}

func (b *BrowsingPane) SetPage(p Page) {
	if p == nil {
		// special case to set a "blank page"
		// only used on logout, in conjunction with clearing the history
		b.pageContainer.Objects[1] = layout.NewSpacer()
		b.curPage = nil
		b.pageContainer.Refresh()
	} else {
		oldPage := b.curPage
		if b.doSetPage(p) && oldPage != nil {
			b.addPageToHistory(oldPage, true)
		}
	}
	b.onHistoryChanged()
}

func (b *BrowsingPane) ClearHistory() {
	b.history = nil
	b.historyIdx = 0
	b.onHistoryChanged()
}

func (b *BrowsingPane) GetSearchBarIfAny() fyne.Focusable {
	if s, ok := b.curPage.(Searchable); ok {
		return s.SearchWidget()
	}
	return nil
}

func (b *BrowsingPane) SelectAll() {
	if s, ok := b.curPage.(CanSelectAll); ok {
		s.SelectAll()
	}
}

func (b *BrowsingPane) UnselectAll() {
	if s, ok := b.curPage.(CanSelectAll); ok {
		s.UnselectAll()
	}
}

func (b *BrowsingPane) ScrollUp() {
	b.scrollBy(-75)
}

func (b *BrowsingPane) ScrollDown() {
	b.scrollBy(75)
}

func (b *BrowsingPane) PageUp() {
	b.scrollBy(-b.Size().Height * 0.9)
}

func (b *BrowsingPane) PageDown() {
	b.scrollBy(b.Size().Height * 0.9)
}

func (b *BrowsingPane) scrollBy(increment float32) {
	if s, ok := b.curPage.(Scrollable); ok {
		s.Scroll(increment)
	}
}

func (b *BrowsingPane) RefreshPage() {
	if b.curPage != nil {
		b.curPage.Refresh()
	}
}

func (b *BrowsingPane) doSetPage(p Page) bool {
	if b.curPage != nil && b.curPage.Route() == p.Route() {
		return false
	}
	// TODO: reset focus only if something inside the previous page had focus
	if c := fyne.CurrentApp().Driver().CanvasForObject(b); c != nil {
		c.Unfocus()
	}
	b.curPage = p
	if np, ok := p.(CanShowNowPlaying); ok {
		// inform page of currently playing track
		np.OnSongChange(b.playbackManager.NowPlaying(), nil)
	}
	b.pageContainer.Remove(b.curPage)
	b.pageContainer.Objects[1] = p
	b.Refresh()
	return true
}

func (b *BrowsingPane) onHistoryChanged() {
	if b.OnHistoryChanged != nil {
		b.OnHistoryChanged()
	}
}

func (b *BrowsingPane) onSongChange(song mediaprovider.MediaItem, lastScrobbledIfAny *mediaprovider.Track) {
	fyne.Do(func() {
		if b.curPage == nil {
			return
		}
		if p, ok := b.curPage.(CanShowNowPlaying); ok {
			p.OnSongChange(song, lastScrobbledIfAny)
		}
	})
}

func (b *BrowsingPane) onPlayTimeUpdate(cur, total float64, seeked bool) {
	fyne.Do(func() {
		if b.curPage == nil {
			return
		}
		if p, ok := b.curPage.(CanShowPlayTime); ok {
			p.OnPlayTimeUpdate(cur, total, seeked)
		}
	})
}

func (b *BrowsingPane) onQueueChange() {
	fyne.Do(b.RefreshPlayQueue)
}

// RefreshPlayQueue notifies the current page, if it displays the play queue,
// that the queue it is showing needs to be refreshed. Must be called on the
// Fyne main goroutine.
func (b *BrowsingPane) RefreshPlayQueue() {
	if b.curPage == nil {
		return
	}
	if p, ok := b.curPage.(CanShowPlayQueue); ok {
		p.OnPlayQueueChange()
	}
}

func (b *BrowsingPane) addPageToHistory(p Page, truncate bool) {
	if truncate {
		// allow garbage collection of pages that will be removed from the history
		for i := b.historyIdx; i < len(b.history); i++ {
			b.history[i] = nil
		}
		b.history = b.history[:b.historyIdx]
	}
	if b.historyIdx < len(b.history) {
		b.history[b.historyIdx] = p.Save()
	} else {
		b.history = append(b.history, p.Save())
	}
	b.historyIdx++
}

func (b *BrowsingPane) CanGoBack() bool {
	return b.historyIdx > 0
}

func (b *BrowsingPane) CanGoForward() bool {
	return b.historyIdx < len(b.history)-1
}

func (b *BrowsingPane) GoBack() {
	if b.historyIdx > 0 {
		// due to widget reuse between pages,
		// we must create the new page before calling addPageToHistory
		p := b.history[b.historyIdx-1].Restore()
		b.addPageToHistory(b.curPage, false)
		b.historyIdx -= 2
		b.doSetPage(p)
		b.onHistoryChanged()
	}
}

func (b *BrowsingPane) GoForward() {
	if b.historyIdx < len(b.history)-1 {
		p := b.history[b.historyIdx+1].Restore()
		b.addPageToHistory(b.curPage, false)
		b.doSetPage(p)
		b.onHistoryChanged()
	}
}

func (b *BrowsingPane) Reload() {
	if b.curPage != nil {
		b.curPage.Reload()
	}
}

func (b *BrowsingPane) CurrentPage() controller.Route {
	if b.curPage == nil {
		return controller.Route{Page: controller.None}
	}
	return b.curPage.Route()
}

func (b *BrowsingPane) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(b.pageContainer)
}
