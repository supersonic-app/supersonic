package browsing

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/deluan/sanitize"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
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

type PlaylistPage struct {
	widget.BaseWidget

	playlistPageState

	disposed     bool
	header       *PlaylistPageHeader
	tracklist    *widgets.Tracklist
	tracks       []*mediaprovider.Track
	nowPlayingID string
	container    *fyne.Container
}

type playlistPageState struct {
	playlistID string
	conf       *backend.PlaylistPageConfig
	contr      *controller.Controller
	widgetPool *util.WidgetPool
	sm         *backend.ServerManager
	pm         *backend.PlaybackManager
	im         *backend.ImageManager
	trackSort  widgets.TracklistSort
	scroll     float32
}

func NewPlaylistPage(
	playlistID string,
	conf *backend.PlaylistPageConfig,
	pool *util.WidgetPool,
	contr *controller.Controller,
	sm *backend.ServerManager,
	pm *backend.PlaybackManager,
	im *backend.ImageManager,
) *PlaylistPage {
	return newPlaylistPage(playlistID, conf, contr, pool, sm, pm, im, widgets.TracklistSort{}, 0)
}

func newPlaylistPage(
	playlistID string,
	conf *backend.PlaylistPageConfig,
	contr *controller.Controller,
	pool *util.WidgetPool,
	sm *backend.ServerManager,
	pm *backend.PlaybackManager,
	im *backend.ImageManager,
	trackSort widgets.TracklistSort,
	scroll float32,
) *PlaylistPage {
	a := &PlaylistPage{playlistPageState: playlistPageState{playlistID: playlistID, conf: conf, contr: contr, widgetPool: pool, sm: sm, pm: pm, im: im, scroll: scroll}}
	a.ExtendBaseWidget(a)
	if h := a.widgetPool.Obtain(util.WidgetTypePlaylistPageHeader); h != nil {
		a.header = h.(*PlaylistPageHeader)
		a.header.Clear()
	} else {
		a.header = NewPlaylistPageHeader(a)
	}
	a.header.page = a
	a.header.Compact = a.conf.CompactHeader
	if tl := a.widgetPool.Obtain(util.WidgetTypeTracklist); tl != nil {
		a.tracklist = tl.(*widgets.Tracklist)
		a.tracklist.Reset()
	} else {
		a.tracklist = widgets.NewTracklist(nil, a.im, false)
	}
	a.tracklist.SetVisibleColumns(conf.TracklistColumns)
	a.tracklist.SetSorting(trackSort)
	a.tracklist.OnVisibleColumnsChanged = func(cols []string) {
		conf.TracklistColumns = cols
	}
	a.tracklist.OnReorderTracks = a.doSetNewTrackOrder
	_, canRate := a.sm.Server.(mediaprovider.SupportsRating)
	_, canShare := a.sm.Server.(mediaprovider.SupportsSharing)
	remove := fyne.NewMenuItem(lang.L("Remove from playlist"), a.onRemoveSelectedFromPlaylist)
	remove.Icon = theme.ContentClearIcon()
	a.tracklist.Options = widgets.TracklistOptions{
		Reorderable:        true,
		DisableRating:      !canRate,
		DisableSharing:     !canShare,
		AuxiliaryMenuItems: []*fyne.MenuItem{remove},
	}
	// connect tracklist actions
	a.contr.ConnectTracklistActions(a.tracklist)

	a.container = container.NewBorder(
		container.New(&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, TopPadding: 15, BottomPadding: 10}, a.header),
		nil, nil, nil, container.New(&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, BottomPadding: 15}, a.tracklist))

	a.tracklist.SetLoading(true)
	go a.load()
	return a
}

func (a *PlaylistPage) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.container)
}

func (a *PlaylistPage) Save() SavedPage {
	a.disposed = true
	a.tracklist.SetLoading(false)
	p := a.playlistPageState
	p.trackSort = a.tracklist.Sorting()
	p.scroll = a.tracklist.GetScrollOffset()
	p.widgetPool.Release(util.WidgetTypePlaylistPageHeader, a.header)
	a.tracklist.Clear()
	a.tracklist.OnReorderTracks = nil
	p.widgetPool.Release(util.WidgetTypeTracklist, a.tracklist)
	return &p
}

func (a *PlaylistPage) Route() controller.Route {
	return controller.PlaylistRoute(a.playlistID)
}

var _ CanShowNowPlaying = (*PlaylistPage)(nil)

func (a *PlaylistPage) OnSongChange(item mediaprovider.MediaItem, lastScrobbledIfAny *mediaprovider.Track) {
	a.nowPlayingID = sharedutil.MediaItemIDOrEmptyStr(item)
	a.tracklist.SetNowPlaying(a.nowPlayingID)
	a.tracklist.IncrementPlayCount(sharedutil.MediaItemIDOrEmptyStr(lastScrobbledIfAny))
}

func (a *PlaylistPage) Reload() {
	a.tracklist.SetLoading(true)
	go a.load()
}

var _ CanSelectAll = (*PlaylistPage)(nil)

func (a *PlaylistPage) SelectAll() {
	a.tracklist.SelectAll()
}

func (a *PlaylistPage) UnselectAll() {
	a.tracklist.UnselectAll()
}

var _ Scrollable = (*PlaylistPage)(nil)

func (a *PlaylistPage) Scroll(scrollAmt float32) {
	a.tracklist.ScrollBy(scrollAmt)
}

// should be called asynchronously
func (a *PlaylistPage) load() {
	playlist, err := a.sm.Server.GetPlaylist(a.playlistID)
	if err != nil {
		msg := err.Error()
		log.Printf("Failed to get playlist: %s", msg)
		toastMsg := "An error occurred"
		if strings.Contains(msg, "deadline exceeded") {
			toastMsg = "The request timed out"
		}
		fyne.Do(func() {
			a.tracklist.SetLoading(false)
			a.contr.ToastProvider.ShowErrorToast(lang.L(toastMsg))
		})
		return
	}
	if a.disposed {
		return
	}
	renumberTracks(playlist.Tracks)
	fyne.Do(func() {
		a.tracklist.SetLoading(false)
		a.tracks = playlist.Tracks
		a.tracklist.SetTracks(playlist.Tracks)
		a.tracklist.SetNowPlaying(a.nowPlayingID)
		if a.scroll != 0 {
			a.tracklist.ScrollToOffset(a.scroll)
			a.scroll = 0
		}
		a.header.Update(playlist)
	})
}

func renumberTracks(tracks []*mediaprovider.Track) {
	// Playlists, like albums, have a sequential running order. We want the number column to
	// represent the track's original position in the playlist, if the user applies a sort.
	// Re-purpose the TrackNumber field for the track's position in the playlist, rather than its parent album.
	for i, tr := range tracks {
		tr.TrackNumber = i + 1
	}
}

func (a *PlaylistPage) doSetNewTrackOrder(ids []string, newPos int) {
	// Since the tracklist view may be sorted in a different order than the
	// actual running order, we need to get the IDs of the selected tracks
	// from the tracklist and convert them to indices in the *original* run order
	idSet := sharedutil.ToSet(ids)
	idxs := make([]int, 0, len(idSet))
	for i, tr := range a.tracks {
		if _, ok := idSet[tr.ID]; ok {
			idxs = append(idxs, i)
		}
	}
	newTracks := sharedutil.ReorderItems(a.tracks, idxs, newPos)
	ids = sharedutil.TracksToIDs(newTracks)

	go func() {
		if err := a.sm.Server.ReplacePlaylistTracks(a.playlistID, ids); err != nil {
			log.Printf("error updating playlist: %s", err.Error())
			fyne.Do(func() {
				a.contr.ToastProvider.ShowErrorToast(
					lang.L("An error occurred updating the playlist"),
				)
			})
		} else {
			renumberTracks(newTracks)
			fyne.Do(func() {
				// force-switch back to unsorted view to show new track order
				a.tracklist.SetSorting(widgets.TracklistSort{})
				a.tracklist.SetTracks(newTracks)
				a.tracklist.UnselectAll()
				a.tracks = newTracks
			})
		}
	}()
}

func (a *PlaylistPage) onRemoveSelectedFromPlaylist() {
	idxToRemove := sharedutil.MapSlice(a.tracklist.SelectedTracks(), func(t *mediaprovider.Track) int {
		return t.TrackNumber - 1
	})
	go func() {
		if err := a.sm.Server.RemovePlaylistTracks(a.playlistID, idxToRemove); err != nil {
			log.Printf("error removing playlist tracks: %s", err.Error())
			fyne.Do(func() {
				a.contr.ToastProvider.ShowErrorToast(
					lang.L("An error occurred updating the playlist"),
				)
			})
		} else {
			fyne.Do(func() {
				a.tracklist.UnselectAll()
				a.Reload()
			})
		}
	}()
}

func (a *PlaylistPage) onSearched(query string) {
	if query == "" {
		// switch back to full playlist view

		ids := a.tracklist.SelectedTrackIDs()
		a.tracklist.Options.Reorderable = true
		a.tracklist.SetSorting(a.trackSort) // restore old sort order
		a.tracklist.SetTracks(a.tracks)
		// if a track was selected in the searched view,
		// scroll to it when switching back to full playlist
		if len(ids) > 0 {
			a.tracklist.SelectAndScrollToTrack(ids[0])
		}
		return
	}

	// search tracks within the playlist
	searched := sharedutil.FilterSlice(a.tracks, func(t *mediaprovider.Track) bool {
		sani := func(s string) string {
			return strings.ToLower(sanitize.Accents(s))
		}
		qLower := strings.ToLower(query)
		return strings.Contains(sani(t.Title), qLower) ||
			strings.Contains(sani(strings.Join(t.ArtistNames, "")), qLower) ||
			strings.Contains(sani(t.Album), qLower) ||
			strings.Contains(sani(strings.Join(t.Genres, "")), qLower) ||
			strings.Contains(sani(strings.Join(t.ComposerNames, "")), qLower) ||
			strings.Contains(fmt.Sprintf("%d", t.Year), query) ||
			strings.Contains(sani(t.Comment), qLower)
	})

	a.trackSort = a.tracklist.Sorting() // save old sort order
	a.tracklist.Options.Reorderable = false
	a.tracklist.SetSorting(widgets.TracklistSort{})
	a.tracklist.SetTracks(searched)
}

type PlaylistPageHeader struct {
	widget.BaseWidget

	Compact bool

	page         *PlaylistPage
	playlistInfo *mediaprovider.PlaylistWithTracks
	image        *widgets.ImagePlaceholder

	editButton       *widget.Button
	titleLabel       *widget.RichText
	descriptionLabel *widget.Label
	createdAtLabel   *widget.Label
	ownerLabel       *widget.Label
	trackTimeLabel   *widget.Label
	collapseBtn      *widgets.HeaderCollapseButton

	fullSizeCoverFetching bool

	container *fyne.Container
}

func NewPlaylistPageHeader(page *PlaylistPage) *PlaylistPageHeader {
	// due to widget reuse a.page can change so page MUST NOT
	// be directly captured in a closure throughout this function!
	a := &PlaylistPageHeader{page: page}
	a.ExtendBaseWidget(a)

	a.image = widgets.NewImagePlaceholder(myTheme.PlaylistIcon, myTheme.HeaderImageSize)
	a.image.OnTapped = func(*fyne.PointEvent) { go a.showPopUpCover() }
	a.titleLabel = util.NewTruncatingRichText()
	a.titleLabel.Segments[0].(*widget.TextSegment).Style = widget.RichTextStyle{
		SizeName: theme.SizeNameHeadingText,
	}
	a.descriptionLabel = util.NewTruncatingLabel()
	a.ownerLabel = util.NewTruncatingLabel()
	a.createdAtLabel = widget.NewLabel("")
	a.trackTimeLabel = widget.NewLabel("")

	a.editButton = widget.NewButtonWithIcon(lang.L("Edit"), theme.DocumentCreateIcon(), func() {
		if a.playlistInfo != nil {
			a.page.contr.DoEditPlaylistWorkflow(&a.playlistInfo.Playlist)
		}
	})
	a.editButton.Hidden = true
	playButton := widget.NewButtonWithIcon(lang.L("Play"), theme.MediaPlayIcon(), func() {
		a.page.pm.LoadTracks(a.page.tracks, backend.Replace, false)
		a.page.pm.PlayFromBeginning()
	})
	shuffleBtn := widget.NewButtonWithIcon(lang.L("Shuffle"), myTheme.ShuffleIcon, func() {
		a.page.pm.LoadTracks(a.page.tracks, backend.Replace, true)
		a.page.pm.PlayFromBeginning()
	})

	// set up search button / entry animation pair
	// needs reference to containing HBox container (initialized later)
	var buttonRow *fyne.Container
	var searchBtn *ttwidget.Button
	var searchEntry *widgets.SearchEntry
	searchBtn = ttwidget.NewButtonWithIcon("", theme.SearchIcon(), func() {
		minW := searchBtn.MinSize().Width
		if searchEntry == nil {
			searchEntry = widgets.NewSearchEntry()
			searchEntry.PlaceHolder = ""
			searchEntry.Scroll = container.ScrollNone
			searchEntry.OnSearched = func(s string) {
				// we can't assign a.page.onSearched directly,
				// since widget reuse means a.page could change
				a.page.onSearched(s)
			}
			searchEntry.OnFocusLost = func() {
				if searchEntry.Text == "" {
					// dismissal animation
					searchEntry.Scroll = container.ScrollNone
					searchEntry.SetPlaceHolder("")
					fyne.NewAnimation(myTheme.AnimationDurationShort, func(f float32) {
						f = 1 - f
						w := (200-minW)*f + minW
						searchEntry.SetMinWidth(w)
						if f == 0 {
							buttonRow.Objects[3] = searchBtn
						}
						// re-layout container (without unneeded full refresh)
						buttonRow.Layout.Layout(buttonRow.Objects, buttonRow.Layout.MinSize(buttonRow.Objects))
					}).Start()
				}
			}
		}
		buttonRow.Objects[3] = searchEntry
		searchEntry.SetMinWidth(minW)
		if c := fyne.CurrentApp().Driver().CanvasForObject(a); c != nil {
			c.Focus(searchEntry)
		}
		fyne.NewAnimation(myTheme.AnimationDurationShort, func(f float32) {
			w := (200-minW)*f + minW
			searchEntry.SetMinWidth(w)
			// re-layout container (without unneeded full refresh)
			buttonRow.Layout.Layout(buttonRow.Objects, buttonRow.Layout.MinSize(buttonRow.Objects))
			if f == 1 {
				searchEntry.Scroll = container.ScrollHorizontalOnly
				searchEntry.Refresh() // needed to initialize widget's scroller
				searchEntry.SetPlaceHolder(lang.L("Search"))
			}
		}).Start()
	})
	searchBtn.SetToolTip(lang.L("Search"))

	var pop *widget.PopUpMenu
	menuBtn := widget.NewButtonWithIcon("", theme.MoreHorizontalIcon(), nil)
	menuBtn.OnTapped = func() {
		if pop == nil {
			playNext := fyne.NewMenuItem(lang.L("Play next"), func() {
				go a.page.pm.LoadPlaylist(a.page.playlistID, backend.InsertNext, false)
			})
			playNext.Icon = myTheme.PlayNextIcon
			queue := fyne.NewMenuItem(lang.L("Add to queue"), func() {
				go a.page.pm.LoadPlaylist(a.page.playlistID, backend.Append, false)
			})
			queue.Icon = theme.ContentAddIcon()
			playlist := fyne.NewMenuItem(lang.L("Add to playlist")+"...", func() {
				a.page.contr.DoAddTracksToPlaylistWorkflow(
					sharedutil.TracksToIDs(a.page.tracks))
			})
			playlist.Icon = myTheme.PlaylistIcon
			download := fyne.NewMenuItem(lang.L("Download")+"...", func() {
				a.page.contr.ShowDownloadDialog(a.page.tracks, a.titleLabel.String())
			})
			download.Icon = theme.DownloadIcon()
			menu := fyne.NewMenu("", playNext, queue, playlist, download)
			pop = widget.NewPopUpMenu(menu, fyne.CurrentApp().Driver().CanvasForObject(a))
		}
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(menuBtn)
		pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+menuBtn.Size().Height))
	}

	buttonRow = container.NewHBox(a.editButton, playButton, shuffleBtn, searchBtn, menuBtn)

	a.collapseBtn = widgets.NewHeaderCollapseButton(func() {
		a.Compact = !a.Compact
		a.page.conf.CompactHeader = a.Compact
		a.page.Refresh()
	})
	a.collapseBtn.Hidden = true

	a.container = util.AddHeaderBackground(
		container.NewStack(
			container.NewBorder(nil, nil, a.image, nil,
				container.NewVBox(a.titleLabel, container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-10),
					a.descriptionLabel,
					a.ownerLabel,
					a.trackTimeLabel),
					buttonRow,
				)),
			container.NewVBox(container.NewHBox(layout.NewSpacer(), a.collapseBtn)),
		),
	)
	return a
}

func (a *PlaylistPageHeader) Clear() {
	a.titleLabel.Segments[0].(*widget.TextSegment).Text = ""
	a.createdAtLabel.Text = ""
	a.descriptionLabel.Text = ""
	a.ownerLabel.Text = ""
	a.image.SetImage(nil, false)
}

func (a *PlaylistPageHeader) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.container)
}

func (a *PlaylistPageHeader) Update(playlist *mediaprovider.PlaylistWithTracks) {
	a.playlistInfo = playlist
	a.editButton.Hidden = playlist.Owner != a.page.sm.LoggedInUser
	a.titleLabel.Segments[0].(*widget.TextSegment).Text = playlist.Name
	a.descriptionLabel.SetText(strings.ReplaceAll(playlist.Description, "\n", " "))
	a.ownerLabel.SetText(a.formatPlaylistOwnerStr(playlist))
	a.trackTimeLabel.SetText(a.formatPlaylistTrackTimeStr(playlist))

	var haveCover bool
	if playlist.CoverArtID != "" {
		if im, err := a.page.im.GetCoverThumbnail(playlist.CoverArtID); err == nil && im != nil {
			a.image.SetImage(im, true /*tappable*/)
			haveCover = true
		}
	}
	if !haveCover {
		if im, err := a.page.im.GetCoverThumbnail(playlist.ID); err == nil && im != nil {
			a.image.SetImage(im, true)
		}
	}
	a.Refresh()
}

var _ desktop.Hoverable = (*PlaylistPageHeader)(nil)

func (a *PlaylistPageHeader) MouseIn(*desktop.MouseEvent) {
	a.collapseBtn.Show()
	a.Refresh()
}

func (a *PlaylistPageHeader) MouseOut() {
	a.collapseBtn.HideIfNotMousedIn()
}

func (a *PlaylistPageHeader) MouseMoved(*desktop.MouseEvent) {
}

func (a *PlaylistPageHeader) Refresh() {
	a.descriptionLabel.Hidden = a.Compact
	a.ownerLabel.Hidden = a.Compact
	a.trackTimeLabel.Hidden = a.Compact
	a.collapseBtn.Collapsed = a.Compact
	if a.Compact {
		a.image.SetMinSize(fyne.NewSquareSize(myTheme.CompactHeaderImageSize))
	} else {
		a.image.SetMinSize(fyne.NewSquareSize(myTheme.HeaderImageSize))
	}
	a.BaseWidget.Refresh()
}

// should be called asynchronously
func (a *PlaylistPageHeader) showPopUpCover() {
	if a.fullSizeCoverFetching || a.playlistInfo == nil {
		return
	}
	a.fullSizeCoverFetching = true
	defer func() { a.fullSizeCoverFetching = false }()
	cover, err := a.page.im.GetFullSizeCoverArt(a.playlistInfo.CoverArtID)
	if err != nil {
		log.Printf("error getting full size playlist cover: %s", err.Error())
		return
	}
	if a.page != nil {
		fyne.Do(func() { a.page.contr.ShowPopUpImage(cover) })
	}
}

func (a *PlaylistPageHeader) formatPlaylistOwnerStr(p *mediaprovider.PlaylistWithTracks) string {
	pubPriv := lang.L("Public playlist by")
	if !p.Public {
		pubPriv = lang.L("Private playlist by")
	}
	return fmt.Sprintf("%s %s", pubPriv, p.Owner)
}

func (a *PlaylistPageHeader) formatPlaylistTrackTimeStr(p *mediaprovider.PlaylistWithTracks) string {
	var tracks string
	if p.TrackCount == 1 {
		tracks = lang.L("track")
	} else {
		tracks = lang.L("tracks")
	}
	fallbackTracksMsg := fmt.Sprintf("%d %s", p.TrackCount, tracks)
	tracksMsg := lang.LocalizePluralKey("{{.trackCount}} tracks",
		fallbackTracksMsg, p.TrackCount, map[string]string{"trackCount": strconv.Itoa(p.TrackCount)})
	return fmt.Sprintf("%s, %s", tracksMsg, util.SecondsToTimeString(p.Duration.Seconds()))
}

func (s *playlistPageState) Restore() Page {
	return newPlaylistPage(s.playlistID, s.conf, s.contr, s.widgetPool, s.sm, s.pm, s.im, s.trackSort, s.scroll)
}
