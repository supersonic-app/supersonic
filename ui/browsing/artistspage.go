package browsing

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/ui/controller"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
	"github.com/supersonic-app/supersonic/ui/widgets"
)

type artistsPageAdapter struct {
	cfg    *backend.ArtistsPageConfig
	contr  *controller.Controller
	mp     mediaprovider.MediaProvider
	pm     *backend.PlaybackManager
	filter mediaprovider.ArtistFilter
}

func NewArtistsPage(cfg *backend.ArtistsPageConfig, pool *util.WidgetPool, contr *controller.Controller, pm *backend.PlaybackManager, mp mediaprovider.MediaProvider, im *backend.ImageManager) Page {
	adapter := &artistsPageAdapter{cfg: cfg, contr: contr, mp: mp, pm: pm}
	return NewGridViewPage(adapter, pool, mp, im)
}

func (a *artistsPageAdapter) Title() string { return lang.L("Artists") }

func (a *artistsPageAdapter) Filter() mediaprovider.ArtistFilter {
	if a.filter == nil {
		a.filter = mediaprovider.NewArtistFilter(
			mediaprovider.ArtistFilterOptions{},
		)
	}
	return a.filter
}

func (a *artistsPageAdapter) FilterButton() widgets.FilterButton[mediaprovider.Artist, mediaprovider.ArtistFilterOptions] {
	return nil
}

func (a *artistsPageAdapter) PlaceholderResource() fyne.Resource { return myTheme.ArtistIcon }

func (a *artistsPageAdapter) Route() controller.Route { return controller.ArtistsRoute() }

func (a *artistsPageAdapter) SortOrders() ([]string, int) {
	orders := a.mp.ArtistSortOrders()
	sortOrder := max(slices.Index(orders, a.cfg.SortOrder), 0)

	return util.LocalizeSlice(orders), sortOrder
}

func (a *artistsPageAdapter) SaveSortOrder(orderIdx int) {
	a.cfg.SortOrder = a.mp.ArtistSortOrders()[orderIdx]
}

func (a *artistsPageAdapter) ActionButton() fyne.CanvasObject { return nil }

func (a *artistsPageAdapter) Iter(sortOrderIdx int, filter mediaprovider.ArtistFilter) widgets.GridViewIterator {
	sortOrder := a.mp.ArtistSortOrders()[sortOrderIdx]
	return widgets.NewGridViewArtistIterator(a.mp.IterateArtists(sortOrder, filter))
}

func (a *artistsPageAdapter) SearchIter(query string, filter mediaprovider.ArtistFilter) widgets.GridViewIterator {
	return widgets.NewGridViewArtistIterator(a.mp.SearchArtists(query, filter))
}

func (a *artistsPageAdapter) InitGrid(gv *widgets.GridView) {
	canShareArtists := false
	if r, canShare := a.mp.(mediaprovider.SupportsSharing); canShare {
		canShareArtists = r.CanShareArtists()
	}
	gv.DisableSharing = !canShareArtists
	a.contr.ConnectArtistGridActions(gv)
}

func (a *artistsPageAdapter) RefreshGrid(gv *widgets.GridView) {
	gv.Refresh()
}
