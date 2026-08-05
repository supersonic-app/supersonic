package browsing

import (
	"log"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/controller"
	"github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
	"github.com/supersonic-app/supersonic/ui/widgets"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type TracksPage struct {
	widget.BaseWidget

	tracksPageState

	nowPlayingID string

	title           *widget.RichText
	searcher        *widgets.SearchEntry
	tracklist       *widgets.Tracklist
	loader          *widgets.TracklistLoader
	searchTracklist *widgets.Tracklist
	searchLoader    *widgets.TracklistLoader
	playRandom      *widget.Button
	container       *fyne.Container
}

type tracksPageState struct {
	searchText string
	widgetPool *util.WidgetPool
	contr      *controller.Controller
	conf       *backend.TracksPageConfig
	mp         mediaprovider.MediaProvider
	im         *backend.ImageManager
	canRate    bool
	canShare   bool
}

func NewTracksPage(contr *controller.Controller, conf *backend.TracksPageConfig, pool *util.WidgetPool, mp mediaprovider.MediaProvider, im *backend.ImageManager) *TracksPage {
	t := &TracksPage{tracksPageState: tracksPageState{contr: contr, conf: conf, widgetPool: pool, mp: mp, im: im}}
	t.ExtendBaseWidget(t)

	t.tracklist = t.obtainTracklist()
	_, t.canRate = mp.(mediaprovider.SupportsRating)
	_, t.canShare = mp.(mediaprovider.SupportsSharing)
	t.tracklist.Options = widgets.TracklistOptions{
		DisableSorting: true,
		DisableRating:  !t.canRate,
		DisableSharing: !t.canShare,
		AutoNumber:     true,
	}
	t.tracklist.SetVisibleColumns(conf.TracklistColumns)
	t.tracklist.OnVisibleColumnsChanged = func(cols []string) {
		t.conf.TracklistColumns = cols
		if t.searchTracklist != nil {
			t.searchTracklist.SetVisibleColumns(cols)
		}
	}
	contr.ConnectTracklistActions(t.tracklist)

	t.title = widget.NewRichTextWithText(lang.L("All Tracks"))
	t.title.Segments[0].(*widget.TextSegment).Style.SizeName = widget.RichTextStyleHeading.SizeName
	t.playRandom = widget.NewButtonWithIcon(lang.L("Play random"), theme.ShuffleIcon, t.playRandomSongs)
	t.searcher = widgets.NewSearchEntry()
	t.searcher.PlaceHolder = lang.L("Search page")
	t.searcher.OnSearched = t.OnSearched
	t.createContainer()
	t.Reload()
	return t
}

func (t *TracksPage) createContainer() {
	playRandomVbox := container.NewVBox(layout.NewSpacer(), t.playRandom, layout.NewSpacer())
	searchVbox := container.NewVBox(layout.NewSpacer(), t.searcher, layout.NewSpacer())
	topRow := container.NewHBox(t.title, playRandomVbox, layout.NewSpacer(), searchVbox)
	t.container = container.New(&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, TopPadding: 5, BottomPadding: 15},
		container.NewBorder(topRow, nil, nil, nil, t.tracklist))
}

func (t *TracksPage) Route() controller.Route {
	return controller.TracksRoute()
}

var _ CanSelectAll = (*TracksPage)(nil)

func (t *TracksPage) SelectAll() {
	// deliberate no-op since we don't want to give the impression
	// that you can select all tracks from the server, since only
	// some of them are actually loaded into the model
}

func (t *TracksPage) UnselectAll() {
	t.currentTracklist().UnselectAll()
}

func (t *TracksPage) Reload() {
	t.tracklist.Clear()
	iter := t.mp.IterateTracks("")
	// loads asynchronously
	t.loader = widgets.NewTracklistLoader(t.tracklist, iter)
}

var _ CanShowNowPlaying = (*TracksPage)(nil)

func (t *TracksPage) OnSongChange(item mediaprovider.MediaItem, lastScrobbledIfAny *mediaprovider.Track) {
	t.nowPlayingID = sharedutil.MediaItemIDOrEmptyStr(item)
	t.tracklist.SetNowPlaying(t.nowPlayingID)
	if t.searchTracklist != nil {
		t.searchTracklist.SetNowPlaying(t.nowPlayingID)
	}
	playedID := sharedutil.MediaItemIDOrEmptyStr(lastScrobbledIfAny)
	t.tracklist.IncrementPlayCount(playedID)
	if t.searchTracklist != nil {
		t.searchTracklist.IncrementPlayCount(playedID)
	}
}

var _ Scrollable = (*TracksPage)(nil)

func (g *TracksPage) Scroll(scrollAmt float32) {
	g.tracklist.ScrollBy(scrollAmt)
}

var _ Searchable = (*TracksPage)(nil)

func (t *TracksPage) SearchWidget() fyne.Focusable {
	return t.searcher
}

func (t *TracksPage) OnSearched(query string) {
	t.searchText = query
	if query == "" {
		t.container.Objects[0].(*fyne.Container).Objects[0] = t.tracklist
		if t.searchTracklist != nil {
			t.searchTracklist.Clear()
		}
		t.Refresh()
		return
	}
	t.doSearch(query)
}

func (t *TracksPage) doSearch(query string) {
	if t.searchTracklist == nil {
		t.searchTracklist = t.obtainTracklist()
		t.searchTracklist.Options = widgets.TracklistOptions{
			AutoNumber:     true,
			DisableSorting: true,
			DisableRating:  !t.canRate,
			DisableSharing: !t.canShare,
		}
		t.searchTracklist.SetVisibleColumns(t.conf.TracklistColumns)
		t.searchTracklist.SetNowPlaying(t.nowPlayingID)
		t.searchTracklist.OnVisibleColumnsChanged = func(cols []string) {
			t.conf.TracklistColumns = cols
			t.tracklist.SetVisibleColumns(cols)
		}
		t.contr.ConnectTracklistActions(t.searchTracklist)
	} else {
		t.searchTracklist.Clear()
	}
	iter := t.mp.IterateTracks(query)
	t.searchLoader = widgets.NewTracklistLoader(t.searchTracklist, iter)
	t.container.Objects[0].(*fyne.Container).Objects[0] = t.searchTracklist
	t.Refresh()
}

func (t *TracksPage) currentTracklist() *widgets.Tracklist {
	return t.container.Objects[0].(*fyne.Container).Objects[0].(*widgets.Tracklist)
}

func (t *TracksPage) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.container)
}

func (t *TracksPage) Save() SavedPage {
	t.loader.Dispose()
	t.tracklist.Clear()
	t.widgetPool.Release(util.WidgetTypeTracklist, t.tracklist)
	if t.searchTracklist != nil {
		t.searchLoader.Dispose()
		t.searchTracklist.Clear()
		t.widgetPool.Release(util.WidgetTypeTracklist, t.searchTracklist)
	}
	state := t.tracksPageState
	return &state
}

func (s *tracksPageState) Restore() Page {
	t := NewTracksPage(s.contr, s.conf, s.widgetPool, s.mp, s.im)
	t.searchText = s.searchText
	if t.searchText != "" {
		t.searcher.Entry.Text = t.searchText
		t.doSearch(t.searchText)
	}
	return t
}

func (t *TracksPage) playRandomSongs() {
	go func() {
		err := t.contr.App.PlaybackManager.PlayRandomSongs("")
		if err != nil {
			log.Printf("error playing random tracks: %v", err)
			fyne.Do(func() {
				t.contr.ToastProvider.ShowErrorToast(lang.L("Unable to play random tracks"))
			})
		}
	}()
}

func (t *TracksPage) obtainTracklist() *widgets.Tracklist {
	if tl := t.widgetPool.Obtain(util.WidgetTypeTracklist); tl != nil {
		tracklist := tl.(*widgets.Tracklist)
		tracklist.Reset()
		return tracklist
	}
	return widgets.NewTracklist(nil, t.im, false)
}
