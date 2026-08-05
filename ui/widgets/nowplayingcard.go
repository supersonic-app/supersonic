package widgets

import (
	"image"
	"strconv"

	"fyne.io/fyne/v2/lang"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Shows the current album art, track name, artist name, and album name
// for the currently playing track. Placed into the left side of the BottomPanel.
type NowPlayingCard struct {
	widget.BaseWidget

	DisableRating bool
	ShowAlbumYear bool

	trackName  *OptionHyperlink
	artistName *MultiHyperlink
	albumName  *MultiHyperlink
	cover      *ImagePlaceholder
	menu       *widget.PopUpMenu
	ratingMenu *fyne.MenuItem

	albumYear string

	OnTrackNameTapped  func()
	OnArtistNameTapped func(artistID string)
	OnAlbumNameTapped  func(albumID string)
	OnCoverTapped      func()
	OnSetRating        func(rating int)
	OnSetFavorite      func(favorite bool)
	OnAddToPlaylist    func()
	OnShowTrackInfo    func()
	OnShare            func()
}

func NewNowPlayingCard() *NowPlayingCard {
	n := &NowPlayingCard{
		trackName:  NewOptionHyperlink(),
		artistName: NewMultiHyperlink(),
		albumName:  NewMultiHyperlink(),
	}
	n.ExtendBaseWidget(n)
	n.cover = NewImagePlaceholder(myTheme.TracksIcon, 85)
	n.cover.OnTapped = n.onShowCoverImage
	n.cover.ScaleMode = canvas.ImageScaleFastest
	n.cover.Hidden = true
	n.trackName.Hidden = true
	n.albumName.Hidden = true
	n.albumName.SuffixParenthesized = true
	n.albumName.SuffixSizeName = myTheme.SizeNameSubText
	n.trackName.SetTextStyle(fyne.TextStyle{Bold: true})
	n.trackName.OnShowMenu = n.showMenu
	n.albumName.OnTapped = n.onAlbumNameTapped
	n.artistName.OnTapped = n.onArtistNameTapped
	n.trackName.SetOnTapped(n.onTrackNameTapped)

	return n
}

func (n *NowPlayingCard) MinSize() fyne.Size {
	// prop up height for when cover image is hidden
	return fyne.NewSize(n.BaseWidget.MinSize().Width, 85)
}

func (n *NowPlayingCard) onAlbumNameTapped(albumID string) {
	if n.OnAlbumNameTapped != nil {
		n.OnAlbumNameTapped(albumID)
	}
}

func (n *NowPlayingCard) onArtistNameTapped(artistID string) {
	if n.OnArtistNameTapped != nil {
		n.OnArtistNameTapped(artistID)
	}
}

func (n *NowPlayingCard) onTrackNameTapped() {
	if n.OnTrackNameTapped != nil {
		n.OnTrackNameTapped()
	}
}

func (n *NowPlayingCard) onShowCoverImage(*fyne.PointEvent) {
	if n.OnCoverTapped != nil {
		n.OnCoverTapped()
	}
}

func (n *NowPlayingCard) onSetFavorite(fav bool) {
	if n.OnSetFavorite != nil {
		n.OnSetFavorite(fav)
	}
}

func (n *NowPlayingCard) onSetRating(rating int) {
	if n.OnSetRating != nil {
		n.OnSetRating(rating)
	}
}

func (n *NowPlayingCard) onAddToPlaylist() {
	if n.OnAddToPlaylist != nil {
		n.OnAddToPlaylist()
	}
}

func (n *NowPlayingCard) onShowTrackInfo() {
	if n.OnShowTrackInfo != nil {
		n.OnShowTrackInfo()
	}
}

func (n *NowPlayingCard) onShare() {
	if n.OnShare != nil {
		n.OnShare()
	}
}

func (n *NowPlayingCard) CreateRenderer() fyne.WidgetRenderer {
	c := container.New(&layout.CustomPaddedLayout{LeftPadding: -4},
		container.NewBorder(nil, nil, n.cover, nil,
			container.New(&layout.CustomPaddedLayout{TopPadding: -2},
				container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-13), n.trackName, n.artistName, n.albumName))),
	)
	return widget.NewSimpleRenderer(c)
}

func (n *NowPlayingCard) Update(track mediaprovider.MediaItem) {
	if track == nil {
		n.trackName.SetTextAndToolTip("")
		n.artistName.BuildSegments([]string{}, []string{})
		n.albumName.BuildSegments([]string{}, []string{})
		n.albumYear = ""
		n.cover.Hidden = true
	} else {
		n.cover.Hidden = false
		n.trackName.SetTextAndToolTip(track.Metadata().Name)
		if tr, ok := track.(*mediaprovider.Track); ok {
			n.artistName.BuildSegments(tr.ArtistNames, tr.ArtistIDs)
			n.albumName.BuildSegments([]string{tr.Album}, []string{tr.AlbumID})
			n.albumYear = strconv.Itoa(tr.Year)
			n.cover.PlaceholderIcon = myTheme.TracksIcon
		} else {
			n.artistName.BuildSegments([]string{}, []string{})
			n.albumName.BuildSegments([]string{}, []string{})
			n.albumName.Suffix = ""
			n.cover.PlaceholderIcon = myTheme.RadioIcon
		}
	}
	n.trackName.Hidden = n.trackName.Text() == ""
	n.trackName.SetMenuBtnEnabled(n.cover.PlaceholderIcon != myTheme.RadioIcon)
	n.artistName.Hidden = len(n.artistName.Segments) == 0
	n.albumName.Hidden = len(n.albumName.Segments) == 0
	n.Refresh()
}

func (n *NowPlayingCard) SetImage(cover image.Image) {
	n.cover.SetImage(cover, true)
}

func (n *NowPlayingCard) Refresh() {
	if n.ShowAlbumYear {
		n.albumName.Suffix = n.albumYear
	} else {
		n.albumName.Suffix = ""
	}
	n.BaseWidget.Refresh()
}

func (n *NowPlayingCard) showMenu(btnPos fyne.Position) {
	if n.menu == nil {
		n.ratingMenu = util.NewRatingSubmenu(n.onSetRating)
		favorite := fyne.NewMenuItem(lang.L("Set favorite"), func() { n.onSetFavorite(true) })
		favorite.Icon = myTheme.FavoriteIcon
		unfavorite := fyne.NewMenuItem(lang.L("Unset favorite"), func() { n.onSetFavorite(false) })
		unfavorite.Icon = myTheme.NotFavoriteIcon
		playlist := fyne.NewMenuItem(lang.L("Add to playlist")+"...", func() { n.onAddToPlaylist() })
		playlist.Icon = myTheme.PlaylistIcon
		info := fyne.NewMenuItem(lang.L("Show info")+"...", func() { n.onShowTrackInfo() })
		info.Icon = theme.InfoIcon()
		share := fyne.NewMenuItem(lang.L("Share")+"...", func() { n.onShare() })
		share.Icon = myTheme.ShareIcon

		m := fyne.NewMenu("", favorite, unfavorite, n.ratingMenu, playlist, info, share)
		n.menu = widget.NewPopUpMenu(m, fyne.CurrentApp().Driver().CanvasForObject(n))
	}
	menuSize := n.menu.MinSize()
	n.ratingMenu.Disabled = n.DisableRating
	btnPos.Y -= (menuSize.Height + theme.Padding()*3)
	btnPos.X -= menuSize.Width / 2
	n.menu.ShowAtPosition(btnPos)
}
