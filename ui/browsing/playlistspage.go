package browsing

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/res"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/controller"
	"github.com/supersonic-app/supersonic/ui/layouts"
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

type PlaylistsPage struct {
	widget.BaseWidget

	pool              *util.WidgetPool
	cfg               *backend.PlaylistsPageConfig
	contr             *controller.Controller
	mp                mediaprovider.MediaProvider
	playlists         []*mediaprovider.Playlist
	searchedPlaylists []*mediaprovider.Playlist

	viewToggle *widgets.ToggleButtonGroup
	newBtn     *widget.Button
	searcher   *widgets.SearchEntry
	titleDisp  *widget.RichText
	container  *fyne.Container
	listView   *PlaylistList
	listSort   widgets.ListHeaderSort
	gridView   *widgets.GridView

	initialListScrollPos float32
	initialGridScrollPos float32
}

func NewPlaylistsPage(contr *controller.Controller, pool *util.WidgetPool, cfg *backend.PlaylistsPageConfig, mp mediaprovider.MediaProvider) *PlaylistsPage {
	activeView := 0
	if cfg.InitialView == "Grid" {
		activeView = 1
	}
	return newPlaylistsPage(contr, pool, cfg, mp, "", activeView, widgets.ListHeaderSort{}, 0, 0)
}

func newPlaylistsPage(
	contr *controller.Controller,
	pool *util.WidgetPool,
	cfg *backend.PlaylistsPageConfig,
	mp mediaprovider.MediaProvider,
	searchText string,
	activeView int,
	listSort widgets.ListHeaderSort,
	listScrollPos float32,
	gridScrollPos float32,
) *PlaylistsPage {
	a := &PlaylistsPage{
		pool:                 pool,
		cfg:                  cfg,
		mp:                   mp,
		contr:                contr,
		listSort:             listSort,
		titleDisp:            widget.NewRichTextWithText(lang.L("Playlists")),
		initialListScrollPos: listScrollPos,
		initialGridScrollPos: gridScrollPos,
	}
	a.ExtendBaseWidget(a)
	a.titleDisp.Segments[0].(*widget.TextSegment).Style.SizeName = theme.SizeNameHeadingText
	a.searcher = widgets.NewSearchEntry()
	a.searcher.PlaceHolder = lang.L("Search page")
	a.searcher.OnSearched = a.onSearched
	a.searcher.Entry.Text = searchText
	a.viewToggle = widgets.NewToggleButtonGroup(0,
		widget.NewButtonWithIcon("", theme.NewThemedResource(res.ResListSvg), a.showListView),
		widget.NewButtonWithIcon("", theme.NewThemedResource(res.ResGridSvg), a.showGridView))
	a.viewToggle.SetActivatedButton(activeView)
	a.newBtn = widget.NewButtonWithIcon(lang.L("New Playlist"), theme.ContentAddIcon(), func() {
		a.contr.DoCreatePlaylistWorkflow()
	})
	if activeView == 0 {
		a.createListView()
		a.buildContainer(a.listView)
	} else {
		a.createGridView(nil)
		a.buildContainer(a.gridView)
	}

	go a.load(searchText != "")
	return a
}

func (a *PlaylistsPage) buildContainer(initialView fyne.CanvasObject) {
	searchVbox := container.NewVBox(layout.NewSpacer(), a.searcher, layout.NewSpacer())
	a.container = container.New(&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, TopPadding: 5, BottomPadding: 15},
		container.NewBorder(
			container.NewHBox(
				a.titleDisp,
				container.NewCenter(a.viewToggle),
				util.NewHSpace(2),
				container.NewCenter(a.newBtn),
				layout.NewSpacer(),
				searchVbox,
			),
			nil, nil, nil, initialView))
}

var _ Scrollable = (*PlaylistsPage)(nil)

func (p *PlaylistsPage) Scroll(scrollAmt float32) {
	if p.viewToggle.ActivatedButtonIndex() == 1 && p.gridView != nil {
		p.gridView.ScrollToOffset(p.gridView.GetScrollOffset() + scrollAmt)
	} else if p.viewToggle.ActivatedButtonIndex() == 0 && p.listView != nil {
		p.listView.list.ScrollToOffset(p.listView.list.GetScrollOffset() + scrollAmt)
	}
}

// should be called asynchronously
func (a *PlaylistsPage) load(searchOnLoad bool) {
	playlists, err := a.mp.GetPlaylists()
	if err != nil {
		log.Printf("error loading playlists: %v", err.Error())
	}

	fyne.Do(func() {
		a.playlists = playlists
		if searchOnLoad {
			a.onSearched(a.searcher.Entry.Text)
		} else {
			a.refreshView(playlists)
		}
	})
}

func (a *PlaylistsPage) createListView() {
	a.listView = NewPlaylistList(a.listSort)
	a.listView.OnNavTo = a.showPlaylistPage
}

func (a *PlaylistsPage) createGridView(playlists []*mediaprovider.Playlist) {
	model := createPlaylistGridViewModel(playlists)
	if g := a.pool.Obtain(util.WidgetTypeGridView); g != nil {
		a.gridView = g.(*widgets.GridView)
		a.gridView.Placeholder = myTheme.PlaylistIcon
		a.gridView.ResetFixed(model)
	} else {
		a.gridView = widgets.NewFixedGridView(model, a.contr.App.ImageManager, myTheme.PlaylistIcon)
	}
	a.gridView.OnPlay = func(id string, shuffle bool) {
		go a.contr.App.PlaybackManager.PlayPlaylist(id, 0, shuffle)
	}
	a.gridView.OnAddToQueue = func(id string) {
		go a.contr.App.PlaybackManager.LoadPlaylist(id, backend.Append, false)
	}
	a.gridView.OnShowItemPage = a.showPlaylistPage
	a.gridView.OnShowSecondaryPage = nil
	a.gridView.OnAddToPlaylist = func(id string) {
		go func() {
			pl, err := a.contr.App.ServerManager.Server.GetPlaylist(id)
			if err != nil {
				log.Printf("error loading playlist: %s", err.Error())
				return
			}
			fyne.Do(func() {
				a.contr.DoAddTracksToPlaylistWorkflow(sharedutil.TracksToIDs(pl.Tracks))
			})
		}()
	}
	a.gridView.OnDownload = func(id string) {
		go func() {
			pl, err := a.contr.App.ServerManager.Server.GetPlaylist(id)
			if err != nil {
				log.Printf("error loading playlist: %s", err.Error())
				return
			}
			fyne.Do(func() { a.contr.ShowDownloadDialog(pl.Tracks, pl.Name) })
		}()
	}
}

func (a *PlaylistsPage) showListView() {
	a.cfg.InitialView = "List" // save setting
	if a.listView == nil {
		a.createListView()
		if a.searcher.Entry.Text != "" {
			a.listView.SetPlaylists(a.searchedPlaylists)
		} else {
			a.listView.SetPlaylists(a.playlists)
		}
	}
	a.container.Objects[0].(*fyne.Container).Objects[0] = a.listView
	a.container.Objects[0].Refresh()
}

func (a *PlaylistsPage) showGridView() {
	a.cfg.InitialView = "Grid" // save setting
	if a.gridView == nil {
		playlists := a.playlists
		if a.searcher.Entry.Text != "" {
			playlists = a.searchedPlaylists
		}
		a.createGridView(playlists)
	}
	a.container.Objects[0].(*fyne.Container).Objects[0] = a.gridView
	a.container.Objects[0].Refresh()
}

func createPlaylistGridViewModel(playlists []*mediaprovider.Playlist) []widgets.GridViewItemModel {
	return sharedutil.MapSlice(playlists, func(pl *mediaprovider.Playlist) widgets.GridViewItemModel {
		var tracks string
		if pl.TrackCount == 1 {
			tracks = lang.L("track")
		} else {
			tracks = lang.L("tracks")
		}
		fallbackTracksMsg := fmt.Sprintf("%d %s", pl.TrackCount, tracks)
		tracksMsg := lang.LocalizePluralKey("{{.trackCount}} tracks",
			fallbackTracksMsg, pl.TrackCount, map[string]string{"trackCount": strconv.Itoa(pl.TrackCount)})
		return widgets.GridViewItemModel{
			Name:       pl.Name,
			ID:         pl.ID,
			CoverArtID: pl.CoverArtID,
			Secondary:  []string{tracksMsg},
		}
	})
}

func (a *PlaylistsPage) showPlaylistPage(id string) {
	a.contr.NavigateTo(controller.PlaylistRoute(id))
}

func (a *PlaylistsPage) onSearched(query string) {
	// since the playlist list is returned in full non-paginated, we will do our own
	// simple search based on the name, description, and owner, rather than calling a server API
	var playlists []*mediaprovider.Playlist
	if query == "" {
		a.searchedPlaylists = nil
		playlists = a.playlists
	} else {
		a.searchedPlaylists = sharedutil.FilterSlice(a.playlists, func(p *mediaprovider.Playlist) bool {
			qLower := strings.ToLower(query)
			return strings.Contains(strings.ToLower(p.Name), qLower) ||
				strings.Contains(strings.ToLower(p.Description), qLower) ||
				strings.Contains(strings.ToLower(p.Owner), qLower)
		})
		playlists = a.searchedPlaylists
	}
	a.refreshView(playlists)
}

// update the model for both views if initialized,
// refresh the active view
func (a *PlaylistsPage) refreshView(playlists []*mediaprovider.Playlist) {
	if a.listView != nil {
		a.listView.SetPlaylists(playlists)
	}
	if a.gridView != nil {
		a.gridView.ResetFixed(createPlaylistGridViewModel(playlists))
	}
	if a.viewToggle.ActivatedButtonIndex() == 0 {
		a.listView.Refresh() // ensures content size updates for ScrollToOffset too
		if a.initialListScrollPos != 0 {
			a.listView.list.ScrollToOffset(a.initialListScrollPos)
			a.initialListScrollPos = 0
		}
	} else {
		if a.initialGridScrollPos != 0 {
			a.gridView.ScrollToOffset(a.initialGridScrollPos)
			a.initialGridScrollPos = 0
		} else {
			a.gridView.Refresh()
		}
	}
}

var _ Searchable = (*PlaylistsPage)(nil)

func (a *PlaylistsPage) SearchWidget() fyne.Focusable {
	return a.searcher
}

func (a *PlaylistsPage) Route() controller.Route {
	return controller.PlaylistsRoute()
}

func (a *PlaylistsPage) Reload() {
	go a.load(a.searcher.Entry.Text != "")
}

func (a *PlaylistsPage) Save() SavedPage {
	s := &savedPlaylistsPage{
		contr:      a.contr,
		pool:       a.pool,
		cfg:        a.cfg,
		mp:         a.mp,
		searchText: a.searcher.Entry.Text,
		activeView: a.viewToggle.ActivatedButtonIndex(),
	}
	if a.gridView != nil {
		s.gridScrollPos = a.gridView.GetScrollOffset()
		a.gridView.Clear()
		a.pool.Release(util.WidgetTypeGridView, a.gridView)
	}
	if a.listView != nil {
		s.listScrollPos = a.listView.list.GetScrollOffset()
		s.listSort = a.listView.sorting
	}
	return s
}

type savedPlaylistsPage struct {
	contr         *controller.Controller
	pool          *util.WidgetPool
	cfg           *backend.PlaylistsPageConfig
	mp            mediaprovider.MediaProvider
	searchText    string
	activeView    int
	listSort      widgets.ListHeaderSort
	listScrollPos float32
	gridScrollPos float32
}

func (s *savedPlaylistsPage) Restore() Page {
	return newPlaylistsPage(s.contr, s.pool, s.cfg, s.mp, s.searchText, s.activeView, s.listSort, s.listScrollPos, s.gridScrollPos)
}

func (a *PlaylistsPage) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.container)
}

type PlaylistList struct {
	widget.BaseWidget

	OnNavTo func(string)

	playlistsOrigOrder []*mediaprovider.Playlist
	playlists          []*mediaprovider.Playlist
	sorting            widgets.ListHeaderSort

	columnsLayout *layouts.ColumnsLayout
	header        *widgets.ListHeader
	list          *widgets.FocusList
	container     *fyne.Container
}

func NewPlaylistList(initialSort widgets.ListHeaderSort) *PlaylistList {
	a := &PlaylistList{
		sorting: initialSort,
	}
	a.buildHeaderAndLayout()
	a.list = widgets.NewFocusList(
		func() int {
			return len(a.playlists)
		},
		func() fyne.CanvasObject {
			r := NewPlaylistListRow(a.columnsLayout)
			r.OnTapped = func() { a.onRowTapped(r.PlaylistID) }
			r.OnFocusNeighbor = func(up bool) {
				a.list.FocusNeighbor(r.ItemID(), up)
			}
			return r
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			row := item.(*PlaylistListRow)
			if row.PlaylistID != a.playlists[id].ID {
				row.EnsureUnfocused()
				row.ListItemID = id
				row.PlaylistID = a.playlists[id].ID
				row.nameLabel.Text = a.playlists[id].Name
				row.descrptionLabel.Text = strings.ReplaceAll(a.playlists[id].Description, "\n", " ")
				row.ownerLabel.Text = a.playlists[id].Owner
				row.trackCountLabel.Text = strconv.Itoa(a.playlists[id].TrackCount)
				row.Refresh()
			}
		},
	)
	a.container = container.NewBorder(a.header, nil, nil, nil, a.list)
	a.ExtendBaseWidget(a)
	return a
}

func (p *PlaylistList) buildHeaderAndLayout() {
	trackCount := lang.L("Track count")
	trackCountWidth := fyne.Max(125, widget.NewLabel(trackCount).MinSize().Width+15)
	p.columnsLayout = layouts.NewColumnsLayout([]float32{-1, -1, 200, trackCountWidth})

	p.header = widgets.NewListHeader([]widgets.ListColumn{
		{Text: lang.L("Name"), Alignment: fyne.TextAlignLeading, CanToggleVisible: false},
		{Text: lang.L("_Description"), Alignment: fyne.TextAlignLeading, CanToggleVisible: false},
		{Text: lang.L("Owner"), Alignment: fyne.TextAlignLeading, CanToggleVisible: false},
		{Text: trackCount, Alignment: fyne.TextAlignTrailing, CanToggleVisible: false},
	}, p.columnsLayout)
	p.header.SetSorting(p.sorting)
	p.header.OnColumnSortChanged = p.onSorted
}

// Sets the playlists in the list. Does not issue Refresh call.
func (p *PlaylistList) SetPlaylists(playlists []*mediaprovider.Playlist) {
	p.playlistsOrigOrder = playlists
	p.doSortPlaylists()
}

func (p *PlaylistList) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.container)
}

func (p *PlaylistList) onRowTapped(id string) {
	if p.OnNavTo != nil {
		p.OnNavTo(id)
	}
}

func (p *PlaylistList) onSorted(sort widgets.ListHeaderSort) {
	p.sorting = sort
	p.doSortPlaylists()
	p.Refresh()
}

func (p *PlaylistList) doSortPlaylists() {
	if p.sorting.Type == widgets.SortNone {
		p.playlists = p.playlistsOrigOrder
		return
	}
	switch p.sorting.ColNumber {
	case 0: // Name
		p.stringSort(func(p *mediaprovider.Playlist) string { return p.Name })
	case 1: // Description
		p.stringSort(func(p *mediaprovider.Playlist) string { return p.Description })
	case 2: // Owner
		p.stringSort(func(p *mediaprovider.Playlist) string { return p.Owner })
	case 3: // Track Count
		p.intSort(func(p *mediaprovider.Playlist) int { return p.TrackCount })
	}
}

func (p *PlaylistList) stringSort(fieldFn func(*mediaprovider.Playlist) string) {
	new := make([]*mediaprovider.Playlist, len(p.playlistsOrigOrder))
	copy(new, p.playlistsOrigOrder)
	sort.SliceStable(new, func(i, j int) bool {
		cmp := strings.Compare(fieldFn(new[i]), fieldFn(new[j]))
		if p.sorting.Type == widgets.SortDescending {
			return cmp > 0
		}
		return cmp < 0
	})
	p.playlists = new
}

func (p *PlaylistList) intSort(fieldFn func(*mediaprovider.Playlist) int) {
	new := make([]*mediaprovider.Playlist, len(p.playlistsOrigOrder))
	copy(new, p.playlistsOrigOrder)
	sort.SliceStable(new, func(i, j int) bool {
		if p.sorting.Type == widgets.SortDescending {
			return fieldFn(new[i]) > fieldFn(new[j])
		}
		return fieldFn(new[i]) < fieldFn(new[j])
	})
	p.playlists = new
}

type PlaylistListRow struct {
	widgets.FocusListRowBase

	PlaylistID string

	nameLabel       *widget.Label
	descrptionLabel *widget.Label
	ownerLabel      *widget.Label
	trackCountLabel *widget.Label
}

func NewPlaylistListRow(layout *layouts.ColumnsLayout) *PlaylistListRow {
	a := &PlaylistListRow{
		nameLabel:       util.NewTruncatingLabel(),
		descrptionLabel: util.NewTruncatingLabel(),
		ownerLabel:      util.NewTruncatingLabel(),
		trackCountLabel: util.NewTrailingAlignLabel(),
	}
	a.Content = container.New(layout, a.nameLabel, a.descrptionLabel, a.ownerLabel, a.trackCountLabel)
	a.ExtendBaseWidget(a)
	return a
}

func (a *PlaylistListRow) Tapped(*fyne.PointEvent) {
	if a.OnTapped != nil {
		a.OnTapped()
	}
}
