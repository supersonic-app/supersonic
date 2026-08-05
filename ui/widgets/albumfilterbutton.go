package widgets

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
)

type AlbumFilterButton struct {
	ttwidget.Button

	OnChanged        func()
	GenreDisabled    bool
	FavoriteDisabled bool

	genreListChan chan []string

	filter mediaprovider.AlbumFilter
	dialog *widget.PopUp
}

func NewAlbumFilterButton(filter mediaprovider.AlbumFilter, fetchGenresFunc func() ([]*mediaprovider.Genre, error)) *AlbumFilterButton {
	a := &AlbumFilterButton{
		filter: filter,
		Button: ttwidget.Button{
			Button: widget.Button{
				Icon: theme.NewThemedResource(myTheme.FilterIcon),
			},
		},
	}
	a.SetToolTip(lang.L("Filter albums"))
	a.OnTapped = a.showFilterDialog
	a.ExtendBaseWidget(a)
	a.genreListChan = make(chan []string)
	go func() {
		if genres, err := fetchGenresFunc(); err == nil {
			genreNames := sharedutil.MapSlice(genres, func(g *mediaprovider.Genre) string {
				return g.Name
			})
			slices.Sort(genreNames)
			a.genreListChan <- genreNames
		}
	}()
	return a
}

func (a *AlbumFilterButton) Refresh() {
	themedIcon := a.Icon.(*theme.ThemedResource)
	if a.filterEmpty() {
		themedIcon.ColorName = theme.ColorNameForeground
	} else {
		themedIcon.ColorName = theme.ColorNamePrimary
	}
	a.Button.Refresh()
}

func (a *AlbumFilterButton) Filter() mediaprovider.AlbumFilter {
	return a.filter
}

func (a *AlbumFilterButton) SetOnChanged(fn func()) {
	a.OnChanged = fn
}

func (a *AlbumFilterButton) filterEmpty() bool {
	filterOptions := a.filter.Options()
	return filterOptions.MinYear == 0 && filterOptions.MaxYear == 0 &&
		(a.FavoriteDisabled || !filterOptions.ExcludeFavorited && !filterOptions.ExcludeUnfavorited) &&
		(a.GenreDisabled || len(filterOptions.Genres) == 0)
}

func (a *AlbumFilterButton) onFilterChanged() {
	a.Refresh()
	if a.OnChanged != nil {
		a.OnChanged()
	}
}

func (a *AlbumFilterButton) showFilterDialog() {
	if a.dialog == nil {
		filterDlg := NewAlbumFilterPopup(a)
		filterDlg.OnChanged = a.onFilterChanged
		cnv := fyne.CurrentApp().Driver().CanvasForObject(a)
		if cnv == nil {
			return
		}
		a.dialog = widget.NewPopUp(container.NewPadded(filterDlg), cnv)
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(a)
	a.dialog.ShowAtPosition(fyne.NewPos(pos.X+a.Size().Width/2-a.dialog.MinSize().Width/2, pos.Y+a.Size().Height))
}

type AlbumFilterPopup struct {
	widget.BaseWidget

	OnChanged func()

	isFavorite    *widget.Check
	isNotFavorite *widget.Check
	genreFilter   *GenreFilterSubsection
	filterBtn     *AlbumFilterButton
	container     *fyne.Container
}

func NewAlbumFilterPopup(filter *AlbumFilterButton) *AlbumFilterPopup {
	a := &AlbumFilterPopup{filterBtn: filter}
	a.ExtendBaseWidget(a)

	debounceOnChanged := util.NewDebouncer(350*time.Millisecond, a.emitOnChanged)

	// setup min and max year filters
	yearValidator := func(curText, selText string, r rune) bool {
		l := len(curText) - len(selText)
		return unicode.IsDigit(r) && l <= 3 && (l > 0 || r != '0')
	}
	minYear := NewTextRestrictedEntry(yearValidator)
	minYear.SetMinCharWidth(4)
	minYear.OnChanged = func(yearStr string) {
		filterOptions := a.filterBtn.filter.Options()
		if yearStr == "" {
			filterOptions.MinYear = 0
		} else if i, err := strconv.Atoi(yearStr); err == nil {
			filterOptions.MinYear = i
		}
		a.filterBtn.filter.SetOptions(filterOptions)
		debounceOnChanged()
	}
	filterOptions := a.filterBtn.filter.Options()
	if filterOptions.MinYear > 0 {
		minYear.Text = strconv.Itoa(filterOptions.MinYear)
	}
	maxYear := NewTextRestrictedEntry(yearValidator)
	maxYear.SetMinCharWidth(4)
	maxYear.OnChanged = func(yearStr string) {
		filterOptions := a.filterBtn.filter.Options()
		if yearStr == "" {
			filterOptions.MaxYear = 0
		} else if i, err := strconv.Atoi(yearStr); err == nil {
			filterOptions.MaxYear = i
		}
		a.filterBtn.filter.SetOptions(filterOptions)
		debounceOnChanged()
	}
	if filterOptions.MaxYear > 0 {
		maxYear.Text = strconv.Itoa(filterOptions.MaxYear)
	}

	// setup is favorite/not favorite filters
	a.isFavorite = widget.NewCheck(lang.L("Is favorite"), func(fav bool) {
		filterOptions := a.filterBtn.filter.Options()
		if fav {
			a.isNotFavorite.SetChecked(false)
		}
		filterOptions.ExcludeUnfavorited = fav
		a.filterBtn.filter.SetOptions(filterOptions)
		debounceOnChanged()
	})
	a.isFavorite.Hidden = a.filterBtn.FavoriteDisabled
	a.isNotFavorite = widget.NewCheck(lang.L("Is not favorite"), func(fav bool) {
		filterOptions := a.filterBtn.filter.Options()
		if fav {
			a.isFavorite.SetChecked(false)
		}
		filterOptions.ExcludeFavorited = fav
		a.filterBtn.filter.SetOptions(filterOptions)
		debounceOnChanged()
	})
	a.isNotFavorite.Hidden = a.filterBtn.FavoriteDisabled

	// create genre filter subsection
	a.genreFilter = NewGenreFilterSubsection(func(selectedGenres []string) {
		filterOptions := a.filterBtn.filter.Options()
		filterOptions.Genres = selectedGenres
		a.filterBtn.filter.SetOptions(filterOptions)
		debounceOnChanged()
	}, filterOptions.Genres)
	a.genreFilter.Hidden = a.filterBtn.GenreDisabled

	// setup container
	title := widget.NewLabel(lang.L("Album filters"))
	title.TextStyle.Bold = true
	a.container = container.NewVBox(
		container.NewHBox(layout.NewSpacer(), title, layout.NewSpacer()),
		container.NewHBox(widget.NewLabel(lang.L("Year from")), minYear, widget.NewLabel(lang.L("to")), maxYear),
		container.NewHBox(a.isFavorite, a.isNotFavorite),
		a.genreFilter,
	)

	go func() {
		genres := <-a.filterBtn.genreListChan
		fyne.Do(func() {
			a.genreFilter.SetGenreList(genres)
		})
	}()

	return a
}

func (a *AlbumFilterPopup) Tapped(_ *fyne.PointEvent) {
	// swallow the Tapped event so that the popup is
	// only dismissed by clicking outside of it
}

func (a *AlbumFilterPopup) Refresh() {
	a.isFavorite.Hidden = a.filterBtn.FavoriteDisabled
	a.isNotFavorite.Hidden = a.filterBtn.FavoriteDisabled
	a.genreFilter.Hidden = a.filterBtn.GenreDisabled
	a.BaseWidget.Refresh()
}

func (a *AlbumFilterPopup) emitOnChanged() {
	if a.OnChanged != nil {
		a.OnChanged()
	}
}

func (a *AlbumFilterPopup) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.container)
}

type GenreFilterSubsection struct {
	widget.BaseWidget

	genreList []string
	onChanged func([]string)

	selectedGenres      map[string]any
	selectedGenresMutex sync.RWMutex

	filterText         *widget.Entry
	genresTitleLine    *widget.RichText
	allBtn             *widget.Button
	noneBtn            *widget.Button
	listModelMutex     sync.RWMutex
	genreListViewModel []string
	genreListView      *FocusList

	container *fyne.Container
}

func NewGenreFilterSubsection(onChanged func([]string), initialSelectedGenres []string) *GenreFilterSubsection {
	g := &GenreFilterSubsection{
		onChanged:      onChanged,
		selectedGenres: make(map[string]any),
	}
	g.ExtendBaseWidget(g)

	for _, genre := range initialSelectedGenres {
		g.selectedGenres[genre] = nil
	}

	g.genreListView = NewFocusList(
		func() int {
			g.listModelMutex.RLock()
			defer g.listModelMutex.RUnlock()
			return len(g.genreListViewModel)
		},
		func() fyne.CanvasObject {
			row := newGenreListViewRow(g.onGenreSelected)
			row.OnFocusNeighbor = func(up bool) {
				g.genreListView.FocusNeighbor(row.ItemID(), up)
			}
			return row
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			g.listModelMutex.RLock()
			defer g.listModelMutex.RUnlock()
			genre := g.genreListViewModel[id]
			g.selectedGenresMutex.RLock()
			_, selected := g.selectedGenres[genre]
			g.selectedGenresMutex.RUnlock()
			row := obj.(*genreListViewRow)
			row.ListItemID = id
			row.Content.(*widget.Check).Text = genre
			row.Content.(*widget.Check).Checked = selected
			row.Refresh()
		},
	)
	g.filterText = widget.NewEntry()
	g.filterText.SetPlaceHolder(lang.L("Filter genres"))
	i := NewTappableIcon(theme.ContentClearIcon())
	i.NoPointerCursor = true
	i.OnTapped = func() { g.filterText.SetText("") }
	g.filterText.ActionItem = i
	debouncer := util.NewDebouncer(300*time.Millisecond, g.updateGenreListView)
	g.filterText.OnChanged = func(_ string) {
		debouncer()
	}
	g.allBtn = widget.NewButton(lang.L("All"), func() { g.selectAllOrNoneInView(false) })
	g.noneBtn = widget.NewButton(lang.L("None"), func() { g.selectAllOrNoneInView(true) })

	g.genresTitleLine = widget.NewRichText(
		&widget.TextSegment{
			Text: lang.L("Genres") + "  ",
			Style: widget.RichTextStyle{
				Inline:    true,
				TextStyle: fyne.TextStyle{Bold: true},
			},
		},
		&widget.TextSegment{Text: fmt.Sprintf("(%s)", lang.L("none selected"))},
	)
	g.updateNumSelectedText()

	filterRow := container.NewBorder(nil, nil, nil, container.NewHBox(g.allBtn, g.noneBtn), g.filterText)
	g.container = container.NewBorder(g.genresTitleLine, nil, nil, nil,
		container.New(&layout.CustomPaddedLayout{LeftPadding: 5, RightPadding: 5},
			container.NewBorder(filterRow, nil, nil, nil, g.genreListView),
		),
	)
	return g
}

func (g *GenreFilterSubsection) MinSize() fyne.Size {
	return fyne.NewSize(g.BaseWidget.MinSize().Width, 250)
}

func (g *GenreFilterSubsection) SetGenreList(genres []string) {
	g.genreList = genres
	g.updateGenreListView()
}

func (g *GenreFilterSubsection) updateGenreListView() {
	g.listModelMutex.Lock()
	if g.filterText.Text == "" {
		g.genreListViewModel = g.genreList
	} else {
		filterText := strings.ToLower(g.filterText.Text)
		g.genreListViewModel = sharedutil.FilterSlice(g.genreList, func(genre string) bool {
			return strings.Contains(strings.ToLower(genre), filterText)
		})
	}
	g.listModelMutex.Unlock()
	g.genreListView.Refresh()
}

func (g *GenreFilterSubsection) onGenreSelected(row widget.ListItemID, selected bool) {
	g.listModelMutex.RLock()
	g.selectedGenresMutex.Lock()
	if selected {
		g.selectedGenres[g.genreListViewModel[row]] = nil
	} else {
		delete(g.selectedGenres, g.genreListViewModel[row])
	}
	g.selectedGenresMutex.Unlock()
	g.listModelMutex.RUnlock()
	g.invokeOnChanged()
}

func (g *GenreFilterSubsection) selectAllOrNoneInView(none bool) {
	g.listModelMutex.RLock()
	g.selectedGenresMutex.Lock()
	for _, genre := range g.genreListViewModel {
		if none {
			delete(g.selectedGenres, genre)
		} else {
			g.selectedGenres[genre] = nil
		}
	}
	g.selectedGenresMutex.Unlock()
	g.listModelMutex.RUnlock()
	g.genreListView.Refresh()
	g.invokeOnChanged()
}

func (g *GenreFilterSubsection) invokeOnChanged() {
	g.selectedGenresMutex.RLock()
	g.updateNumSelectedText()
	genres := make([]string, 0, len(g.selectedGenres))
	for genre := range g.selectedGenres {
		genres = append(genres, genre)
	}
	g.selectedGenresMutex.RUnlock()
	g.onChanged(genres)
}

func (g *GenreFilterSubsection) updateNumSelectedText() {
	numText := lang.L("none")
	if l := len(g.selectedGenres); l > 0 {
		numText = strconv.Itoa(l)
	}
	t := fmt.Sprintf("(%s %s)", numText, lang.L("selected"))
	g.genresTitleLine.Segments[1].(*widget.TextSegment).Text = t
	g.genresTitleLine.Refresh()
}

func (g *GenreFilterSubsection) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.container)
}

type genreListViewRow struct {
	FocusListRowBase
}

func newGenreListViewRow(onChanged func(widget.ListItemID, bool)) *genreListViewRow {
	g := &genreListViewRow{}
	g.ExtendBaseWidget(g)
	g.Content = widget.NewCheck("", func(b bool) {
		onChanged(g.ItemID(), b)
	})
	g.OnTapped = func() {
		chk := g.Content.(*widget.Check)
		chk.SetChecked(!chk.Checked)
	}
	return g
}
