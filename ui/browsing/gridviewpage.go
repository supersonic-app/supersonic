package browsing

import (
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/ui/controller"
	"github.com/supersonic-app/supersonic/ui/util"
	"github.com/supersonic-app/supersonic/ui/widgets"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Base widget for grid view pages
type GridViewPage[M, F any] struct {
	widget.BaseWidget

	adapter GridViewPageAdapter[M, F]
	pool    *util.WidgetPool
	mp      mediaprovider.MediaProvider
	im      *backend.ImageManager

	grid            *widgets.GridView
	gridState       *widgets.GridViewState
	searchGridState *widgets.GridViewState

	title      *widget.RichText
	sortOrder  *widgets.SortChooserButton
	filterBtn  widgets.FilterButton[M, F]
	filter     mediaprovider.MediaFilter[M, F]
	searcher   *widgets.SearchEntry
	searchText string

	container *fyne.Container
}

// Base type for pages that show an iterable GridView
type GridViewPageAdapter[M, F any] interface {
	// Returns the title for the page
	Title() string

	// Returns the base media filter for this page, if any.
	// A filterable page with no base filters applied should return a zero-valued
	// filter pointer, *not* nil. (Nil means unfilterable and no filter button created.)
	Filter() mediaprovider.MediaFilter[M, F]

	// Returns the filter button for the page, if any.
	FilterButton() widgets.FilterButton[M, F]

	// Returns the cover placeholder resource for the page
	PlaceholderResource() fyne.Resource

	// Returns the route for the page
	Route() controller.Route

	// Returns the ActionButton for this page, if any
	ActionButton() fyne.CanvasObject

	// Returns the iterator for the given sortOrder and filter.
	// (Non-media pages can ignore the filter argument)
	Iter(sortOrderIdx int, filter mediaprovider.MediaFilter[M, F]) widgets.GridViewIterator

	// Returns the iterator for the given search query and filter.
	SearchIter(query string, filter mediaprovider.MediaFilter[M, F]) widgets.GridViewIterator

	// Function that initialized the GridView with page-specific settings
	// and connects the GridView callbacks to the appropriate action handlers.
	InitGrid(*widgets.GridView)

	// Function called when settings may have changed and the grid needs
	// reconfiguring with possible settings changes.
	RefreshGrid(*widgets.GridView)
}

type GridViewPageAdapterGetItems interface {
	// Optionally allows the GridViewPage to inject a function that
	// can be used to retrieve the GridViewItemModels for the
	// items currently loaded into the GridView.
	SetItemsFunc(func() []widgets.GridViewItemModel)
}

type SortableGridViewPageAdapter interface {
	// Returns the list of sort orders
	// and the index of the initially selected sort order
	SortOrders() ([]string, int)

	// Saves the given sort order setting, the selected index is passed.
	SaveSortOrder(int)
}

func NewGridViewPage[M, F any](
	adapter GridViewPageAdapter[M, F],
	pool *util.WidgetPool,
	mp mediaprovider.MediaProvider,
	im *backend.ImageManager,
) *GridViewPage[M, F] {
	gp := &GridViewPage[M, F]{
		adapter:   adapter,
		pool:      pool,
		mp:        mp,
		im:        im,
		filter:    adapter.Filter(),
		filterBtn: adapter.FilterButton(),
	}
	gp.ExtendBaseWidget(gp)
	gp.createTitleAndSort()

	_, canShare := mp.(mediaprovider.SupportsSharing)
	iter := adapter.Iter(gp.getSortOrderIdx(), gp.getFilter())
	if g := pool.Obtain(util.WidgetTypeGridView); g != nil {
		gp.grid = g.(*widgets.GridView)
		gp.grid.Placeholder = adapter.PlaceholderResource()
		gp.grid.Reset(iter)
	} else {
		gp.grid = widgets.NewGridView(iter, im, adapter.PlaceholderResource())
	}
	gp.grid.DisableSharing = !canShare
	adapter.InitGrid(gp.grid)

	// If adapter supports SetItemsFunc, call it to inject the items dependency
	if plfSetter, ok := adapter.(GridViewPageAdapterGetItems); ok {
		plfSetter.SetItemsFunc(gp.grid.Items)
	}

	gp.createSearchAndFilter()
	gp.createContainer()
	return gp
}

func (g *GridViewPage[M, F]) createTitleAndSort() {
	g.title = widget.NewRichText(&widget.TextSegment{
		Text:  g.adapter.Title(),
		Style: widget.RichTextStyle{SizeName: theme.SizeNameHeadingText},
	})
	if s, ok := g.adapter.(SortableGridViewPageAdapter); ok {
		sorts, selected := s.SortOrders()
		g.sortOrder = widgets.NewSortChooserButton(sorts, g.onSortOrderChanged)
		g.sortOrder.SetSelectedIndex(selected)
	}
}

func (g *GridViewPage[M, F]) createSearchAndFilter() {
	g.searcher = widgets.NewSearchEntry()
	g.searcher.PlaceHolder = lang.L("Search page")
	g.searcher.Text = g.searchText
	g.searcher.OnSearched = g.OnSearched
	g.filterBtn = g.adapter.FilterButton()
	if g.filterBtn != nil {
		g.filterBtn.SetOnChanged(g.Reload)
	}
}

func (g *GridViewPage[M, F]) createContainer() {
	header := container.NewHBox(util.NewHSpace(6), g.title)
	if g.sortOrder != nil {
		header.Add(container.NewCenter(g.sortOrder))
	}
	if b := g.adapter.ActionButton(); b != nil {
		header.Add(container.NewCenter(b))
	}
	header.Add(layout.NewSpacer())
	if g.filterBtn != nil {
		header.Add(container.NewCenter(g.filterBtn))
	}
	header.Add(container.NewCenter(g.searcher))
	header.Add(util.NewHSpace(12))
	g.container = container.NewBorder(header, nil, nil, nil, g.grid)
}

func (g *GridViewPage[M, F]) Reload() {
	if g.searchText != "" {
		g.doSearch(g.searchText)
	} else {
		g.grid.Reset(g.adapter.Iter(g.getSortOrderIdx(), g.getFilter()))
	}
}

func (g *GridViewPage[M, F]) Route() controller.Route {
	return g.adapter.Route()
}

func (g *GridViewPage[M, F]) SearchWidget() fyne.Focusable {
	return g.searcher
}

func (g *GridViewPage[M, F]) Scroll(scrollAmt float32) {
	g.grid.ScrollToOffset(g.grid.GetScrollOffset() + scrollAmt)
}

func (g *GridViewPage[M, F]) OnSearched(query string) {
	if query == "" {
		if g.sortOrder != nil {
			g.sortOrder.Enable()
		}
		g.grid.ResetFromState(g.gridState)
		g.searchGridState = nil
	} else {
		if g.sortOrder != nil {
			g.sortOrder.Disable()
		}
		g.doSearch(query)
	}
	g.searchText = query
}

func (g *GridViewPage[M, F]) Refresh() {
	g.adapter.RefreshGrid(g.grid)
}

func (g *GridViewPage[M, F]) doSearch(query string) {
	if g.searchText == "" {
		g.gridState = g.grid.SaveToState()
	}
	g.grid.Reset(g.adapter.SearchIter(query, g.getFilter()))
}

func (g *GridViewPage[M, F]) onSortOrderChanged(idx int) {
	if g.grid == nil {
		return // callback from initializing
	}

	g.adapter.(SortableGridViewPageAdapter).SaveSortOrder(g.getSortOrderIdx())
	g.grid.Reset(g.adapter.Iter(idx, g.getFilter()))
}

func (g *GridViewPage[M, F]) getFilter() mediaprovider.MediaFilter[M, F] {
	return g.filter
}

func (g *GridViewPage[M, F]) getSortOrderIdx() int {
	if g.sortOrder != nil {
		return g.sortOrder.SelectedIndex()
	}
	return 0
}

func (g *GridViewPage[M, F]) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.container)
}

type savedGridViewPage[M, F any] struct {
	adapter         GridViewPageAdapter[M, F]
	im              *backend.ImageManager
	mp              mediaprovider.MediaProvider
	searchText      string
	filter          mediaprovider.MediaFilter[M, F]
	pool            *util.WidgetPool
	sortOrderIdx    int
	gridState       *widgets.GridViewState
	searchGridState *widgets.GridViewState
}

func (g *GridViewPage[M, F]) Save() SavedPage {
	sa := &savedGridViewPage[M, F]{
		adapter:         g.adapter,
		pool:            g.pool,
		mp:              g.mp,
		im:              g.im,
		searchText:      g.searchText,
		filter:          g.filter,
		sortOrderIdx:    g.getSortOrderIdx(),
		gridState:       g.gridState,
		searchGridState: g.searchGridState,
	}
	if g.searchText == "" {
		sa.gridState = g.grid.SaveToState()
	} else {
		sa.searchGridState = g.grid.SaveToState()
	}
	g.grid.Clear()
	g.pool.Release(util.WidgetTypeGridView, g.grid)
	return sa
}

func (s *savedGridViewPage[M, F]) Restore() Page {
	gp := &GridViewPage[M, F]{
		adapter:         s.adapter,
		pool:            s.pool,
		mp:              s.mp,
		im:              s.im,
		gridState:       s.gridState,
		searchGridState: s.searchGridState,
		searchText:      s.searchText,
		filter:          s.filter,
	}
	gp.ExtendBaseWidget(gp)

	gp.createTitleAndSort()
	state := s.gridState
	if s.searchText != "" {
		if gp.sortOrder != nil {
			gp.sortOrder.Disable()
		}
		state = s.searchGridState
	}
	if g := gp.pool.Obtain(util.WidgetTypeGridView); g != nil {
		gp.grid = g.(*widgets.GridView)
		gp.grid.ResetFromState(state)
	} else {
		gp.grid = widgets.NewGridViewFromState(state)
	}
	gp.adapter.InitGrid(gp.grid)
	gp.createSearchAndFilter()
	gp.createContainer()
	return gp
}
