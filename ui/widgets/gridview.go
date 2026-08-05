package widgets

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2/lang"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	gridViewUpdateCounter = util.NewEventCounter(70)

	emptyItem = GridViewItemModel{
		ID:        "dummy",
		Name:      "—",
		Secondary: []string{"—"},
		Suffix:    "—",
	}
)

const batchFetchSize = 6

type BatchingIterator[M any] struct {
	iter mediaprovider.MediaIterator[M]
}

func NewBatchingIterator[M any](iter mediaprovider.MediaIterator[M]) BatchingIterator[M] {
	return BatchingIterator[M]{iter}
}

func (b *BatchingIterator[M]) NextN(n int) []*M {
	results := make([]*M, 0, n)
	i := 0
	for i < n {
		value := b.iter.Next()
		if value == nil {
			break
		}
		results = append(results, value)
		i++
	}
	return results
}

type GridViewIterator interface {
	NextN(int) []GridViewItemModel
}

type gridViewAlbumIterator struct {
	iter BatchingIterator[mediaprovider.Album]
}

func (g gridViewAlbumIterator) NextN(n int) []GridViewItemModel {
	albums := g.iter.NextN(n)
	return sharedutil.MapSlice(albums, func(al *mediaprovider.Album) GridViewItemModel {
		model := GridViewItemModel{
			Name:         al.Name,
			ID:           al.ID,
			CoverArtID:   al.CoverArtID,
			Secondary:    al.ArtistNames,
			SecondaryIDs: al.ArtistIDs,
			CanFavorite:  true,
			IsFavorite:   al.Favorite,
		}
		if y := al.Date.Year; y != nil {
			model.Suffix = strconv.Itoa(*al.Date.Year)
		}
		return model
	})
}

func NewGridViewAlbumIterator(iter mediaprovider.AlbumIterator) GridViewIterator {
	return gridViewAlbumIterator{iter: NewBatchingIterator(iter)}
}

type gridViewArtistIterator struct {
	iter BatchingIterator[mediaprovider.Artist]
}

func (g gridViewArtistIterator) NextN(n int) []GridViewItemModel {
	artists := g.iter.NextN(n)
	return sharedutil.MapSlice(artists, func(ar *mediaprovider.Artist) GridViewItemModel {
		albumsLabel := lang.L("albums")
		if ar.AlbumCount == 1 {
			albumsLabel = lang.L("album")
		}
		fallbackAlbumsMsg := fmt.Sprintf("%d %s", ar.AlbumCount, albumsLabel)
		albumsMsg := lang.LocalizePluralKey("{{.albumsCount}} albums",
			fallbackAlbumsMsg, ar.AlbumCount, map[string]string{"albumsCount": strconv.Itoa(ar.AlbumCount)})
		return GridViewItemModel{
			Name:        ar.Name,
			ID:          ar.ID,
			CoverArtID:  ar.CoverArtID,
			Secondary:   []string{albumsMsg},
			CanFavorite: true,
			IsFavorite:  ar.Favorite,
		}
	})
}

func NewGridViewArtistIterator(iter mediaprovider.ArtistIterator) GridViewIterator {
	return gridViewArtistIterator{iter: NewBatchingIterator(iter)}
}

type GridView struct {
	widget.BaseWidget

	ShowSuffix bool

	stateMutex  sync.RWMutex
	fetchCancel context.CancelFunc
	GridViewState

	grid               *disabledGridWrap
	loadingDots        *LoadingDots
	menu               *widget.PopUpMenu
	menuGridViewItemId string
	itemForIndex       map[int]*GridViewItem
	itemWidth          float32
	numColsCached      int
	shareMenuItem      *fyne.MenuItem
}

type GridViewState struct {
	items        []GridViewItemModel
	iter         GridViewIterator
	imageFetcher util.ImageFetcher
	Placeholder  fyne.Resource
	highestShown int
	done         bool

	DisableSharing bool

	OnPlay              func(id string, shuffle bool)
	OnPlayNext          func(id string)
	OnAddToQueue        func(id string)
	OnAddToPlaylist     func(id string)
	OnFavorite          func(id string, fav bool)
	OnDownload          func(id string)
	OnShare             func(id string)
	OnShowItemPage      func(id string)
	OnShowSecondaryPage func(id string)

	scrollPos float32
}

var _ fyne.Widget = (*GridView)(nil)

func newGridView() *GridView {
	g := &GridView{
		loadingDots:  NewLoadingDots(),
		itemWidth:    NewGridViewItem(nil).MinSize().Width,
		itemForIndex: make(map[int]*GridViewItem),
	}
	return g
}

func NewFixedGridView(items []GridViewItemModel, fetch util.ImageFetcher, placeholder fyne.Resource) *GridView {
	g := newGridView()
	g.GridViewState = GridViewState{
		items:        items,
		done:         true,
		imageFetcher: fetch,
		Placeholder:  placeholder,
	}
	g.ExtendBaseWidget(g)
	g.createGridWrap()
	return g
}

func NewGridView(iter GridViewIterator, fetch util.ImageFetcher, placeholder fyne.Resource) *GridView {
	g := newGridView()
	g.GridViewState = GridViewState{
		iter:         iter,
		imageFetcher: fetch,
		Placeholder:  placeholder,
	}
	g.ExtendBaseWidget(g)
	g.createGridWrap()
	g.loadingDots.Start()

	// fetch initial items
	g.checkFetchMoreItems(36)
	return g
}

// Items returns the items that are currently loaded into the GridView state.
func (g *GridView) Items() []GridViewItemModel {
	g.stateMutex.Lock()
	defer g.stateMutex.Unlock()

	items := make([]GridViewItemModel, len(g.items))
	copy(items, g.items)
	return items
}

func (g *GridView) SaveToState() *GridViewState {
	g.stateMutex.RLock()
	s := g.GridViewState
	g.stateMutex.RUnlock()
	s.scrollPos = g.grid.GetScrollOffset()
	return &s
}

func NewGridViewFromState(state *GridViewState) *GridView {
	g := newGridView()
	g.GridViewState = *state
	g.ExtendBaseWidget(g)
	g.createGridWrap()
	g.Refresh() // needed to initialize the widget
	g.grid.ScrollToOffset(state.scrollPos)
	return g
}

func (g *GridView) Clear() {
	g.stateMutex.Lock()
	defer g.stateMutex.Unlock()
	g.cancelFetch()
	g.items = nil
	g.done = true
}

func (g *GridView) Reset(iter GridViewIterator) {
	g.stateMutex.Lock()
	g.cancelFetch()
	g.items = nil
	g.itemForIndex = make(map[int]*GridViewItem)
	g.done = false
	g.highestShown = 0
	g.iter = iter
	g.stateMutex.Unlock()
	g.checkFetchMoreItems(36)
	g.loadingDots.Start()
	g.Refresh()
}

func (g *GridView) ResetFromState(state *GridViewState) {
	g.stateMutex.Lock()
	g.cancelFetch()
	g.GridViewState = *state
	g.itemForIndex = make(map[int]*GridViewItem)
	g.stateMutex.Unlock()
	g.grid.Refresh()
	g.grid.ScrollToOffset(state.scrollPos)
}

func (g *GridView) ResetFixed(items []GridViewItemModel) {
	g.stateMutex.Lock()
	g.cancelFetch()
	g.items = items
	g.itemForIndex = make(map[int]*GridViewItem)
	g.done = true
	g.highestShown = 0
	g.iter = nil
	g.stateMutex.Unlock()
	g.Refresh()
}

func (g *GridView) GetScrollOffset() float32 {
	return g.grid.GetScrollOffset()
}

func (g *GridView) ScrollToOffset(offs float32) {
	g.grid.ScrollToOffset(offs)
}

func (g *GridView) Resize(size fyne.Size) {
	g.numColsCached = -1
	g.BaseWidget.Resize(size)
}

var _ fyne.Tappable = (*GridView)(nil)

func (g *GridView) Tapped(*fyne.PointEvent) {
	if c := fyne.CurrentApp().Driver().CanvasForObject(g); c != nil {
		c.Unfocus()
	}
}

func (g *GridView) createGridWrap() {
	g.grid = NewDisabledGridWrap(
		g.lenItems,
		g.createNewItemCard,
		// update func
		func(itemID widget.GridWrapItemID, obj fyne.CanvasObject) {
			ac := obj.(*GridViewItem)
			ac.SetSize(backend.AppInstance().Config.GridView.CardSize)
			g.doUpdateItemCard(int(itemID), ac)
		},
	)
}

func (g *GridView) createNewItemCard() fyne.CanvasObject {
	card := NewGridViewItem(g.Placeholder)
	card.SetSize(backend.AppInstance().Config.GridView.CardSize)
	card.ItemIndex = -1
	card.ImgLoader = util.NewThumbnailLoader(g.imageFetcher, card.Cover.SetImage)
	card.ImgLoader.OnBeforeLoad = func() { card.Cover.SetImage(nil) }
	card.OnPlay = func() { g.onPlay(card.ItemID(), false) }
	card.OnFavorite = func(fav bool) {
		if g.OnFavorite != nil {
			g.OnFavorite(card.itemID, fav)
		}
	}
	card.OnShowSecondaryPage = func(id string) {
		if g.OnShowSecondaryPage != nil {
			g.OnShowSecondaryPage(id)
		}
	}
	card.OnShowItemPage = func() {
		if g.OnShowItemPage != nil {
			g.OnShowItemPage(card.ItemID())
		}
	}
	card.OnShowContextMenu = func(p fyne.Position) {
		g.showContextMenu(card, p)
	}
	card.OnFocusNeighbor = func(neighbor int) {
		focusIndex := -1
		switch neighbor {
		case 0: // left
			focusIndex = card.ItemIndex - 1
		case 1: // right
			focusIndex = card.ItemIndex + 1
		case 2: // up
			focusIndex = card.ItemIndex - g.grid.ColumnCount()
		case 3: // down
			focusIndex = card.ItemIndex + g.grid.ColumnCount()
		}
		if focusIndex >= 0 && focusIndex < g.lenItems() {
			g.grid.ScrollTo(focusIndex)
			g.stateMutex.RLock()
			if item, ok := g.itemForIndex[focusIndex]; ok {
				if c := fyne.CurrentApp().Driver().CanvasForObject(g); c != nil {
					c.Focus(item)
				}
			}
			g.stateMutex.RUnlock()
		}
	}
	return card
}

func (g *GridView) doUpdateItemCard(itemIdx int, card *GridViewItem) {
	if itemIdx > g.highestShown {
		g.highestShown = itemIdx
	}
	var item GridViewItemModel
	g.stateMutex.Lock()
	// itemIdx can rarely be out of range if the data is being updated
	// as the view is requested to refresh
	if itemIdx < len(g.items) {
		item = g.items[itemIdx]
	}
	// update itemForIndex map
	if c, ok := g.itemForIndex[card.ItemIndex]; ok && c == card {
		delete(g.itemForIndex, card.ItemIndex)
	}
	card.ItemIndex = itemIdx
	g.itemForIndex[itemIdx] = card
	g.stateMutex.Unlock()

	card.ShowSuffix = g.ShowSuffix
	card.Cover.Im.PlaceholderIcon = g.Placeholder
	if !card.NeedsUpdate(&item) {
		// nothing to do
		return
	}

	if gridViewUpdateCounter.NumEventsSince(time.Now().Add(-150*time.Millisecond)) > 64 {
		if card.itemID != emptyItem.ID {
			card.Update(&emptyItem)
			card.ImgLoader.Load("")
		}
		if card.NextUpdateModel == nil {
			// queue to run later
			go func() {
				<-time.After(10 * time.Millisecond)
				fyne.Do(func() {
					if card.NextUpdateModel != nil {
						gridViewUpdateCounter.Add()
						card.Update(card.NextUpdateModel)
						card.ImgLoader.Load(card.NextUpdateModel.CoverArtID)
					}
					card.NextUpdateModel = nil
				})
			}()
		}
		card.NextUpdateModel = &item
	} else {
		card.NextUpdateModel = nil
		gridViewUpdateCounter.Add()
		card.Update(&item)
		card.ImgLoader.Load(item.CoverArtID)
	}

	// if user has scrolled near the bottom, fetch more
	if itemIdx > g.lenItems()-10 {
		g.checkFetchMoreItems(20)
	}
}

func (g *GridView) lenItems() int {
	g.stateMutex.RLock()
	defer g.stateMutex.RUnlock()
	return len(g.items)
}

// fetches at least count more items if fetch not in progress and not done
// acquires stateMutex for atomicity
func (g *GridView) checkFetchMoreItems(count int) {
	g.stateMutex.Lock()
	defer g.stateMutex.Unlock()
	if g.done || g.fetchCancel != nil {
		return // done, or fetch already in progress
	}
	if g.iter == nil {
		g.done = true
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	g.fetchCancel = cancel
	go func() {
		// keep repeating the fetch task as long as the user
		// has scrolled near the bottom
		for !g.done && g.highestShown >= g.lenItems()-10 {
			n := 0
			for !g.done && n < count {
				items := g.iter.NextN(batchFetchSize)
				select {
				case <-ctx.Done():
					return
				default:
					g.stateMutex.Lock()
					g.items = append(g.items, items...)
					g.stateMutex.Unlock()
					if len(items) < batchFetchSize {
						g.done = true
					}
					n += len(items)
					fyne.DoAndWait(func() {
						g.loadingDots.Stop()
						if len(items) > 0 {
							g.grid.Refresh()
						}
					})
				}
			}
		}
		// call cancelfunc to release Context resources
		g.stateMutex.Lock()
		g.cancelFetch()
		g.stateMutex.Unlock()
	}()
}

// must be called with stateMutex locked for writing
func (g *GridView) cancelFetch() {
	if g.fetchCancel != nil {
		g.fetchCancel()
		g.fetchCancel = nil
		g.loadingDots.Stop()
	}
}

func (g *GridView) showContextMenu(card *GridViewItem, pos fyne.Position) {
	g.menuGridViewItemId = card.ItemID()
	if g.menu == nil {
		play := fyne.NewMenuItem(lang.L("Play"), func() { g.onPlay(g.menuGridViewItemId, false) })
		play.Icon = theme.MediaPlayIcon()
		shuffle := fyne.NewMenuItem(lang.L("Shuffle"), func() { g.onPlay(g.menuGridViewItemId, true) })
		shuffle.Icon = myTheme.ShuffleIcon
		queueNext := fyne.NewMenuItem(lang.L("Play next"), func() {
			if g.OnPlayNext != nil {
				g.OnPlayNext(g.menuGridViewItemId)
			}
		})
		queueNext.Icon = myTheme.PlayNextIcon
		queue := fyne.NewMenuItem(lang.L("Add to queue"), func() {
			if g.OnAddToQueue != nil {
				g.OnAddToQueue(g.menuGridViewItemId)
			}
		})
		queue.Icon = theme.ContentAddIcon()
		playlist := fyne.NewMenuItem(lang.L("Add to playlist")+"...", func() {
			if g.OnAddToPlaylist != nil {
				g.OnAddToPlaylist(g.menuGridViewItemId)
			}
		})
		playlist.Icon = myTheme.PlaylistIcon
		download := fyne.NewMenuItem(lang.L("Download")+"...", func() {
			if g.OnDownload != nil {
				g.OnDownload(g.menuGridViewItemId)
			}
		})
		download.Icon = theme.DownloadIcon()
		g.shareMenuItem = fyne.NewMenuItem(lang.L("Share")+"...", func() {
			g.OnShare(g.menuGridViewItemId)
		})
		g.shareMenuItem.Icon = myTheme.ShareIcon
		g.menu = widget.NewPopUpMenu(fyne.NewMenu("", play, shuffle, queueNext, queue, playlist, download, g.shareMenuItem),
			fyne.CurrentApp().Driver().CanvasForObject(g))
	}
	g.shareMenuItem.Disabled = g.DisableSharing
	g.menu.ShowAtPosition(pos)
}

func (g *GridView) onPlay(itemID string, shuffle bool) {
	if g.OnPlay != nil {
		g.OnPlay(itemID, shuffle)
	}
}

func (g *GridView) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(
		g.grid, container.NewCenter(g.loadingDots),
	))
}

// a disabled widget is not considered focusable by the focus manager
type disabledGridWrap struct {
	widget.GridWrap
}

func NewDisabledGridWrap(len func() int, create func() fyne.CanvasObject, update func(widget.GridWrapItemID, fyne.CanvasObject)) *disabledGridWrap {
	g := &disabledGridWrap{
		GridWrap: widget.GridWrap{
			Length:     len,
			CreateItem: create,
			UpdateItem: update,
		},
	}
	g.ExtendBaseWidget(g)
	return g
}

var _ fyne.Disableable = (*disabledGridWrap)(nil)

func (g *disabledGridWrap) Disabled() bool { return true }

func (g *disabledGridWrap) Disable() {}

func (g *disabledGridWrap) Enable() {}
