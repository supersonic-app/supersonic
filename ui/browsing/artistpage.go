package browsing

import (
	"log"
	"slices"
	"sort"
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
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// these strings should be keys in the translation dictionaries
var (
	discographySorts = []string{"Year (ascending)", "Year (descending)", "Name (A-Z)"}
)

var _ fyne.Widget = (*ArtistPage)(nil)

type artistPageState struct {
	artistID     string
	activeView   int
	topTrackSort widgets.TracklistSort
	allTrackSort widgets.TracklistSort

	pool  *util.WidgetPool
	cfg   *backend.ArtistPageConfig
	pm    *backend.PlaybackManager
	mp    mediaprovider.MediaProvider
	im    *backend.ImageManager
	contr *controller.Controller

	gridScrollPos    float32 // for album grid (or grouped releases)
	topListScrollPos float32 // for Top Tracks list
	allListScrollPos float32 // for All Tracks list

	sectionVis          widgets.GroupedReleasesSectionVisibility
	sectionVisNeedApply bool
}

type ArtistPage struct {
	widget.BaseWidget

	artistPageState
	disposed bool

	artistInfo *mediaprovider.ArtistWithAlbums

	topTracks []*mediaprovider.Track
	allTracks []*mediaprovider.Track

	albumGrid       *widgets.GridView
	groupedReleases *widgets.GroupedReleases
	tracklistCtr    *fyne.Container
	sortButton      *widgets.SortChooserButton
	nowPlayingID    string
	header          *ArtistPageHeader
	container       *fyne.Container
}

const (
	viewTopTracks   = "Top Tracks"
	viewDiscography = "Discography"
	viewAllTracks   = "All Tracks"
)

func NewArtistPage(artistID string, cfg *backend.ArtistPageConfig, pool *util.WidgetPool, pm *backend.PlaybackManager, mp mediaprovider.MediaProvider, im *backend.ImageManager, contr *controller.Controller) *ArtistPage {
	activeView := 0

	switch cfg.InitialView {
	case viewDiscography:
		activeView = 0
	case viewTopTracks:
		activeView = 1
	case viewAllTracks:
		activeView = 2
	}

	return newArtistPage(artistPageState{
		artistID:   artistID,
		cfg:        cfg,
		pool:       pool,
		pm:         pm,
		mp:         mp,
		im:         im,
		contr:      contr,
		activeView: activeView,
	})
}

func newArtistPage(state artistPageState) *ArtistPage {
	a := &ArtistPage{artistPageState: state}
	a.ExtendBaseWidget(a)
	if h := a.pool.Obtain(util.WidgetTypeArtistPageHeader); h != nil {
		a.header = h.(*ArtistPageHeader)
		a.header.Clear()
	} else {
		a.header = NewArtistPageHeader(a)
	}
	a.header.artistPage = a
	a.header.Compact = a.cfg.CompactHeader
	if img, ok := state.im.GetCachedArtistImage(state.artistID); ok {
		a.header.artistImage.SetImage(img, true /*tappable*/)
	}
	viewToggle := widgets.NewToggleText(0, []string{lang.L("Discography"), lang.L("Top Tracks"), lang.L("All Tracks")})
	viewToggle.SetActivatedLabel(a.activeView)
	viewToggle.OnChanged = a.onViewChange
	a.sortButton = widgets.NewSortChooserButton(util.LocalizeSlice(discographySorts), func(selIdx int) {
		a.cfg.DiscographySort = discographySorts[selIdx]
		a.showAlbumGrid(true /*reSort*/)
	})
	a.sortButton.AlignLeft = true
	for i, sort := range discographySorts {
		if sort == a.cfg.DiscographySort {
			a.sortButton.SetSelectedIndex(i)
		}
	}
	viewToggleRow := container.NewBorder(nil, nil,
		container.NewHBox(util.NewHSpace(5), viewToggle),
		container.NewHBox(a.sortButton, util.NewHSpace(10)),
		layout.NewSpacer(),
	)
	a.container = container.NewBorder(
		container.New(&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, TopPadding: 15, BottomPadding: 10}, a.header),
		nil, nil, nil,
		container.NewBorder(viewToggleRow, nil, nil, nil, layout.NewSpacer()))
	go a.load()
	return a
}

var _ CanSelectAll = (*ArtistPage)(nil)

func (a *ArtistPage) SelectAll() {
	if a.activeView == 1 && a.tracklistCtr != nil {
		a.tracklistCtr.Objects[0].(*widgets.Tracklist).SelectAll()
	}
}

func (a *ArtistPage) UnselectAll() {
	if a.tracklistCtr != nil {
		a.tracklistCtr.Objects[0].(*widgets.Tracklist).UnselectAll()
	}
}

func (a *ArtistPage) Route() controller.Route {
	return controller.ArtistRoute(a.artistID)
}

func (a *ArtistPage) Reload() {
	a.topTracks = nil
	a.allTracks = nil
	go a.load()
}

func (a *ArtistPage) Save() SavedPage {
	a.disposed = true
	s := a.artistPageState
	if a.tracklistCtr != nil {
		tl := a.tracklistCtr.Objects[0].(*widgets.Tracklist)
		switch a.activeView {
		case 1:
			s.topListScrollPos = tl.GetScrollOffset()
			s.topTrackSort = tl.Sorting()
		case 2:
			s.allListScrollPos = tl.GetScrollOffset()
			s.allTrackSort = tl.Sorting()
		}
		tl.Clear()
		a.pool.Release(util.WidgetTypeTracklist, tl)
	}
	a.header.artistPage = nil
	a.pool.Release(util.WidgetTypeArtistPageHeader, a.header)
	if a.albumGrid != nil {
		s.gridScrollPos = a.albumGrid.GetScrollOffset()
		a.albumGrid.Clear()
		a.pool.Release(util.WidgetTypeGridView, a.albumGrid)
	}
	if a.groupedReleases != nil {
		s.gridScrollPos = a.groupedReleases.GetScrollOffset()
		s.sectionVis = a.groupedReleases.GetSectionVisibility()
		s.sectionVisNeedApply = true
		a.groupedReleases.Model = widgets.GroupedReleasesModel{}
		a.pool.Release(util.WidgetTypeGroupedReleases, a.groupedReleases)
	}
	return &s
}

var _ CanShowNowPlaying = (*ArtistPage)(nil)

func (a *ArtistPage) OnSongChange(track mediaprovider.MediaItem, lastScrobbledIfAny *mediaprovider.Track) {
	a.nowPlayingID = sharedutil.MediaItemIDOrEmptyStr(track)
	if a.tracklistCtr != nil {
		tl := a.tracklistCtr.Objects[0].(*widgets.Tracklist)
		tl.SetNowPlaying(a.nowPlayingID)
		tl.IncrementPlayCount(sharedutil.MediaItemIDOrEmptyStr(lastScrobbledIfAny))
	}
}

var _ Scrollable = (*ArtistPage)(nil)

func (g *ArtistPage) Scroll(scrollAmt float32) {
	if g.activeView == 0 && g.albumGrid != nil {
		g.albumGrid.ScrollToOffset(g.albumGrid.GetScrollOffset() + scrollAmt)
	} else if g.activeView == 0 && g.groupedReleases != nil {
		g.groupedReleases.ScrollToOffset(g.groupedReleases.GetScrollOffset() + scrollAmt)
	} else if (g.activeView == 1 || g.activeView == 2) && g.tracklistCtr != nil {
		tl := g.tracklistCtr.Objects[0].(*widgets.Tracklist)
		tl.ScrollBy(scrollAmt)
	}
}

func (a *ArtistPage) playArtistRadio() {
	go func() {
		err := a.pm.PlaySimilarSongs(a.artistID)
		if err != nil {
			fyne.Do(func() {
				log.Printf("error playing similar songs: %v", err)
				a.contr.ToastProvider.ShowErrorToast(lang.L("Unable to play artist radio"))
			})
		}
	}()
}

func (a *ArtistPage) getGridViewAlbumsModel() []widgets.GridViewItemModel {
	if a.artistInfo == nil {
		return nil
	}

	a.sortAlbumsSlices(a.artistInfo.Albums)
	return sharedutil.MapSlice(a.artistInfo.Albums, a.albumToGridViewItemModel)
}

func (a *ArtistPage) albumToGridViewItemModel(al *mediaprovider.Album) widgets.GridViewItemModel {
	return widgets.GridViewItemModel{
		Name:        al.Name,
		ID:          al.ID,
		CoverArtID:  al.CoverArtID,
		Secondary:   []string{strconv.Itoa(al.YearOrZero())},
		CanFavorite: true,
		IsFavorite:  al.Favorite,
	}
}

func (a *ArtistPage) getGroupedReleasesModel() widgets.GroupedReleasesModel {
	if a.artistInfo == nil {
		return widgets.GroupedReleasesModel{}
	}
	albums := []*mediaprovider.Album{}
	compilations := []*mediaprovider.Album{}
	eps := []*mediaprovider.Album{}
	singles := []*mediaprovider.Album{}

	for _, album := range a.artistInfo.Albums {
		switch rt := album.ReleaseTypes; {
		case rt&mediaprovider.ReleaseTypeEP > 0:
			eps = append(eps, album)
		case rt&mediaprovider.ReleaseTypeCompilation > 0:
			compilations = append(compilations, album)
		case rt&mediaprovider.ReleaseTypeSingle > 0:
			singles = append(singles, album)
		default:
			albums = append(albums, album)
		}
	}

	a.sortAlbumsSlices(albums, compilations, eps, singles)
	return widgets.GroupedReleasesModel{
		Albums:       sharedutil.MapSlice(albums, a.albumToGridViewItemModel),
		Compilations: sharedutil.MapSlice(compilations, a.albumToGridViewItemModel),
		EPs:          sharedutil.MapSlice(eps, a.albumToGridViewItemModel),
		Singles:      sharedutil.MapSlice(singles, a.albumToGridViewItemModel),
	}
}

func (a *ArtistPage) sortAlbumsSlices(slices ...[]*mediaprovider.Album) {
	var curSlice []*mediaprovider.Album
	sortFunc := func(x, y int) bool {
		return curSlice[y].Date.After(curSlice[x].Date)
	}
	switch a.cfg.DiscographySort {
	case discographySorts[1]: /*year descending*/
		sortFunc = func(x, y int) bool {
			return curSlice[x].Date.After(curSlice[y].Date)
		}
	case discographySorts[2]: /*name*/
		sortFunc = func(x, y int) bool {
			sortStr := func(a *mediaprovider.Album) string {
				if a.SortName != "" {
					return a.SortName
				}
				return a.Name
			}
			return sortStr(curSlice[x]) < sortStr(curSlice[y])
		}
	}

	for _, slice := range slices {
		curSlice = slice
		sort.Slice(slice, sortFunc)
	}
}

// should be called asynchronously
func (a *ArtistPage) load() {
	artist, err := a.mp.GetArtist(a.artistID)
	if err != nil {
		log.Printf("Failed to get artist: %s", err.Error())
		return
	}

	fyne.Do(func() {
		if a.disposed {
			return
		}
		a.artistInfo = artist
		a.header.Update(artist, a.im)
		a.onViewChange(a.activeView)
	})

	info, err := a.mp.GetArtistInfo(a.artistID)
	if err != nil {
		log.Printf("Failed to get artist info: %s", err.Error())
	}
	fyne.Do(func() {
		if !a.disposed {
			a.header.UpdateInfo(info)
		}
	})
}

func (a *ArtistPage) showAlbumGrid(reSort bool) {
	if a.activeView == 0 && (a.albumGrid != nil || a.groupedReleases != nil) && !reSort {
		return // already showing album grid
	}
	a.activeView = 0

	allAlbums := func() bool {
		return slices.IndexFunc(a.artistInfo.Albums, func(al *mediaprovider.Album) bool {
			return al.ReleaseTypes&mediaprovider.ReleaseTypeCompilation > 0 ||
				al.ReleaseTypes&mediaprovider.ReleaseTypeEP > 0 ||
				al.ReleaseTypes&mediaprovider.ReleaseTypeSingle > 0
		}) < 0
	}
	useGroupedReleases := a.artistInfo != nil && len(a.artistInfo.Albums) <= 50 && !allAlbums()

	if a.albumGrid == nil && a.groupedReleases == nil {
		if a.artistInfo == nil {
			// page not loaded yet or invalid artist
			return
		}
		if useGroupedReleases {
			model := a.getGroupedReleasesModel()
			if g := a.pool.Obtain(util.WidgetTypeGroupedReleases); g != nil {
				a.groupedReleases = g.(*widgets.GroupedReleases)
				a.groupedReleases.Model = model
			} else {
				a.groupedReleases = widgets.NewGroupedReleases(model, a.im)
			}
			a.contr.ConnectGroupedReleasesActions(a.groupedReleases)
			if a.sectionVisNeedApply {
				a.groupedReleases.SetSectionVisibility(a.sectionVis, true)
				a.sectionVisNeedApply = false
			}
			if a.gridScrollPos != 0 {
				a.groupedReleases.ScrollToOffset(a.gridScrollPos)
				a.gridScrollPos = 0
			}
		} else {
			model := a.getGridViewAlbumsModel()
			if g := a.pool.Obtain(util.WidgetTypeGridView); g != nil {
				a.albumGrid = g.(*widgets.GridView)
				a.albumGrid.Placeholder = myTheme.AlbumIcon
				a.albumGrid.ResetFixed(model)
			} else {
				a.albumGrid = widgets.NewFixedGridView(model, a.im, myTheme.AlbumIcon)
			}
			a.contr.ConnectAlbumGridActions(a.albumGrid)
			if a.gridScrollPos != 0 {
				a.albumGrid.ScrollToOffset(a.gridScrollPos)
				a.gridScrollPos = 0
			}
		}
	} else if reSort {
		if useGroupedReleases {
			a.groupedReleases.Model = a.getGroupedReleasesModel()
		} else {
			model := a.getGridViewAlbumsModel()
			a.albumGrid.ResetFixed(model)
		}
	}
	a.sortButton.Show()
	if useGroupedReleases {
		a.container.Objects[0].(*fyne.Container).Objects[0] = a.groupedReleases
		a.container.Objects[0].Refresh()
	} else {
		a.container.Objects[0].(*fyne.Container).Objects[0] = a.albumGrid
	}
	a.container.Objects[0].Refresh()
}

func (a *ArtistPage) showTopTracks() {
	if a.activeView == 1 && a.tracklistCtr != nil {
		return // already showing top tracks
	}

	var tl *widgets.Tracklist
	if a.tracklistCtr == nil {
		tl = a.obtainNewTracklist()
		a.tracklistCtr = container.New(
			&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, BottomPadding: 10},
			tl)
		a.contr.ConnectTracklistActions(tl)
	} else {
		tl = a.tracklistCtr.Objects[0].(*widgets.Tracklist)
		tl.Clear()
	}
	a.activeView = 1
	tl.SetSorting(a.topTrackSort)
	a.container.Objects[0].(*fyne.Container).Objects[0] = a.tracklistCtr
	a.container.Objects[0].Refresh()

	updateTracklist := func(ts []*mediaprovider.Track) {
		tl.SetTracks(ts)
		tl.SetNowPlaying(a.nowPlayingID)
		if a.topListScrollPos != 0 {
			tl.ScrollToOffset(a.topListScrollPos)
		}
	}
	if a.topTracks != nil {
		updateTracklist(a.topTracks)
		return
	}

	tl.SetLoading(true)
	go func() {
		ts, err := a.mp.GetTopTracks(a.artistInfo.Artist, 20)
		if err != nil {
			log.Printf("error getting top songs: %s", err.Error())
			return
		}
		if a.disposed || a.activeView != 1 {
			return
		}

		fyne.Do(func() {
			a.topTracks = ts
			tl.SetLoading(false)
			updateTracklist(ts)
		})
	}()
}

func (a *ArtistPage) showAllTracks() {
	if a.activeView == 2 && a.tracklistCtr != nil {
		return // already showing all tracks
	}

	var tl *widgets.Tracklist
	if a.tracklistCtr == nil {
		tl = a.obtainNewTracklist()
		a.tracklistCtr = container.New(
			&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, BottomPadding: 10},
			tl)
		a.contr.ConnectTracklistActions(tl)
	} else {
		tl = a.tracklistCtr.Objects[0].(*widgets.Tracklist)
		tl.Clear()
	}
	a.activeView = 2
	tl.SetSorting(a.allTrackSort)
	a.container.Objects[0].(*fyne.Container).Objects[0] = a.tracklistCtr
	a.container.Objects[0].Refresh()

	updateTracklist := func(ts []*mediaprovider.Track) {
		tl.SetTracks(ts)
		tl.SetNowPlaying(a.nowPlayingID)
		if a.allListScrollPos != 0 {
			tl.ScrollToOffset(a.allListScrollPos)
		}
	}
	if a.allTracks != nil {
		updateTracklist(a.allTracks)
		return
	}

	tl.SetLoading(true)
	go func() {
		ts, err := a.mp.GetArtistTracks(a.artistID)
		if err != nil {
			log.Printf("error getting all songs: %s", err.Error())
			return
		}
		if a.disposed || a.activeView != 2 {
			return
		}

		fyne.Do(func() {
			a.allTracks = ts
			tl.SetLoading(false)
			updateTracklist(ts)
		})
	}()
}

func (a *ArtistPage) obtainNewTracklist() *widgets.Tracklist {
	var tl *widgets.Tracklist
	if t := a.pool.Obtain(util.WidgetTypeTracklist); t != nil {
		tl = t.(*widgets.Tracklist)
		tl.Reset()
		tl.SetTracks([]*mediaprovider.Track{})
	} else {
		tl = widgets.NewTracklist([]*mediaprovider.Track{}, a.im, false)
	}
	tl.Options = widgets.TracklistOptions{AutoNumber: true}
	_, canRate := a.mp.(mediaprovider.SupportsRating)
	_, canShare := a.mp.(mediaprovider.SupportsSharing)
	tl.Options.DisableRating = !canRate
	tl.Options.DisableSharing = !canShare
	tl.SetVisibleColumns(a.cfg.TracklistColumns)
	tl.OnVisibleColumnsChanged = func(cols []string) {
		a.cfg.TracklistColumns = cols
	}
	return tl
}

func (a *ArtistPage) onViewChange(num int) {
	// save current data
	if a.tracklistCtr != nil {
		tl := a.tracklistCtr.Objects[0].(*widgets.Tracklist)
		switch a.activeView {
		case 1:
			a.topListScrollPos = tl.GetScrollOffset()
			a.topTrackSort = tl.Sorting()
		case 2:
			a.allListScrollPos = tl.GetScrollOffset()
			a.allTrackSort = tl.Sorting()
		}
	}

	switch num {
	case 0:
		a.showAlbumGrid(false /*reSort*/)
		a.sortButton.Show()
		a.cfg.InitialView = viewDiscography
	case 1:
		a.showTopTracks()
		a.sortButton.Hide()
		a.cfg.InitialView = viewTopTracks
	case 2:
		a.showAllTracks()
		a.sortButton.Hide()
		a.cfg.InitialView = viewAllTracks
	}
}

func (a *ArtistPage) CreateRenderer() fyne.WidgetRenderer {
	a.ExtendBaseWidget(a)
	return widget.NewSimpleRenderer(a.container)
}

func (s *artistPageState) Restore() Page {
	return newArtistPage(*s)
}

const artistBioNotAvailableKey = "Artist biography not available."

type ArtistPageHeader struct {
	widget.BaseWidget

	Compact bool

	artistID              string
	artistPage            *ArtistPage
	artistImage           *widgets.ImagePlaceholder
	artistImageID         string
	titleDisp             *widget.RichText
	biographyDisp         *widget.Label
	biographyScroll       *container.Scroll
	similarArtists        *fyne.Container
	favoriteBtn           *widgets.FavoriteButton
	playBtn               *widget.Button
	playRadioBtn          *widget.Button
	menuBtn               *widget.Button
	container             *fyne.Container
	collapseBtn           *widgets.HeaderCollapseButton
	fullSizeCoverFetching bool
	// shareMenuItem  *fyne.MenuItem
}

func NewArtistPageHeader(page *ArtistPage) *ArtistPageHeader {
	// due to widget reuse a.artistPage can change so page MUST NOT
	// be directly captured in a closure throughout this function!
	biographyText := widget.NewLabel(lang.L(artistBioNotAvailableKey))

	scroll := container.NewScroll(biographyText)
	scroll.SetMinSize(fyne.NewSize(0, 100))

	a := &ArtistPageHeader{
		artistPage:      page,
		titleDisp:       widget.NewRichTextWithText(""),
		biographyDisp:   biographyText,
		biographyScroll: scroll,
		similarArtists:  container.NewHBox(),
	}
	a.titleDisp.Segments[0].(*widget.TextSegment).Style = widget.RichTextStyle{
		SizeName: theme.SizeNameHeadingText,
	}
	a.artistImage = widgets.NewImagePlaceholder(myTheme.ArtistIcon, myTheme.HeaderImageSize)
	a.artistImage.OnTapped = func(*fyne.PointEvent) { a.showPopUpCover() }
	a.favoriteBtn = widgets.NewFavoriteButton(func() { go a.toggleFavorited() })
	a.playBtn = widget.NewButtonWithIcon(lang.L("Play Discography"), theme.MediaPlayIcon(), func() {
		go a.artistPage.pm.PlayArtistDiscography(a.artistID, false /*shuffle*/)
	})
	a.playRadioBtn = widget.NewButtonWithIcon(lang.L("Play Artist Radio"), myTheme.ShuffleIcon, func() {
		// must not pass playArtistRadio func directly to NewButton
		// because the artistPage bound to this header can change when reused
		a.artistPage.playArtistRadio()
	})

	var pop *widget.PopUpMenu
	a.menuBtn = widget.NewButtonWithIcon("", theme.MoreHorizontalIcon(), nil)
	a.menuBtn.OnTapped = func() {
		if pop == nil {
			shuffleTracks := fyne.NewMenuItem(lang.L("Shuffle tracks"), func() {
				go a.artistPage.pm.PlayArtistDiscography(a.artistID, true /*shuffle*/)
			})
			shuffleTracks.Icon = myTheme.TracksIcon
			shuffleAlbums := fyne.NewMenuItem(lang.L("Shuffle albums"), func() {
				go a.artistPage.pm.ShuffleArtistAlbums(a.artistID)
			})
			shuffleAlbums.Icon = myTheme.AlbumIcon
			menu := fyne.NewMenu("", shuffleTracks, shuffleAlbums)
			pop = widget.NewPopUpMenu(menu, fyne.CurrentApp().Driver().CanvasForObject(a))
		}
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(a.menuBtn)
		pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+a.menuBtn.Size().Height))
	}

	// TODO: Uncomment and merge into OnTapped above when at least one media provider supports sharing artists.
	// shareMenuItem = fyne.NewMenuItem("Share...", func() {
	// 	a.artistPage.contr.ShowShareDialog(a.artistID)
	// })
	// a.shareMenuItem.Icon = myTheme.ShareIcon
	//
	// canShareArtists := false
	// if r, canShare := a.artistPage.mp.(mediaprovider.SupportsSharing); canShare {
	// 	canShareArtists = r.CanShareArtists()
	// }
	// shareMenuItem.Disabled = !canShareArtists

	a.biographyDisp.Wrapping = fyne.TextWrapWord
	a.collapseBtn = widgets.NewHeaderCollapseButton(func() {
		a.Compact = !a.Compact
		a.artistPage.cfg.CompactHeader = a.Compact
		a.artistPage.Refresh()
	})
	a.collapseBtn.Hidden = true
	a.ExtendBaseWidget(a)
	a.createContainer()
	return a
}

func (a *ArtistPageHeader) Clear() {
	a.artistID = ""
	a.artistImageID = ""
	a.favoriteBtn.IsFavorited = false
	a.titleDisp.Segments[0].(*widget.TextSegment).Text = ""
	a.biographyDisp.Text = lang.L(artistBioNotAvailableKey)
	for _, obj := range a.similarArtists.Objects {
		obj.Hide()
	}
	a.artistImage.SetImage(nil, false)
	a.fullSizeCoverFetching = false
}

func (a *ArtistPageHeader) Update(artist *mediaprovider.ArtistWithAlbums, im *backend.ImageManager) {
	if artist == nil {
		return
	}
	a.favoriteBtn.IsFavorited = artist.Favorite
	a.favoriteBtn.Refresh()
	a.artistID = artist.ID
	a.titleDisp.Segments[0].(*widget.TextSegment).Text = artist.Name
	a.titleDisp.Refresh()
	if artist.CoverArtID == "" {
		return
	}
	a.artistImageID = artist.CoverArtID
	go func() {
		if cover, err := im.GetCoverThumbnail(artist.CoverArtID); err == nil {
			fyne.Do(func() { a.artistImage.SetImage(cover, true) })
		} else {
			log.Printf("error fetching cover: %v", err)
		}
	}()
}

func (a *ArtistPageHeader) UpdateInfo(info *mediaprovider.ArtistInfo) {
	if info == nil {
		return
	}

	if text := util.PlaintextFromHTMLString(info.Biography); text != "" {
		a.biographyDisp.SetText(text)
	}

	if len(a.similarArtists.Objects) == 0 {
		a.similarArtists.Add(widget.NewLabel(lang.L("Similar artists") + ":"))
	}
	for _, obj := range a.similarArtists.Objects {
		obj.Hide()
	}
	for i, art := range info.SimilarArtists {
		if i == 0 {
			a.similarArtists.Objects[0].Show() // "Similar Artists:" label
		}
		if i == 4 {
			break
		}
		if len(a.similarArtists.Objects) <= i+1 {
			a.similarArtists.Add(widget.NewHyperlink("", nil))
		}
		h := a.similarArtists.Objects[i+1].(*widget.Hyperlink)
		h.SetText(art.Name)
		h.OnTapped = func(id string) func() {
			return func() { a.artistPage.contr.NavigateTo(controller.ArtistRoute(id)) }
		}(art.ID)
		h.Show()
	}
	a.similarArtists.Refresh()

	if info.ImageURL != "" {
		if a.artistImage.HaveImage() {
			go func() {
				_ = a.artistPage.im.RefreshCachedArtistImageIfExpired(a.artistID, info.ImageURL)
			}()
		} else {
			go func() {
				im, err := a.artistPage.im.FetchAndCacheArtistImage(a.artistID, info.ImageURL)
				if err == nil {
					fyne.Do(func() { a.artistImage.SetImage(im, true /*tappable*/) })
				}
			}()
		}
	}
}

var _ desktop.Hoverable = (*ArtistPageHeader)(nil)

func (a *ArtistPageHeader) MouseIn(*desktop.MouseEvent) {
	a.collapseBtn.Show()
	a.Refresh()
}

func (a *ArtistPageHeader) MouseOut() {
	a.collapseBtn.HideIfNotMousedIn()
}

func (a *ArtistPageHeader) MouseMoved(*desktop.MouseEvent) {
}

func (a *ArtistPageHeader) Refresh() {
	a.biographyDisp.Hidden = a.Compact
	a.similarArtists.Hidden = a.Compact
	a.collapseBtn.Collapsed = a.Compact
	if a.Compact {
		a.biographyScroll.Hide()
		a.artistImage.SetMinSize(fyne.NewSquareSize(myTheme.CompactHeaderImageSize))
	} else {
		a.biographyScroll.Show()
		a.artistImage.SetMinSize(fyne.NewSquareSize(myTheme.HeaderImageSize))
	}
	a.BaseWidget.Refresh()
}

// should NOT be called asynchronously
func (a *ArtistPageHeader) showPopUpCover() {
	if a.artistImageID == "" {
		if im := a.artistImage.Image(); im != nil {
			a.artistPage.contr.ShowPopUpImage(im)
		}
	} else {
		if a.fullSizeCoverFetching {
			return
		}
		a.fullSizeCoverFetching = true
		go func() {
			defer func() { a.fullSizeCoverFetching = false }()
			cover, err := a.artistPage.im.GetFullSizeCoverArt(a.artistImageID)
			if err != nil {
				log.Printf("error getting full size album cover: %s", err.Error())
				return
			}
			if a.artistPage != nil {
				fyne.Do(func() { a.artistPage.contr.ShowPopUpImage(cover) })
			}
		}()
	}
}

func (a *ArtistPageHeader) toggleFavorited() {
	params := mediaprovider.RatingFavoriteParameters{ArtistIDs: []string{a.artistID}}
	a.artistPage.mp.SetFavorite(params, a.favoriteBtn.IsFavorited)
}

func (a *ArtistPageHeader) createContainer() {
	btnContainer := container.NewHBox(util.NewHSpace(2), a.favoriteBtn, a.playBtn, a.playRadioBtn, a.menuBtn)

	a.container = util.AddHeaderBackground(
		container.NewStack(
			container.NewBorder(nil, nil, a.artistImage, nil,
				container.NewVBox(
					container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-10),
						a.titleDisp, a.biographyScroll, util.NewVSpace(10), a.similarArtists),
					btnContainer),
			),
			container.NewVBox(container.NewHBox(layout.NewSpacer(), a.collapseBtn)),
		),
	)
}

func (a *ArtistPageHeader) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.container)
}
