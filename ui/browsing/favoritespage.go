package browsing

import (
	"fmt"
	"log"
	"strconv"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/controller"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
	"github.com/supersonic-app/supersonic/ui/widgets"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type FavoritesPage struct {
	widget.BaseWidget

	cfg   *backend.FavoritesPageConfig
	pool  *util.WidgetPool
	contr *controller.Controller
	pm    *backend.PlaybackManager
	im    *backend.ImageManager
	mp    mediaprovider.MediaProvider

	disposed          bool
	trackSort         widgets.TracklistSort
	filter            mediaprovider.AlbumFilter
	searchText        string
	nowPlayingID      string
	pendingViewSwitch bool

	albumGrid       *widgets.GridView
	gridState       *widgets.GridViewState
	searchGridState *widgets.GridViewState
	artistGrid      *widgets.GridView
	tracklistCtr    *fyne.Container
	shuffleBtn      *widget.Button
	searcher        *widgets.SearchEntry
	filterBtn       *widgets.AlbumFilterButton
	titleDisp       *widget.RichText
	toggleBtns      *widgets.ToggleButtonGroup
	container       *fyne.Container
}

func NewFavoritesPage(cfg *backend.FavoritesPageConfig, pool *util.WidgetPool, contr *controller.Controller, mp mediaprovider.MediaProvider, pm *backend.PlaybackManager, im *backend.ImageManager) *FavoritesPage {
	a := &FavoritesPage{
		filter: mediaprovider.NewAlbumFilter(mediaprovider.AlbumFilterOptions{
			ExcludeUnfavorited: true,
		}),
		cfg:   cfg,
		pool:  pool,
		contr: contr,
		pm:    pm,
		mp:    mp,
		im:    im,
	}
	a.ExtendBaseWidget(a)
	a.createHeader(0)
	iter := widgets.NewGridViewAlbumIterator(mp.IterateAlbums("", a.filter))
	if g := pool.Obtain(util.WidgetTypeGridView); g != nil {
		a.albumGrid = g.(*widgets.GridView)
		a.albumGrid.Placeholder = myTheme.AlbumIcon
		a.albumGrid.Reset(iter)
	} else {
		a.albumGrid = widgets.NewGridView(iter, a.im, myTheme.AlbumIcon)
	}
	a.albumGrid.ShowSuffix = cfg.ShowAlbumYears
	a.contr.ConnectAlbumGridActions(a.albumGrid)
	if cfg.InitialView == "Artists" {
		a.toggleBtns.SetActivatedButton(1)
		a.onShowFavoriteArtists()
	} else if cfg.InitialView == "Songs" {
		a.toggleBtns.SetActivatedButton(2)
		a.onShowFavoriteSongs()
	} else { // Albums view
		a.createContainer(a.albumGrid)
	}
	return a
}

func (a *FavoritesPage) createHeader(activeBtnIdx int) {
	a.titleDisp = widget.NewRichTextWithText(lang.L("Favorites"))
	a.titleDisp.Segments[0].(*widget.TextSegment).Style = widget.RichTextStyle{
		SizeName: theme.SizeNameHeadingText,
	}
	a.toggleBtns = widgets.NewToggleButtonGroup(activeBtnIdx,
		widget.NewButtonWithIcon("", myTheme.AlbumIcon, a.onShowFavoriteAlbums),
		widget.NewButtonWithIcon("", myTheme.ArtistIcon, a.onShowFavoriteArtists),
		widget.NewButtonWithIcon("", myTheme.TracksIcon, a.onShowFavoriteSongs))
	a.shuffleBtn = widget.NewButtonWithIcon(lang.L("Shuffle"), myTheme.ShuffleIcon, func() {
		if tr := a.tracklistOrNil(); tr != nil {
			a.pm.LoadTracks(tr.GetTracks(), backend.Replace, true /*shuffle*/)
			a.pm.PlayFromBeginning()
		}
	})
	a.shuffleBtn.Hidden = activeBtnIdx != 2 /*favorite songs*/
	a.searcher = widgets.NewSearchEntry()
	a.searcher.PlaceHolder = lang.L("Search page")
	a.searcher.OnSearched = a.onSearched
	a.searcher.Entry.Text = a.searchText
	a.filterBtn = widgets.NewAlbumFilterButton(a.filter, a.mp.GetGenres)
	a.filterBtn.FavoriteDisabled = true
	a.filterBtn.OnChanged = a.Reload
}

func (a *FavoritesPage) createContainer(initialView fyne.CanvasObject) {
	searchVbox := container.NewVBox(layout.NewSpacer(), a.searcher, layout.NewSpacer())
	a.container = container.NewBorder(container.NewHBox(util.NewHSpace(9),
		a.titleDisp,
		container.NewCenter(a.toggleBtns),
		util.NewHSpace(2),
		container.NewCenter(a.shuffleBtn),
		layout.NewSpacer(),
		container.NewCenter(a.filterBtn),
		searchVbox, util.NewHSpace(15)),
		nil, nil, nil, initialView)
}

func restoreFavoritesPage(saved *savedFavoritesPage) *FavoritesPage {
	a := &FavoritesPage{
		cfg:             saved.cfg,
		contr:           saved.contr,
		pool:            saved.pool,
		pm:              saved.pm,
		mp:              saved.mp,
		im:              saved.im,
		gridState:       saved.gridState,
		searchGridState: saved.searchGridState,
		searchText:      saved.searchText,
		filter:          saved.filter,
		trackSort:       saved.trackSort,
	}
	a.ExtendBaseWidget(a)
	a.createHeader(saved.activeToggleBtn)
	state := saved.gridState
	if saved.searchText != "" {
		state = saved.searchGridState
	}
	if g := a.pool.Obtain(util.WidgetTypeGridView); g != nil {
		a.albumGrid = g.(*widgets.GridView)
		a.albumGrid.Placeholder = myTheme.AlbumIcon
		a.albumGrid.ResetFromState(state)
	} else {
		a.albumGrid = widgets.NewGridViewFromState(state)
	}
	a.toggleBtns.SetActivatedButton(saved.activeToggleBtn)

	if saved.activeToggleBtn == 1 {
		a.onShowFavoriteArtists()
	} else if saved.activeToggleBtn == 2 {
		a.onShowFavoriteSongs()
	} else {
		a.createContainer(a.albumGrid)
	}

	return a
}

var _ Scrollable = (*FavoritesPage)(nil)

func (a *FavoritesPage) Scroll(amount float32) {
	var grid *widgets.GridView
	switch a.toggleBtns.ActivatedButtonIndex() {
	case 0: // albums
		grid = a.albumGrid
	case 1: // artists
		grid = a.artistGrid
	default:
		if tr := a.tracklistOrNil(); tr != nil {
			tr.ScrollBy(amount)
		}
		return
	}
	if grid != nil {
		grid.ScrollToOffset(grid.GetScrollOffset() + amount)
	}
}

func (a *FavoritesPage) Route() controller.Route {
	return controller.FavoritesRoute()
}

func (a *FavoritesPage) Reload() {
	// reload favorite albums view
	if a.searchText != "" {
		a.doSearchAlbums(a.searchText)
	} else {
		iter := a.mp.IterateAlbums("", a.filter)
		a.albumGrid.Reset(widgets.NewGridViewAlbumIterator(iter))
	}
	if a.tracklistCtr != nil || a.artistGrid != nil {
		go func() {
			// re-fetch starred info from server
			starred, err := a.mp.GetFavorites()
			if err != nil {
				log.Printf("error getting starred items: %s", err.Error())
				return
			}
			if a.disposed {
				return
			}
			fyne.Do(func() {
				if tr := a.tracklistOrNil(); tr != nil {
					// refresh favorite songs view
					tr.SetTracks(starred.Tracks)
					if a.toggleBtns.ActivatedButtonIndex() == 2 {
						// favorite songs view is visible
						tr.Refresh()
					}
				}
				if a.artistGrid != nil {
					// refresh favorite artists view
					a.artistGrid.ResetFixed(buildArtistGridViewModel(starred.Artists))
					if a.toggleBtns.ActivatedButtonIndex() == 1 {
						// favorite artists view is visible
						a.artistGrid.Refresh()
					}
				}
			})
		}()
	}
}

func (a *FavoritesPage) Save() SavedPage {
	a.disposed = true
	sf := &savedFavoritesPage{
		cfg:             a.cfg,
		contr:           a.contr,
		pool:            a.pool,
		pm:              a.pm,
		mp:              a.mp,
		im:              a.im,
		filter:          a.filter,
		searchText:      a.searchText,
		gridState:       a.gridState,
		searchGridState: a.searchGridState,
		activeToggleBtn: a.toggleBtns.ActivatedButtonIndex(),
	}
	if a.searchText != "" {
		sf.searchGridState = a.albumGrid.SaveToState()
	} else {
		sf.gridState = a.albumGrid.SaveToState()
	}
	a.albumGrid.Clear()
	a.pool.Release(util.WidgetTypeGridView, a.albumGrid)
	if a.artistGrid != nil {
		a.artistGrid.Clear()
		a.pool.Release(util.WidgetTypeGridView, a.artistGrid)
	}
	if tl := a.tracklistOrNil(); tl != nil {
		sf.trackSort = tl.Sorting()
		tl.Clear()
		a.pool.Release(util.WidgetTypeTracklist, tl)
	}
	return sf
}

var _ Searchable = (*FavoritesPage)(nil)

func (a *FavoritesPage) SearchWidget() fyne.Focusable {
	return a.searcher
}

func (a *FavoritesPage) onSearched(query string) {
	if query == "" {
		a.albumGrid.ResetFromState(a.gridState)
		a.searchGridState = nil
	} else {
		a.doSearchAlbums(query)
	}
	a.searchText = query
}

var _ CanShowNowPlaying = (*FavoritesPage)(nil)

func (a *FavoritesPage) OnSongChange(item mediaprovider.MediaItem, lastScrobbledIfAny *mediaprovider.Track) {
	a.nowPlayingID = sharedutil.MediaItemIDOrEmptyStr(item)
	if tracklist := a.tracklistOrNil(); tracklist != nil {
		tracklist.SetNowPlaying(a.nowPlayingID)
		tracklist.IncrementPlayCount(sharedutil.MediaItemIDOrEmptyStr(lastScrobbledIfAny))
	}
}

var _ CanSelectAll = (*FavoritesPage)(nil)

func (a *FavoritesPage) SelectAll() {
	if a.toggleBtns.ActivatedButtonIndex() == 2 /*songs*/ && a.tracklistCtr != nil {
		a.tracklistOrNil().SelectAll() // can't be nil in this case
	}
}

func (a *FavoritesPage) UnselectAll() {
	if a.toggleBtns.ActivatedButtonIndex() == 2 /*songs*/ && a.tracklistCtr != nil {
		a.tracklistOrNil().UnselectAll() // can't be nil in this case
	}
}

func (a *FavoritesPage) Refresh() {
	if a.albumGrid != nil {
		a.albumGrid.ShowSuffix = a.cfg.ShowAlbumYears
	}
	a.BaseWidget.Refresh()
}

func (a *FavoritesPage) tracklistOrNil() *widgets.Tracklist {
	if a.tracklistCtr != nil {
		return a.tracklistCtr.Objects[0].(*widgets.Tracklist)
	}
	return nil
}

func (a *FavoritesPage) doSearchAlbums(query string) {
	if a.searchText == "" {
		a.gridState = a.albumGrid.SaveToState()
	}
	iter := widgets.NewGridViewAlbumIterator(a.mp.SearchAlbums(query, a.filter))
	a.albumGrid.Reset(iter)
}

func (a *FavoritesPage) onShowFavoriteAlbums() {
	a.cfg.InitialView = "Albums" // save setting
	a.searcher.Entry.Show()
	a.shuffleBtn.Hide()
	a.filterBtn.Show()
	a.container.Objects[0] = a.albumGrid
	a.Refresh()
}

func (a *FavoritesPage) onShowFavoriteArtists() {
	a.cfg.InitialView = "Artists" // save setting
	a.shuffleBtn.Hide()
	a.searcher.Entry.Hide() // disable search on artists for now
	a.filterBtn.Hide()
	if a.artistGrid == nil {
		if a.pendingViewSwitch {
			return
		}
		a.pendingViewSwitch = true
		if a.container == nil {
			a.createContainer(layout.NewSpacer())
		}
		go func() {
			fav, err := a.mp.GetFavorites()
			if err != nil {
				log.Printf("error getting starred items: %s", err.Error())
				return
			}
			if a.disposed {
				return
			}
			fyne.Do(func() {
				model := buildArtistGridViewModel(fav.Artists)
				if g := a.pool.Obtain(util.WidgetTypeGridView); g != nil {
					a.artistGrid = g.(*widgets.GridView)
					a.artistGrid.Placeholder = myTheme.ArtistIcon
					a.artistGrid.ResetFixed(model)
				} else {
					a.artistGrid = widgets.NewFixedGridView(model, a.im, myTheme.ArtistIcon)
				}
				canShareArtists := false
				if r, canShare := a.mp.(mediaprovider.SupportsSharing); canShare {
					canShareArtists = r.CanShareArtists()
				}
				a.artistGrid.DisableSharing = !canShareArtists
				a.contr.ConnectArtistGridActions(a.artistGrid)
				a.container.Objects[0] = a.artistGrid
				a.Refresh()
				a.pendingViewSwitch = false
			})
		}()
	} else {
		a.container.Objects[0] = a.artistGrid
		a.Refresh()
	}
}

func buildArtistGridViewModel(artists []*mediaprovider.Artist) []widgets.GridViewItemModel {
	model := make([]widgets.GridViewItemModel, 0)
	for _, ar := range artists {
		albums := lang.L("albums")
		if ar.AlbumCount == 1 {
			albums = lang.L("album")
		}
		fallbackAlbumsMsg := fmt.Sprintf("%d %s", ar.AlbumCount, albums)
		albumsMsg := lang.LocalizePluralKey("{{.albumsCount}} albums",
			fallbackAlbumsMsg, ar.AlbumCount, map[string]string{"albumsCount": strconv.Itoa(ar.AlbumCount)})
		model = append(model, widgets.GridViewItemModel{
			ID:          ar.ID,
			CoverArtID:  ar.CoverArtID,
			Name:        ar.Name,
			Secondary:   []string{albumsMsg},
			CanFavorite: true,
			IsFavorite:  ar.Favorite,
		})
	}
	return model
}

func (a *FavoritesPage) onShowFavoriteSongs() {
	a.cfg.InitialView = "Songs" // save setting
	a.searcher.Entry.Hide()     // disable search on songs for now
	a.filterBtn.Hide()
	a.shuffleBtn.Show()
	if a.tracklistCtr == nil {
		if a.pendingViewSwitch {
			return
		}
		a.pendingViewSwitch = true
		if a.container == nil {
			a.createContainer(layout.NewSpacer())
		}
		go func() {
			fav, err := a.mp.GetFavorites()
			if err != nil {
				log.Printf("error getting starred items: %s", err.Error())
				return
			}
			if a.disposed {
				return
			}
			fyne.Do(func() {
				var tracklist *widgets.Tracklist
				if tl := a.pool.Obtain(util.WidgetTypeTracklist); tl != nil {
					tracklist = tl.(*widgets.Tracklist)
					tracklist.Reset()
					tracklist.SetTracks(fav.Tracks)
				} else {
					tracklist = widgets.NewTracklist(fav.Tracks, a.im, false)
				}
				tracklist.Options = widgets.TracklistOptions{AutoNumber: true}
				_, canRate := a.mp.(mediaprovider.SupportsRating)
				_, canShare := a.mp.(mediaprovider.SupportsSharing)
				tracklist.Options.DisableRating = !canRate
				tracklist.Options.DisableSharing = !canShare
				tracklist.SetVisibleColumns(a.cfg.TracklistColumns)
				tracklist.SetSorting(a.trackSort)
				tracklist.OnVisibleColumnsChanged = func(cols []string) {
					a.cfg.TracklistColumns = cols
				}
				tracklist.SetNowPlaying(a.nowPlayingID)
				a.contr.ConnectTracklistActions(tracklist)
				a.tracklistCtr = container.New(
					&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, TopPadding: 5, BottomPadding: 15},
					tracklist)
				a.container.Objects[0] = a.tracklistCtr
				a.Refresh()
				a.pendingViewSwitch = false
			})
		}()
	} else {
		a.container.Objects[0] = a.tracklistCtr
		a.Refresh()
	}
}

func (a *FavoritesPage) CreateRenderer() fyne.WidgetRenderer {
	a.ExtendBaseWidget(a)
	return widget.NewSimpleRenderer(a.container)
}

type savedFavoritesPage struct {
	cfg             *backend.FavoritesPageConfig
	contr           *controller.Controller
	pool            *util.WidgetPool
	pm              *backend.PlaybackManager
	mp              mediaprovider.MediaProvider
	im              *backend.ImageManager
	gridState       *widgets.GridViewState
	searchGridState *widgets.GridViewState
	filter          mediaprovider.AlbumFilter
	searchText      string
	activeToggleBtn int
	trackSort       widgets.TracklistSort
}

func (s *savedFavoritesPage) Restore() Page {
	return restoreFavoritesPage(s)
}
