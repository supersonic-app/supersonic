package browsing

import (
	"log"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/ui/controller"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
	"github.com/supersonic-app/supersonic/ui/widgets"
)

type albumsPageAdapter struct {
	cfg       *backend.AlbumsPageConfig
	contr     *controller.Controller
	mp        mediaprovider.MediaProvider
	pm        *backend.PlaybackManager
	filter    mediaprovider.AlbumFilter
	filterBtn *widgets.AlbumFilterButton

	// dependency injected from the GridViewPage
	itemsFn func() []widgets.GridViewItemModel
}

func NewAlbumsPage(cfg *backend.AlbumsPageConfig, pool *util.WidgetPool, contr *controller.Controller, pm *backend.PlaybackManager, mp mediaprovider.MediaProvider, im *backend.ImageManager) Page {
	adapter := &albumsPageAdapter{cfg: cfg, contr: contr, mp: mp, pm: pm}
	return NewGridViewPage(adapter, pool, mp, im)
}

var _ GridViewPageAdapterGetItems = (*albumsPageAdapter)(nil)

func (a *albumsPageAdapter) SetItemsFunc(f func() []widgets.GridViewItemModel) {
	a.itemsFn = f
}

func (a *albumsPageAdapter) Title() string { return lang.L("Albums") }

func (a *albumsPageAdapter) Filter() mediaprovider.AlbumFilter {
	if a.filter == nil {
		a.filter = mediaprovider.NewAlbumFilter(
			mediaprovider.AlbumFilterOptions{},
		)
	}
	return a.filter
}

func (a *albumsPageAdapter) FilterButton() widgets.FilterButton[mediaprovider.Album, mediaprovider.AlbumFilterOptions] {
	if a.filterBtn == nil {
		a.filterBtn = widgets.NewAlbumFilterButton(a.Filter(), a.mp.GetGenres)
	}
	return a.filterBtn
}

func (a *albumsPageAdapter) PlaceholderResource() fyne.Resource { return myTheme.AlbumIcon }

func (a *albumsPageAdapter) Route() controller.Route { return controller.AlbumsRoute() }

func (a *albumsPageAdapter) SortOrders() ([]string, int) {
	orders := a.mp.AlbumSortOrders()
	sortOrder := max(slices.Index(orders, a.cfg.SortOrder), 0)

	return util.LocalizeSlice(orders), sortOrder
}

func (a *albumsPageAdapter) SaveSortOrder(orderIdx int) {
	a.cfg.SortOrder = a.mp.AlbumSortOrders()[orderIdx]
}

func (a *albumsPageAdapter) ActionButton() fyne.CanvasObject {
	shuffleAlbumsFn := func() {
		go func() {
			if err := a.pm.PlayRandomAlbums(""); err != nil {
				log.Printf("error playing random albums: %v", err)
				fyne.Do(func() {
					a.contr.ToastProvider.ShowErrorToast(lang.L("Unable to play random albums"))
				})
			}
		}()
	}

	playAlbumsFn := func() {
		if a.itemsFn == nil {
			a.contr.ToastProvider.ShowErrorToast(lang.L("Unable to play albums"))
		}

		go func() {
			for i, item := range a.itemsFn() {
				if i >= 20 {
					break // don't load more than first 20 albums
				}
				if i == 0 {
					a.pm.LoadAlbum(item.ID, backend.Replace, false)
					a.pm.PlayFromBeginning()
				} else {
					a.pm.LoadAlbum(item.ID, backend.Append, false)
				}
			}
		}()
	}

	var btn *widgets.OptionButton
	var inOrder, shuffled *fyne.MenuItem
	inOrder = fyne.NewMenuItem(lang.L("In order"), func() {
		shuffled.Checked = false
		inOrder.Checked = true
		a.cfg.PlayInOrder = true
		btn.Text = lang.L("Play albums")
		btn.Icon = theme.MediaPlayIcon()
		btn.Refresh()
	})
	inOrder.Icon = myTheme.AlbumIcon
	shuffled = fyne.NewMenuItem(lang.L("Shuffled"), func() {
		inOrder.Checked = false
		shuffled.Checked = true
		a.cfg.PlayInOrder = false
		btn.Text = lang.L("Shuffle albums")
		btn.Icon = myTheme.ShuffleIcon
		btn.Refresh()
	})
	shuffled.Icon = myTheme.ShuffleIcon
	shuffled.Checked = !a.cfg.PlayInOrder
	inOrder.Checked = a.cfg.PlayInOrder

	menu := fyne.NewMenu("", inOrder, shuffled)
	icon := myTheme.ShuffleIcon
	textKey := "Shuffle albums"
	if a.cfg.PlayInOrder {
		textKey = "Play albums"
		icon = theme.MediaPlayIcon()
	}
	btn = widgets.NewOptionButton(lang.L(textKey), menu, func() {
		if a.cfg.PlayInOrder {
			playAlbumsFn()
		} else {
			shuffleAlbumsFn()
		}
	})
	btn.Icon = icon

	return btn
}

func (a *albumsPageAdapter) Iter(sortOrderIdx int, filter mediaprovider.AlbumFilter) widgets.GridViewIterator {
	sortOrder := a.mp.AlbumSortOrders()[sortOrderIdx]
	return widgets.NewGridViewAlbumIterator(a.mp.IterateAlbums(sortOrder, filter))
}

func (a *albumsPageAdapter) SearchIter(query string, filter mediaprovider.AlbumFilter) widgets.GridViewIterator {
	return widgets.NewGridViewAlbumIterator(a.mp.SearchAlbums(query, filter))
}

func (a *albumsPageAdapter) InitGrid(gv *widgets.GridView) {
	a.contr.ConnectAlbumGridActions(gv)
	gv.ShowSuffix = a.cfg.ShowYears
}

func (a *albumsPageAdapter) RefreshGrid(gv *widgets.GridView) {
	gv.ShowSuffix = a.cfg.ShowYears
	gv.Refresh()
}
