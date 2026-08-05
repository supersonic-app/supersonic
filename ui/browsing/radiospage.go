package browsing

import (
	"log"
	"net/url"
	"strings"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/controller"
	"github.com/supersonic-app/supersonic/ui/layouts"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/widgets"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var _ fyne.Widget = (*ArtistPage)(nil)

type RadiosPage struct {
	widget.BaseWidget

	contr  *controller.Controller
	rp     mediaprovider.RadioProvider
	pm     *backend.PlaybackManager
	radios []*mediaprovider.RadioStation
	list   *RadioList

	nowPlayingID string

	titleDisp   *widget.RichText
	noRadiosMsg fyne.CanvasObject
	container   *fyne.Container
	searcher    *widgets.SearchEntry
}

func NewRadiosPage(contr *controller.Controller, rp mediaprovider.RadioProvider, pm *backend.PlaybackManager) *RadiosPage {
	return newRadiosPage(contr, rp, pm, "", 0)
}

func newRadiosPage(contr *controller.Controller, rp mediaprovider.RadioProvider, pm *backend.PlaybackManager, searchText string, scrollPos float32) *RadiosPage {
	a := &RadiosPage{
		contr:     contr,
		rp:        rp,
		pm:        pm,
		titleDisp: widget.NewRichTextWithText(lang.L("Internet Radio Stations")),
	}
	a.ExtendBaseWidget(a)
	a.titleDisp.Segments[0].(*widget.TextSegment).Style.SizeName = theme.SizeNameHeadingText
	a.list = NewRadioList(&a.nowPlayingID)
	a.list.OnPlay = a.onPlay
	a.list.OnQueue = a.onQueue
	a.searcher = widgets.NewSearchEntry()
	a.searcher.PlaceHolder = lang.L("Search page")
	a.searcher.OnSearched = a.onSearched
	a.searcher.Entry.Text = searchText

	a.noRadiosMsg = container.NewCenter(widgets.NewInfoMessage(
		lang.L("No radio stations available"),
		lang.L("Configure your music server to add radio stations"),
	))
	a.noRadiosMsg.Hide()

	a.buildContainer()
	go a.load(searchText != "", scrollPos)
	return a
}

// should be called asynchronously
func (a *RadiosPage) load(searchOnLoad bool, scrollPos float32) {
	radios, err := a.rp.GetRadioStations()
	if err != nil {
		log.Printf("error loading radios: %v", err.Error())
	}

	fyne.Do(func() {
		if len(radios) == 0 {
			a.noRadiosMsg.Show()
		} else {
			a.noRadiosMsg.Hide()
		}
		a.radios = radios
		if searchOnLoad {
			a.onSearched(a.searcher.Entry.Text)
			if scrollPos != 0 {
				a.list.list.ScrollToOffset(scrollPos)
			}
		} else {
			a.list.SetRadios(a.radios)
			if scrollPos != 0 {
				a.list.list.ScrollToOffset(scrollPos)
				return
			}
			a.list.Refresh()
		}
	})
}

func (a *RadiosPage) onPlay(station *mediaprovider.RadioStation) {
	a.pm.PlayRadioStation(station)
}

func (a *RadiosPage) onQueue(station *mediaprovider.RadioStation, next bool) {
	queueMode := backend.Append
	if next {
		queueMode = backend.InsertNext
	}
	a.pm.LoadRadioStation(station, queueMode)
}

func (a *RadiosPage) onSearched(query string) {
	// since the radios list is returned in full non-paginated, we will do our own
	// simple search based on the radio name, rather than calling a server API
	if query == "" {
		a.list.SetRadios(a.radios)
	} else {
		query = strings.ToLower(query)
		result := sharedutil.FilterSlice(a.radios, func(x *mediaprovider.RadioStation) bool {
			return strings.Contains(strings.ToLower(x.StationName), query)
		})
		a.list.SetRadios(result)
	}
	a.list.list.ScrollTo(0)
}

var _ Searchable = (*RadiosPage)(nil)

func (a *RadiosPage) SearchWidget() fyne.Focusable {
	return a.searcher
}

var _ Scrollable = (*RadiosPage)(nil)

func (a *RadiosPage) Scroll(amount float32) {
	a.list.list.ScrollToOffset(a.list.list.GetScrollOffset() + amount)
}

var _ CanShowNowPlaying = (*RadiosPage)(nil)

func (a *RadiosPage) OnSongChange(playing mediaprovider.MediaItem, _ *mediaprovider.Track) {
	if playing != nil {
		a.nowPlayingID = playing.Metadata().ID
	} else {
		a.nowPlayingID = ""
	}
	a.list.Refresh()
}

func (a *RadiosPage) Route() controller.Route {
	return controller.RadiosRoute()
}

func (a *RadiosPage) Reload() {
	go a.load(false, 0)
}

func (a *RadiosPage) Save() SavedPage {
	return &savedrRadiosPage{
		contr:      a.contr,
		rp:         a.rp,
		pm:         a.pm,
		searchText: a.searcher.Entry.Text,
		scrollPos:  a.list.list.GetScrollOffset(),
	}
}

type savedrRadiosPage struct {
	contr      *controller.Controller
	rp         mediaprovider.RadioProvider
	pm         *backend.PlaybackManager
	searchText string
	scrollPos  float32
}

func (s *savedrRadiosPage) Restore() Page {
	return newRadiosPage(s.contr, s.rp, s.pm, s.searchText, s.scrollPos)
}

func (a *RadiosPage) buildContainer() {
	searchVbox := container.NewVBox(layout.NewSpacer(), a.searcher, layout.NewSpacer())
	a.container = container.New(&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, TopPadding: 5, BottomPadding: 15},
		container.NewBorder(
			container.New(&layout.CustomPaddedLayout{LeftPadding: -5},
				container.NewHBox(a.titleDisp, layout.NewSpacer(), searchVbox)),
			nil, nil, nil,
			container.NewStack(a.noRadiosMsg, a.list)),
	)
}

func (a *RadiosPage) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.container)
}

type RadioList struct {
	widget.BaseWidget

	OnPlay  func(*mediaprovider.RadioStation)
	OnQueue func(r *mediaprovider.RadioStation, next bool)

	radios   []*mediaprovider.RadioStation
	selected *RadioListRow

	columnsLayout *layouts.ColumnsLayout
	hdr           *widgets.ListHeader
	list          *widgets.FocusList
	container     *fyne.Container
	playingIcon   fyne.CanvasObject
	menu          *widget.PopUpMenu
}

type RadioListRow struct {
	widgets.FocusListRowBase

	Item              *mediaprovider.RadioStation
	IsPlaying         bool
	OnTappedSecondary func(*fyne.PointEvent)

	nameLabel    *widget.RichText
	homePageLink *widget.Hyperlink
}

func NewRadioListRow(layout *layouts.ColumnsLayout) *RadioListRow {
	a := &RadioListRow{
		nameLabel:    widget.NewRichTextWithText(""),
		homePageLink: widget.NewHyperlink("", nil),
	}
	a.ExtendBaseWidget(a)
	a.nameLabel.Truncation = fyne.TextTruncateEllipsis
	a.homePageLink.Truncation = fyne.TextTruncateEllipsis
	a.Content = container.New(layout, a.nameLabel, a.homePageLink)
	return a
}

func (a *RadioListRow) TappedSecondary(e *fyne.PointEvent) {
	if a.OnTappedSecondary != nil {
		a.OnTappedSecondary(e)
	}
}

func NewRadioList(nowPlayingIDPtr *string) *RadioList {
	a := &RadioList{
		columnsLayout: layouts.NewColumnsLayout([]float32{-1, -1}),
	}
	playIcon := theme.NewThemedResource(theme.MediaPlayIcon())
	playIcon.ColorName = theme.ColorNamePrimary
	a.playingIcon = container.NewCenter(widget.NewIcon(playIcon))
	a.ExtendBaseWidget(a)
	a.hdr = widgets.NewListHeader([]widgets.ListColumn{
		{Text: lang.L("Name"), Alignment: fyne.TextAlignLeading, CanToggleVisible: false},
		{Text: lang.L("Home Page"), Alignment: fyne.TextAlignLeading, CanToggleVisible: false},
	},
		a.columnsLayout)
	a.hdr.DisableSorting = true
	a.list = widgets.NewFocusList(
		func() int { return len(a.radios) },
		func() fyne.CanvasObject {
			r := NewRadioListRow(a.columnsLayout)
			r.OnTapped = func() {
				r.Selected = true
				if a.selected != nil {
					// unselect old row
					a.selected.Selected = false
					a.selected.Refresh()
				}
				a.selected = r
				r.Refresh()
			}
			r.OnDoubleTapped = func() { a.onPlayRadio(r.Item) }
			r.OnTappedSecondary = func(e *fyne.PointEvent) {
				r.OnTapped() // handle selection
				a.showMenu(e.AbsolutePosition)
			}
			r.OnFocusNeighbor = func(up bool) {
				a.list.FocusNeighbor(r.ItemID(), up)
			}
			return r
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			row := item.(*RadioListRow)
			changed := false
			if row.Item != a.radios[id] {
				row.EnsureUnfocused()
				row.ListItemID = id
				row.Item = a.radios[id]
				row.nameLabel.Segments[0].(*widget.TextSegment).Text = row.Item.StationName
				row.homePageLink.Text = row.Item.HomePageURL
				if u, err := url.Parse(row.Item.HomePageURL); err == nil {
					row.homePageLink.URL = u
				}
				changed = true
			}
			isPlaying := *nowPlayingIDPtr == row.Item.ID
			if row.IsPlaying != isPlaying {
				row.IsPlaying = isPlaying
				row.nameLabel.Segments[0].(*widget.TextSegment).Style.TextStyle.Bold = isPlaying
				if isPlaying {
					row.Content.(*fyne.Container).Objects[0] = container.NewBorder(nil, nil, a.playingIcon, nil,
						container.New(layout.NewCustomPaddedLayout(0, 0, -5, 0), row.nameLabel))
				} else {
					row.Content.(*fyne.Container).Objects[0] = row.nameLabel
				}
				changed = true
			}
			if changed {
				row.Refresh()
			}
		},
	)
	a.container = container.NewBorder(a.hdr, nil, nil, nil, a.list)
	return a
}

func (a *RadioList) showMenu(pos fyne.Position) {
	if a.menu == nil {
		play := fyne.NewMenuItem(lang.L("Play"), func() {
			if a.OnPlay != nil {
				a.OnPlay(a.selected.Item)
			}
		})
		play.Icon = theme.MediaPlayIcon()

		playNext := fyne.NewMenuItem(lang.L("Play next"), func() {
			if a.OnQueue != nil {
				a.OnQueue(a.selected.Item, true)
			}
		})
		playNext.Icon = myTheme.PlayNextIcon

		append := fyne.NewMenuItem(lang.L("Add to queue"), func() {
			if a.OnQueue != nil {
				a.OnQueue(a.selected.Item, false)
			}
		})
		append.Icon = theme.ContentAddIcon()

		a.menu = widget.NewPopUpMenu(fyne.NewMenu("",
			play,
			playNext,
			append,
		),
			fyne.CurrentApp().Driver().CanvasForObject(a),
		)
	}
	a.menu.ShowAtPosition(pos)
}

func (g *RadioList) SetRadios(radios []*mediaprovider.RadioStation) {
	g.radios = radios
	g.Refresh()
}

func (a *RadioList) onPlayRadio(item *mediaprovider.RadioStation) {
	if a.OnPlay != nil {
		a.OnPlay(item)
	}
}

func (a *RadioList) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.container)
}
