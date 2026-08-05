package widgets

import (
	"image"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
)

// Shows the current album art, track name, artist name, and album name
// for the currently playing track. Placed into the left side of the BottomPanel.
type LargeNowPlayingCard struct {
	CaptionedImage

	DisableRating bool
	ShowAlbumYear bool

	isRadio                 bool
	trackName               *widget.RichText
	artistName              *MultiHyperlink
	albumName               *MultiHyperlink
	rating                  *StarRating
	favorite                *FavoriteIcon
	ratingFavoriteContainer *fyne.Container
	cover                   *ImagePlaceholder

	albumYear string

	OnArtistNameTapped func(artistID string)
	OnAlbumNameTapped  func()
	OnRadioURLTapped   func(url string)
	OnSetRating        func(rating int)
	OnSetFavorite      func(favorite bool)
}

func NewLargeNowPlayingCard() *LargeNowPlayingCard {
	n := &LargeNowPlayingCard{
		trackName:  widget.NewRichTextWithText(""),
		artistName: NewMultiHyperlink(),
		albumName:  NewMultiHyperlink(),
		rating:     NewStarRating(),
		favorite:   NewFavoriteIcon(),
		cover:      NewImagePlaceholder(myTheme.TracksIcon, 300),
	}
	n.ExtendBaseWidget(n)
	n.rating.StarSize = theme.IconInlineSize() + theme.InnerPadding()/2
	n.rating.OnRatingChanged = n.onSetRating
	n.favorite.OnTapped = n.onToggleFavorite
	n.cover.ScaleMode = canvas.ImageScaleFastest
	// set up the layout
	n.Content = n.cover
	n.ratingFavoriteContainer = container.NewHBox(
		layout.NewSpacer(),
		n.favorite,
		widget.NewLabel("·"),
		n.rating,
		layout.NewSpacer(),
	)
	n.Caption = container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-15),
		n.trackName,
		n.artistName,
		n.albumName,
		util.NewVSpace(20),
		n.ratingFavoriteContainer,
	)

	n.trackName.Segments[0].(*widget.TextSegment).Style.SizeName = theme.SizeNameSubHeadingText
	n.trackName.Truncation = fyne.TextTruncateEllipsis
	n.albumName.SizeName = myTheme.SizeNameSubSubHeadingText
	n.albumName.OnTapped = n.onAlbumNameTapped
	n.albumName.SuffixParenthesized = true
	n.artistName.OnTapped = n.onArtistNameTapped
	n.artistName.SizeName = myTheme.SizeNameSubSubHeadingText

	return n
}

func (n *LargeNowPlayingCard) onAlbumNameTapped(_ string) {
	if n.OnAlbumNameTapped != nil {
		n.OnAlbumNameTapped()
	}
}

func (n *LargeNowPlayingCard) onArtistNameTapped(artistID string) {
	if n.isRadio {
		if n.OnRadioURLTapped != nil {
			n.OnRadioURLTapped(artistID)
		}
		return
	}
	if n.OnArtistNameTapped != nil {
		n.OnArtistNameTapped(artistID)
	}
}

func (n *LargeNowPlayingCard) onToggleFavorite() {
	n.favorite.Favorite = !n.favorite.Favorite
	n.favorite.Refresh()
	if n.OnSetFavorite != nil {
		n.OnSetFavorite(n.favorite.Favorite)
	}
}

func (n *LargeNowPlayingCard) onSetRating(rating int) {
	if n.OnSetRating != nil {
		n.OnSetRating(rating)
	}
}

func (n *LargeNowPlayingCard) SetDisplayedRating(rating int) {
	n.rating.Rating = rating
	n.rating.Refresh()
}

func (n *LargeNowPlayingCard) SetDisplayedFavorite(favorite bool) {
	n.favorite.Favorite = favorite
	n.favorite.Refresh()
}

func (n *LargeNowPlayingCard) Update(item mediaprovider.MediaItem) {
	if item == nil {
		n.trackName.Segments[0].(*widget.TextSegment).Text = ""
		n.artistName.BuildSegments([]string{}, []string{})
		n.albumName.BuildSegments([]string{}, []string{})
		n.albumName.Suffix = ""
		n.rating.Rating = 0
		n.favorite.Favorite = false
		n.ratingFavoriteContainer.Hidden = true
		n.Refresh()
		return
	}

	meta := item.Metadata()
	n.trackName.Segments[0].(*widget.TextSegment).Text = meta.Name
	n.albumName.BuildSegments([]string{meta.Album}, []string{meta.AlbumID})

	if tr, ok := item.(*mediaprovider.Track); ok {
		n.artistName.BuildSegments(meta.Artists, meta.ArtistIDs)
		n.rating.Rating = tr.Rating
		n.favorite.Favorite = tr.Favorite
		n.cover.PlaceholderIcon = myTheme.TracksIcon
		n.ratingFavoriteContainer.Hidden = false
		n.isRadio = false
		n.albumYear = strconv.Itoa(tr.Year)
	} else if rd, ok := item.(*mediaprovider.RadioStation); ok {
		n.artistName.BuildSegments([]string{rd.HomePageURL}, []string{rd.HomePageURL})
		n.ratingFavoriteContainer.Hidden = true
		n.cover.PlaceholderIcon = myTheme.RadioIcon
		n.isRadio = true
		n.albumYear = ""
	}

	n.Refresh()
}

func (n *LargeNowPlayingCard) SetCoverImage(im image.Image) {
	n.cover.SetImage(im, false)
}

func (n *LargeNowPlayingCard) Refresh() {
	if n.DisableRating {
		n.rating.Disable()
	} else {
		n.rating.Enable()
	}
	if n.ShowAlbumYear {
		n.albumName.Suffix = n.albumYear
	} else {
		n.albumName.Suffix = ""
	}
	n.BaseWidget.Refresh()
}
