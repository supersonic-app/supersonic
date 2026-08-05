package browsing

import (
	"log"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/ui/controller"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
	"github.com/supersonic-app/supersonic/ui/widgets"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
)

type genrePageAdapter struct {
	genre     string
	cfg       *backend.AlbumsPageConfig
	contr     *controller.Controller
	mp        mediaprovider.MediaProvider
	pm        *backend.PlaybackManager
	filter    mediaprovider.AlbumFilter
	filterBtn *widgets.AlbumFilterButton
}

func NewGenrePage(genre string, cfg *backend.AlbumsPageConfig, pool *util.WidgetPool, contr *controller.Controller, pm *backend.PlaybackManager, mp mediaprovider.MediaProvider, im *backend.ImageManager) Page {
	adapter := &genrePageAdapter{genre: genre, cfg: cfg, contr: contr, mp: mp, pm: pm}
	return NewGridViewPage(adapter, pool, mp, im)
}

func (g *genrePageAdapter) Title() string { return g.genre }

func (g *genrePageAdapter) Filter() mediaprovider.AlbumFilter {
	if g.filter == nil {
		g.filter = mediaprovider.NewAlbumFilter(
			mediaprovider.AlbumFilterOptions{
				Genres: []string{g.genre},
			},
		)
	}
	return g.filter
}

func (g *genrePageAdapter) FilterButton() widgets.FilterButton[mediaprovider.Album, mediaprovider.AlbumFilterOptions] {
	if g.filterBtn == nil {
		g.filterBtn = widgets.NewAlbumFilterButton(g.Filter(), func() ([]*mediaprovider.Genre, error) { return nil, nil })
		g.filterBtn.GenreDisabled = true
	}
	return g.filterBtn
}

func (g *genrePageAdapter) PlaceholderResource() fyne.Resource {
	return myTheme.AlbumIcon
}

func (g *genrePageAdapter) Route() controller.Route {
	return controller.GenreRoute(g.genre)
}

func (g *genrePageAdapter) ActionButton() fyne.CanvasObject {
	fn := func() {
		go func() {
			var err error
			if g.cfg.ShuffleMode == "Albums" {
				err = g.pm.PlayRandomAlbums(g.genre)
			} else {
				err = g.pm.PlayRandomSongs(g.genre)
			}
			if err != nil {
				log.Printf("error playing random tracks: %v", err)
				fyne.Do(func() {
					g.contr.ToastProvider.ShowErrorToast(lang.L("Unable to play random tracks"))
				})
			}
		}()
	}

	var tracks, albums *fyne.MenuItem

	setShuffleMode := func(isAlbums bool) {
		if isAlbums {
			g.cfg.ShuffleMode = "Albums"
		} else {
			g.cfg.ShuffleMode = "Tracks"
		}
		albums.Checked = isAlbums
		tracks.Checked = !isAlbums
	}

	tracks = fyne.NewMenuItem(lang.L("Tracks"), func() { setShuffleMode(false) })
	tracks.Icon = myTheme.TracksIcon
	albums = fyne.NewMenuItem(lang.L("Albums"), func() { setShuffleMode(true) })
	albums.Icon = myTheme.AlbumIcon

	isAlbums := g.cfg.ShuffleMode == "Albums"
	albums.Checked = isAlbums
	tracks.Checked = !isAlbums

	menu := fyne.NewMenu("", tracks, albums)
	return widgets.NewOptionButtonWithIcon(lang.L("Play random"), myTheme.ShuffleIcon, menu, fn)
}

func (a *genrePageAdapter) Iter(sortOrderIdx int, filter mediaprovider.AlbumFilter) widgets.GridViewIterator {
	return widgets.NewGridViewAlbumIterator(a.mp.IterateAlbums("", filter))
}

func (a *genrePageAdapter) SearchIter(query string, filter mediaprovider.AlbumFilter) widgets.GridViewIterator {
	return widgets.NewGridViewAlbumIterator(a.mp.SearchAlbums(query, filter))
}

func (g *genrePageAdapter) InitGrid(gv *widgets.GridView) {
	g.contr.ConnectAlbumGridActions(gv)
	gv.ShowSuffix = g.cfg.ShowYears
}

func (g *genrePageAdapter) RefreshGrid(gv *widgets.GridView) {
	gv.ShowSuffix = g.cfg.ShowYears
	gv.Refresh()
}
