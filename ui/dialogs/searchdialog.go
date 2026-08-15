package dialogs

import (
	"fmt"
	"image"
	"log"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"

	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"

	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
	"github.com/supersonic-app/supersonic/ui/widgets"
)

// SearchDialog is a base widget to be built upon for creating custom search dialogs
type SearchDialog struct {
	widget.BaseWidget

	PlaceholderText string

	// Additional item that can be placed to the left
	// of the dismiss buttons
	ActionItem fyne.CanvasObject

	OnDismiss         func()
	OnNavigateTo      func(mediaprovider.ContentType, string)
	OnShowContextMenu func(itemIdx int, pos fyne.Position)
	OnSearched        func(string) []*mediaprovider.SearchResult

	imgSource                util.ImageFetcher
	resultsMutex             sync.RWMutex
	resultsGeneration        int
	pendingResultsGeneration int
	searchResults            []*mediaprovider.SearchResult
	selectedIndex            int

	searchEntry *searchEntry
	loadingDots *widgets.LoadingDots
	list        *widget.List
	dialogTitle string
	dismissText string
	content     *fyne.Container
}

func NewSearchDialog(im util.ImageFetcher, title, dismissBtn string, onSearched func(string) []*mediaprovider.SearchResult) *SearchDialog {
	sd := &SearchDialog{
		imgSource:   im,
		loadingDots: widgets.NewLoadingDots(),
		OnSearched:  onSearched,
		dialogTitle: title,
		dismissText: dismissBtn,
	}
	sd.ExtendBaseWidget(sd)

	se := newSearchEntry()
	se.OnSearched = sd.onSearched
	se.OnSubmitted = func(_ string) {
		sd.onSelected(sd.selectedIndex)
	}
	se.OnTypedDown = sd.moveSelectionDown
	se.OnTypedUp = sd.moveSelectionUp
	se.OnTypedEscape = sd.onDismiss
	sd.searchEntry = se
	sd.list = widget.NewList(
		func() int {
			sd.resultsMutex.RLock()
			defer sd.resultsMutex.RUnlock()
			return len(sd.searchResults)
		},
		func() fyne.CanvasObject { return newSearchResult(sd) },
		func(lii widget.ListItemID, co fyne.CanvasObject) {
			var result *mediaprovider.SearchResult
			sd.resultsMutex.RLock()
			if len(sd.searchResults) > lii {
				result = sd.searchResults[lii]
			}
			sd.resultsMutex.RUnlock()
			sr := co.(*searchResult)
			sr.index = lii
			sr.Update(result)
		},
	)
	sd.list.HideSeparators = true
	return sd
}

// GetSearchEntry returns the search Entry widget for focusing
func (sd *SearchDialog) GetSearchEntry() fyne.Focusable {
	return sd.searchEntry
}

// SearchQuery returns the current search query entered by the user
func (sd *SearchDialog) SearchQuery() string {
	return sd.searchEntry.Text
}

func (sd *SearchDialog) Show() {
	sd.BaseWidget.Show()
	sd.onSearched("")
}

func (sd *SearchDialog) Refresh() {
	if sd.PlaceholderText != "" {
		sd.searchEntry.SetPlaceHolder(sd.PlaceholderText)
	}
	sd.BaseWidget.Refresh()
}

func (sd *SearchDialog) onDismiss() {
	if sd.OnDismiss != nil {
		sd.OnDismiss()
	}
}

func (sd *SearchDialog) onSelected(idx int) {
	if sd.OnNavigateTo == nil {
		return
	}
	sd.resultsMutex.RLock()
	if len(sd.searchResults) <= idx {
		sd.resultsMutex.RUnlock()
		return
	}
	id := sd.searchResults[idx].ID
	typ := sd.searchResults[idx].Type
	sd.resultsMutex.RUnlock()
	sd.OnNavigateTo(typ, id)
}

func (sd *SearchDialog) moveSelectionDown() {
	sd.resultsMutex.RLock()
	if sd.selectedIndex < len(sd.searchResults)-1 {
		sd.selectedIndex++
	}
	sd.resultsMutex.RUnlock()
	sd.list.Select(sd.selectedIndex)
}

func (sd *SearchDialog) moveSelectionUp() {
	sd.resultsMutex.RLock()
	if sd.selectedIndex > 0 {
		sd.selectedIndex--
	}
	sd.resultsMutex.RUnlock()
	sd.list.Select(sd.selectedIndex)
}

func (sd *SearchDialog) setResults(results []*mediaprovider.SearchResult) {
	sd.resultsMutex.Lock()
	sd.searchResults = results
	sd.resultsMutex.Unlock()
	sd.list.Refresh()
	sd.list.ScrollToTop()
	sd.selectedIndex = 0
	sd.list.Select(0)
}

func (sd *SearchDialog) onSearched(query string) {
	sd.loadingDots.Start()
	var results []*mediaprovider.SearchResult
	sd.pendingResultsGeneration += 1
	var generation = sd.pendingResultsGeneration
	go func() {
		res := sd.OnSearched(query)
		if len(res) == 0 {
			log.Println("No results matched the query.")
		} else {
			results = res
		}
		if sd.resultsGeneration < generation {
			fyne.Do(func() {
				if sd.pendingResultsGeneration == generation {
					sd.loadingDots.Stop()
				}
				sd.setResults(results)
			})
			sd.resultsGeneration = generation
		}
	}()
}

func (sd *SearchDialog) CreateRenderer() fyne.WidgetRenderer {
	dismissBtn := widget.NewButton(sd.dismissText, sd.onDismiss)
	title := widget.NewRichText(&widget.TextSegment{Text: sd.dialogTitle, Style: util.BoldRichTextStyle})
	title.Segments[0].(*widget.TextSegment).Style.Alignment = fyne.TextAlignCenter
	bottomRow := container.NewHBox()
	if sd.ActionItem != nil {
		bottomRow.Objects = []fyne.CanvasObject{sd.ActionItem, layout.NewSpacer(), dismissBtn}
	} else {
		bottomRow.Objects = []fyne.CanvasObject{layout.NewSpacer(), dismissBtn}
	}
	sd.content = container.NewStack(
		container.NewBorder(
			container.NewVBox(title,
				container.New(layout.NewCustomPaddedLayout(0, 0, 2, 2),
					sd.searchEntry)),
			container.NewVBox(widget.NewSeparator(), bottomRow),
			nil, nil,
			container.New(layout.NewCustomPaddedLayout(0, 0, 4, 4), sd.list)),
		container.NewCenter(sd.loadingDots),
	)

	return widget.NewSimpleRenderer(sd.content)
}

func (sd *SearchDialog) MinSize() fyne.Size {
	return fyne.NewSize(400, 350)
}

func placeholderIconForContentType(c mediaprovider.ContentType) fyne.Resource {
	switch c {
	case mediaprovider.ContentTypeAlbum:
		return myTheme.AlbumIcon
	case mediaprovider.ContentTypeArtist:
		return myTheme.ArtistIcon
	case mediaprovider.ContentTypeTrack:
		return myTheme.TracksIcon
	case mediaprovider.ContentTypeGenre:
		return myTheme.GenreIcon
	case mediaprovider.ContentTypePlaylist:
		return myTheme.PlaylistIcon
	case mediaprovider.ContentTypeRadioStation:
		return myTheme.RadioIcon
	default:
		return theme.WarningIcon() // unreached
	}
}

type searchResult struct {
	widget.BaseWidget

	parent *SearchDialog

	id          string
	index       int
	contentType mediaprovider.ContentType

	imageLoader util.ThumbnailLoader

	image     *widgets.ImagePlaceholder
	title     *widget.Label
	secondary *widget.RichText

	content *fyne.Container
}

func newSearchResult(parent *SearchDialog) *searchResult {
	qs := &searchResult{
		parent:    parent,
		image:     widgets.NewImagePlaceholder(myTheme.AlbumIcon, 50),
		title:     widget.NewLabel(""),
		secondary: widget.NewRichText(),
	}
	qs.title.Truncation = fyne.TextTruncateEllipsis
	qs.secondary.Truncation = fyne.TextTruncateEllipsis
	qs.ExtendBaseWidget(qs)
	qs.imageLoader = util.NewThumbnailLoader(parent.imgSource, func(im image.Image) {
		qs.image.SetImage(im, false)
	})
	qs.imageLoader.OnBeforeLoad = func() {
		qs.image.SetImage(nil, false)
	}

	return qs
}

func (s *searchResult) Update(result *mediaprovider.SearchResult) {
	if result == nil {
		return
	}
	if s.contentType == result.Type && s.id == result.ID && s.title.Text == result.Name {
		return // nothing to do
	}
	s.id = result.ID
	s.contentType = result.Type
	if result.Icon != nil {
		s.image.PlaceholderIcon = result.Icon
	} else {
		s.image.PlaceholderIcon = placeholderIconForContentType(result.Type)
	}
	s.imageLoader.Load(result.CoverID)
	s.title.SetText(result.Name)

	maybePluralize := func(key string, size int) string {
		if size != 1 {
			return lang.L(key + "s")
		}
		return lang.L(key)
	}
	var secondaryText string
	switch result.Type {
	case mediaprovider.ContentTypeAlbum:
		secondaryText = result.ArtistName
	case mediaprovider.ContentTypeArtist:
		secondaryText = fmt.Sprintf("%d %s", result.Size, maybePluralize("album", result.Size))
	case mediaprovider.ContentTypeTrack:
		secondaryText = result.ArtistName
	case mediaprovider.ContentTypePlaylist:
		secondaryText = fmt.Sprintf("%d %s", result.Size, maybePluralize("track", result.Size))
	case mediaprovider.ContentTypeRadioStation:
		fallthrough // result.Size is always 0 for radios
	case mediaprovider.ContentTypeGenre:
		if result.Size > 0 {
			secondaryText = fmt.Sprintf("%d %s", result.Size, maybePluralize("album", result.Size))
		} else {
			secondaryText = ""
		}
	case mediaprovider.ContentTypeOther:
		secondaryText = result.ArtistName
	}

	if result.Type == mediaprovider.ContentTypeOther {
		s.secondary.Segments = []widget.RichTextSegment{}
	} else {
		s.secondary.Segments = []widget.RichTextSegment{
			&widget.TextSegment{
				Text:  result.Type.String(),
				Style: widget.RichTextStyle{SizeName: theme.SizeNameCaptionText, TextStyle: fyne.TextStyle{Bold: true}, Inline: true},
			},
		}
	}
	if secondaryText != "" {
		if len(s.secondary.Segments) > 0 {
			s.secondary.Segments = append(s.secondary.Segments,
				&widget.TextSegment{
					Text:  " · ",
					Style: widget.RichTextStyle{SizeName: theme.SizeNameCaptionText, Inline: true},
				})
		}
		s.secondary.Segments = append(s.secondary.Segments,
			&widget.TextSegment{
				Text:  secondaryText,
				Style: widget.RichTextStyle{SizeName: theme.SizeNameCaptionText, Inline: true},
			},
		)
	}

	s.secondary.Refresh()
}

func (q *searchResult) Tapped(_ *fyne.PointEvent) {
	q.parent.onSelected(q.index)
}

func (q *searchResult) TappedSecondary(e *fyne.PointEvent) {
	if q.parent.OnShowContextMenu != nil {
		q.parent.OnShowContextMenu(q.index, e.AbsolutePosition)
	}
}

func (q *searchResult) CreateRenderer() fyne.WidgetRenderer {
	if q.content == nil {
		q.content = container.NewBorder(nil, nil, container.NewCenter(q.image), nil,
			container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-15),
				q.title,
				q.secondary,
			))
	}
	return widget.NewSimpleRenderer(q.content)
}

func (q *searchResult) Refresh() {
	q.BaseWidget.Refresh()
}

type searchEntry struct {
	widgets.SearchEntry

	OnTypedUp     func()
	OnTypedDown   func()
	OnTypedEscape func()
}

func newSearchEntry() *searchEntry {
	q := &searchEntry{}
	q.ExtendBaseWidget(q)
	q.SearchEntry.Init()
	return q
}

func (q *searchEntry) TypedKey(e *fyne.KeyEvent) {
	switch {
	case e.Name == fyne.KeyUp && q.OnTypedUp != nil:
		q.OnTypedUp()
	case e.Name == fyne.KeyDown && q.OnTypedDown != nil:
		q.OnTypedDown()
	case e.Name == fyne.KeyEscape && q.OnTypedEscape != nil:
		q.OnTypedEscape()
	default:
		q.SearchEntry.TypedKey(e)
	}
}
