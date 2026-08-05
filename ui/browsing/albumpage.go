package browsing

import (
	"fmt"
	"log"
	"strconv"
	"strings"

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

type AlbumPage struct {
	widget.BaseWidget

	albumPageState

	disposed     bool
	header       *AlbumPageHeader
	tracks       []*mediaprovider.Track
	tracklist    *widgets.Tracklist
	nowPlayingID string
	container    *fyne.Container
}

type albumPageState struct {
	albumID string
	sort    widgets.TracklistSort
	scroll  float32
	cfg     *backend.AlbumPageConfig
	pool    *util.WidgetPool
	mp      mediaprovider.MediaProvider
	pm      *backend.PlaybackManager
	im      *backend.ImageManager
	contr   *controller.Controller
}

func NewAlbumPage(
	albumID string,
	cfg *backend.AlbumPageConfig,
	pool *util.WidgetPool,
	pm *backend.PlaybackManager,
	mp mediaprovider.MediaProvider,
	im *backend.ImageManager,
	contr *controller.Controller,
) *AlbumPage {
	return newAlbumPage(albumID, cfg, pool, pm, mp, im, contr, widgets.TracklistSort{}, 0)
}

func newAlbumPage(
	albumID string,
	cfg *backend.AlbumPageConfig,
	pool *util.WidgetPool,
	pm *backend.PlaybackManager,
	mp mediaprovider.MediaProvider,
	im *backend.ImageManager,
	contr *controller.Controller,
	sort widgets.TracklistSort,
	scroll float32,
) *AlbumPage {
	a := &AlbumPage{
		albumPageState: albumPageState{
			albumID: albumID,
			cfg:     cfg,
			pool:    pool,
			pm:      pm,
			mp:      mp,
			im:      im,
			contr:   contr,
			scroll:  scroll,
		},
	}
	a.ExtendBaseWidget(a)
	if h := pool.Obtain(util.WidgetTypeAlbumPageHeader); h != nil {
		a.header = h.(*AlbumPageHeader)
		a.header.Clear()
	} else {
		a.header = NewAlbumPageHeader(a)
	}
	a.header.page = a
	a.header.Compact = a.cfg.CompactHeader
	if t := a.pool.Obtain(util.WidgetTypeCompactTracklist); t != nil {
		a.tracklist = t.(*widgets.Tracklist)
		a.tracklist.Reset()
	} else {
		a.tracklist = widgets.NewTracklist(nil, a.im, true)
	}
	a.tracklist.SetVisibleColumns(a.cfg.TracklistColumns)
	a.tracklist.SetSorting(sort)
	_, canRate := a.mp.(mediaprovider.SupportsRating)
	_, canShare := a.mp.(mediaprovider.SupportsSharing)
	a.tracklist.Options.DisableRating = !canRate
	a.tracklist.Options.DisableSharing = !canShare
	a.tracklist.OnVisibleColumnsChanged = func(cols []string) {
		a.cfg.TracklistColumns = cols
	}
	a.contr.ConnectTracklistActionsWithReplayGainAlbum(a.tracklist)

	a.container = container.NewBorder(
		container.New(&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, TopPadding: 15, BottomPadding: 10}, a.header),
		nil, nil, nil, container.New(&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 15, BottomPadding: 15}, a.tracklist))

	a.tracklist.SetLoading(true)
	go a.load()
	return a
}

func (a *AlbumPage) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.container)
}

func (a *AlbumPage) Save() SavedPage {
	a.disposed = true
	a.tracklist.SetLoading(false)
	s := a.albumPageState
	s.sort = a.tracklist.Sorting()
	s.scroll = a.tracklist.GetScrollOffset()
	a.header.page = nil
	a.pool.Release(util.WidgetTypeAlbumPageHeader, a.header)
	a.tracklist.Clear()
	a.pool.Release(util.WidgetTypeCompactTracklist, a.tracklist)
	return &s
}

func (a *AlbumPage) Route() controller.Route {
	return controller.AlbumRoute(a.albumID)
}

var _ CanShowNowPlaying = (*AlbumPage)(nil)

func (a *AlbumPage) OnSongChange(track mediaprovider.MediaItem, lastScrobbledIfAny *mediaprovider.Track) {
	a.nowPlayingID = sharedutil.MediaItemIDOrEmptyStr(track)
	a.tracklist.SetNowPlaying(a.nowPlayingID)
	a.tracklist.IncrementPlayCount(sharedutil.MediaItemIDOrEmptyStr(lastScrobbledIfAny))
}

func (a *AlbumPage) Reload() {
	a.tracklist.SetLoading(true)
	go a.load()
}

var _ CanSelectAll = (*AlbumPage)(nil)

func (a *AlbumPage) SelectAll() {
	a.tracklist.SelectAll()
}

func (a *AlbumPage) UnselectAll() {
	a.tracklist.UnselectAll()
}

var _ Scrollable = (*AlbumPage)(nil)

func (a *AlbumPage) Scroll(scrollAmt float32) {
	a.tracklist.ScrollBy(scrollAmt)
}

// should be called asynchronously
func (a *AlbumPage) load() {
	album, err := a.mp.GetAlbum(a.albumID)
	if err != nil {
		msg := err.Error()
		log.Printf("Failed to get album: %s", msg)
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
	fyne.Do(func() {
		a.tracklist.SetLoading(false)
		a.header.Update(album, a.im)
		a.tracklist.Options.ShowDiscNumber = len(album.Tracks) > 0 && album.Tracks[0].DiscNumber != album.Tracks[len(album.Tracks)-1].DiscNumber
		a.tracks = album.Tracks
		a.tracklist.SetTracks(album.Tracks)
		a.tracklist.SetNowPlaying(a.nowPlayingID)
		if a.scroll != 0 {
			a.tracklist.ScrollToOffset(a.scroll)
			a.scroll = 0
		}
	})
}

type AlbumPageHeader struct {
	widget.BaseWidget

	Compact bool

	albumID string
	coverID string

	page *AlbumPage

	cover                 *widgets.ImagePlaceholder
	titleLabel            *widget.RichText
	releaseTypeLabel      *widget.RichText
	artistLabel           *widgets.MultiHyperlink
	artistLabelSpace      *util.Space // TODO: remove when no longer needed
	genreLabel            *widgets.MultiHyperlink
	miscLabel             *widget.Label
	shareMenuItem         *fyne.MenuItem
	collapseBtn           *widgets.HeaderCollapseButton
	artistReleaseTypeLine *fyne.Container

	toggleFavButton *widgets.FavoriteButton

	fullSizeCoverFetching bool

	container *fyne.Container
}

func NewAlbumPageHeader(page *AlbumPage) *AlbumPageHeader {
	// due to widget reuse a.page can change so page MUST NOT
	// be directly captured in a closure throughout this function!
	a := &AlbumPageHeader{page: page}
	a.ExtendBaseWidget(a)
	a.cover = widgets.NewImagePlaceholder(myTheme.AlbumIcon, myTheme.HeaderImageSize)
	a.cover.OnTapped = func(*fyne.PointEvent) { go a.showPopUpCover() }

	a.titleLabel = widget.NewRichTextWithText("")
	a.titleLabel.Truncation = fyne.TextTruncateEllipsis
	a.titleLabel.Segments[0].(*widget.TextSegment).Style = widget.RichTextStyle{
		SizeName: theme.SizeNameHeadingText,
	}
	a.releaseTypeLabel = widget.NewRichText(
		&widget.TextSegment{Text: lang.L("Album"), Style: util.BoldRichTextStyle},
		&widget.TextSegment{Text: " " + lang.L("by"), Style: widget.RichTextStyle{Inline: true}},
	)
	a.artistLabel = widgets.NewMultiHyperlink()
	a.artistLabel.OnTapped = func(id string) {
		a.page.contr.NavigateTo(controller.ArtistRoute(id))
	}
	a.artistLabelSpace = util.NewHSpace(0) // updated in Update
	a.genreLabel = widgets.NewMultiHyperlink()
	a.genreLabel.OnTapped = func(genre string) {
		a.page.contr.NavigateTo(controller.GenreRoute(genre))
	}
	a.miscLabel = widget.NewLabel("")
	playButton := widget.NewButtonWithIcon(lang.L("Play"), theme.MediaPlayIcon(), func() {
		go a.page.pm.PlayAlbum(a.page.albumID, 0, false)
	})
	shuffleBtn := widget.NewButtonWithIcon(lang.L("Shuffle"), myTheme.ShuffleIcon, func() {
		a.page.pm.LoadTracks(a.page.tracklist.GetTracks(), backend.Replace, true)
		a.page.pm.PlayFromBeginning()
	})
	var pop *widget.PopUpMenu
	menuBtn := widget.NewButtonWithIcon("", theme.MoreHorizontalIcon(), nil)
	menuBtn.OnTapped = func() {
		if pop == nil {
			playNext := fyne.NewMenuItem(lang.L("Play next"), func() {
				go a.page.pm.LoadAlbum(a.albumID, backend.InsertNext, false /*shuffle*/)
			})
			playNext.Icon = myTheme.PlayNextIcon
			queue := fyne.NewMenuItem(lang.L("Add to queue"), func() {
				go a.page.pm.LoadAlbum(a.albumID, backend.Append, false /*shuffle*/)
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
			info := fyne.NewMenuItem(lang.L("Show info")+"...", func() {
				a.page.contr.ShowAlbumInfoDialog(a.albumID, a.titleLabel.String(), a.cover.Image())
			})
			info.Icon = theme.InfoIcon()
			a.shareMenuItem = fyne.NewMenuItem(lang.L("Share")+"...", func() {
				a.page.contr.ShowShareDialog(a.albumID)
			})
			a.shareMenuItem.Icon = myTheme.ShareIcon
			menu := fyne.NewMenu("", playNext, queue, playlist, download, info, a.shareMenuItem)
			pop = widget.NewPopUpMenu(menu, fyne.CurrentApp().Driver().CanvasForObject(a))
		}
		_, canShare := page.mp.(mediaprovider.SupportsSharing)
		a.shareMenuItem.Disabled = !canShare
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(menuBtn)
		pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+menuBtn.Size().Height))
	}
	a.toggleFavButton = widgets.NewFavoriteButton(func() { go a.toggleFavorited() })

	a.collapseBtn = widgets.NewHeaderCollapseButton(func() {
		a.Compact = !a.Compact
		a.page.cfg.CompactHeader = a.Compact
		a.page.Refresh()
	})
	a.collapseBtn.Hidden = true

	// TODO: Create a nicer custom layout to set this up properly
	//   OR once TODO in MultiHyperlink to use RichText as a provider is solved,
	//   extend MultiHyperlink to support prepending rich text segments and
	//   don't use two separate widgets here at all.
	// n.b. cannot place MultiHyperlink in a HBox or it collapses in width
	a.artistReleaseTypeLine = container.NewStack(
		a.releaseTypeLabel,
		container.NewBorder(nil, nil, a.artistLabelSpace, nil, a.artistLabel))
	// TODO: there's got to be a way to make this less convoluted. Custom layout?
	a.container = util.AddHeaderBackground(
		container.NewStack(
			container.NewBorder(nil, nil, a.cover, nil,
				container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-10),
					a.titleLabel,
					container.NewVBox(
						container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-12), a.artistReleaseTypeLine, a.genreLabel, a.miscLabel),
						container.NewVBox(
							container.NewHBox(util.NewHSpace(2), playButton, shuffleBtn, menuBtn),
							container.NewHBox(util.NewHSpace(2), a.toggleFavButton),
						),
					),
				),
			),
			container.NewVBox(container.NewHBox(layout.NewSpacer(), a.collapseBtn)),
		),
	)
	return a
}

func (a *AlbumPageHeader) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.container)
}

func (a *AlbumPageHeader) Update(album *mediaprovider.AlbumWithTracks, im *backend.ImageManager) {
	a.albumID = album.ID
	a.coverID = album.CoverArtID
	a.titleLabel.Segments[0].(*widget.TextSegment).Text = album.Name
	a.releaseTypeLabel.Segments[0].(*widget.TextSegment).Text = util.DisplayReleaseType(album.ReleaseTypes)
	a.releaseTypeLabel.Refresh() // needed so MinSize returns correct width below
	a.artistLabelSpace.Width = a.releaseTypeLabel.MinSize().Width - 16
	a.artistLabel.BuildSegments(album.ArtistNames, album.ArtistIDs)
	a.genreLabel.BuildSegments(album.Genres, album.Genres)
	a.miscLabel.SetText(formatMiscLabelStr(album))
	a.toggleFavButton.IsFavorited = album.Favorite
	a.Refresh()

	go func() {
		if cover, err := im.GetCoverThumbnail(album.CoverArtID); err == nil {
			fyne.Do(func() {
				a.cover.SetImage(cover, true)
				a.cover.Refresh()
			})
		} else {
			log.Printf("error fetching cover: %v", err)
		}
	}()
}

func (a *AlbumPageHeader) Clear() {
	a.albumID = ""
	a.coverID = ""
	a.titleLabel.Segments[0].(*widget.TextSegment).Text = ""
	a.artistLabel.Segments = nil
	a.genreLabel.Segments = nil
	a.miscLabel.SetText("")
	a.toggleFavButton.IsFavorited = false
	a.fullSizeCoverFetching = false
	a.cover.SetImage(nil, false)
	a.cover.Refresh()
}

func (a *AlbumPageHeader) toggleFavorited() {
	params := mediaprovider.RatingFavoriteParameters{AlbumIDs: []string{a.albumID}}
	a.page.mp.SetFavorite(params, a.toggleFavButton.IsFavorited)
}

var _ desktop.Hoverable = (*AlbumPageHeader)(nil)

func (a *AlbumPageHeader) MouseIn(e *desktop.MouseEvent) {
	a.collapseBtn.Show()
	a.Refresh()
}

func (a *AlbumPageHeader) MouseOut() {
	a.collapseBtn.HideIfNotMousedIn()
}

func (a *AlbumPageHeader) MouseMoved(*desktop.MouseEvent) {
}

func (a *AlbumPageHeader) Refresh() {
	a.artistReleaseTypeLine.Hidden = a.Compact
	a.genreLabel.Hidden = a.Compact
	a.miscLabel.Hidden = a.Compact
	a.toggleFavButton.Hidden = a.Compact
	a.collapseBtn.Collapsed = a.Compact
	if a.Compact {
		a.cover.SetMinSize(fyne.NewSquareSize(myTheme.CompactHeaderImageSize))
	} else {
		a.cover.SetMinSize(fyne.NewSquareSize(myTheme.HeaderImageSize))
	}
	a.BaseWidget.Refresh()
}

// should be called asynchronously
func (a *AlbumPageHeader) showPopUpCover() {
	if a.fullSizeCoverFetching {
		return
	}
	a.fullSizeCoverFetching = true
	defer func() { a.fullSizeCoverFetching = false }()
	cover, err := a.page.im.GetFullSizeCoverArt(a.coverID)
	if err != nil {
		log.Printf("error getting full size album cover: %s", err.Error())
		return
	}
	if a.page != nil {
		fyne.Do(func() { a.page.contr.ShowPopUpImage(cover) })
	}
}

func formatMiscLabelStr(a *mediaprovider.AlbumWithTracks) string {
	var discs string
	if len(a.Tracks) > 0 {
		if discCount := a.Tracks[len(a.Tracks)-1].DiscNumber; discCount > 1 {
			discs = fmt.Sprintf("%d %s · ", discCount, lang.L("discs"))
		}
	}
	tracks := lang.L("tracks")
	if a.TrackCount == 1 {
		tracks = lang.L("track")
	}
	fallbackTracksMsg := fmt.Sprintf("%d %s", a.TrackCount, tracks)
	tracksMsg := lang.LocalizePluralKey("{{.trackCount}} tracks",
		fallbackTracksMsg, a.TrackCount, map[string]string{"trackCount": strconv.Itoa(a.TrackCount)})
	yearStr := util.FormatItemDate(a.Date)
	if y := a.ReissueDate.Year; y != nil && *y > a.YearOrZero() {
		yearStr += fmt.Sprintf(" (%s %s)", lang.L("reissued"), util.FormatItemDate(a.ReissueDate))
	}
	return fmt.Sprintf("%s · %s · %s%s", yearStr, tracksMsg, discs, util.SecondsToTimeString(a.Duration.Seconds()))
}

func (s *albumPageState) Restore() Page {
	return newAlbumPage(s.albumID, s.cfg, s.pool, s.pm, s.mp, s.im, s.contr, s.sort, s.scroll)
}
