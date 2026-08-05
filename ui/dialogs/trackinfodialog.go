package dialogs

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/ui/util"
	"github.com/supersonic-app/supersonic/ui/widgets"
)

type TrackInfoDialog struct {
	widget.BaseWidget

	OnDismiss          func()
	OnNavigateToArtist func(artistID string)
	OnNavigateToAlbum  func(albumID string)
	OnNavigateToGenre  func(genre string)
	OnCopyFilePath     func()

	track *mediaprovider.Track
}

func NewTrackInfoDialog(track *mediaprovider.Track) *TrackInfoDialog {
	t := &TrackInfoDialog{track: track}
	t.ExtendBaseWidget(t)
	return t
}

func (t *TrackInfoDialog) CreateRenderer() fyne.WidgetRenderer {
	c := container.New(layout.NewFormLayout())

	addFormRow(c, lang.L("Title"), t.track.Title)

	c.Add(newFormText(lang.L("Album"), true))
	album := widget.NewHyperlink(t.track.Album, nil)
	album.OnTapped = func() {
		if t.OnNavigateToAlbum != nil {
			t.OnNavigateToAlbum(t.track.AlbumID)
		}
	}
	c.Add(album)

	c.Add(newFormText(lang.L("Artists"), true))
	artists := widgets.NewMultiHyperlink()
	artists.BuildSegments(t.track.ArtistNames, t.track.ArtistIDs)
	artists.OnTapped = func(id string) {
		if t.OnNavigateToArtist != nil {
			t.OnNavigateToArtist(id)
		}
	}
	c.Add(artists)

	if len(t.track.AlbumArtistNames) > 0 {
		c.Add(newFormText(lang.L("Album artists"), true))
		albumArtists := widgets.NewMultiHyperlink()
		albumArtists.BuildSegments(t.track.AlbumArtistNames, t.track.AlbumArtistIDs)
		albumArtists.OnTapped = func(id string) {
			if t.OnNavigateToArtist != nil {
				t.OnNavigateToArtist(id)
			}
		}
		c.Add(albumArtists)
	}

	if len(t.track.ComposerNames) > 0 {
		c.Add(newFormText(lang.L("Composers"), true))
		composers := widgets.NewMultiHyperlink()
		composers.BuildSegments(t.track.ComposerNames, t.track.ComposerIDs)
		artists.OnTapped = func(id string) {
			if t.OnNavigateToArtist != nil {
				t.OnNavigateToArtist(id)
			}
		}
		c.Add(composers)
	}

	if len(t.track.Genres) > 0 {
		c.Add(newFormText(lang.L("Genres"), true))
		genres := widgets.NewMultiHyperlink()
		genres.BuildSegments(t.track.Genres, t.track.Genres)
		genres.OnTapped = func(g string) {
			if t.OnNavigateToGenre != nil {
				t.OnNavigateToGenre(g)
			}
		}
		c.Add(genres)
	}

	addFormRow(c, lang.L("Duration"), util.SecondsToTimeString(t.track.Duration.Seconds()))
	addFormRow(c, lang.L("Comment"), t.track.Comment)
	addFormRow(c, lang.L("Year"), strconv.Itoa(t.track.Year))
	addFormRow(c, lang.L("Track number"), strconv.Itoa(t.track.TrackNumber))
	addFormRow(c, lang.L("Disc number"), strconv.Itoa(t.track.DiscNumber))

	if t.track.BPM > 0 {
		addFormRow(c, lang.L("BPM"), strconv.Itoa(t.track.BPM))
	}

	copyBtn := widgets.NewIconButton(theme.ContentCopyIcon(), func() {
		if t.OnCopyFilePath != nil {
			t.OnCopyFilePath()
		}
	})
	copyBtn.IconSize = widgets.IconButtonSizeSmaller
	btnCtr := container.New(layout.NewCustomPaddedLayout(8, 0, 10, 0),
		container.NewVBox(copyBtn, layout.NewSpacer()))
	c.Add(container.NewHBox(layout.NewSpacer(), btnCtr, newFormText(lang.L("File path"), true)))
	c.Add(newFormText(t.track.FilePath, false))

	addFormRow(c, lang.L("Content type"), t.track.ContentType)
	addFormRow(c, lang.L("File type"), t.track.Extension)
	addFormRow(c, lang.L("Bit rate"), fmt.Sprintf("%d kbps", t.track.BitRate))
	if t.track.SampleRate > 0 {
		addFormRow(c, lang.L("Sample rate"), fmt.Sprintf("%d Hz", t.track.SampleRate))
	}
	if t.track.BitDepth > 0 {
		addFormRow(c, lang.L("Bit depth"), strconv.Itoa(t.track.BitDepth))
	}
	if t.track.Channels > 0 {
		addFormRow(c, lang.L("Channels"), strconv.Itoa(t.track.Channels))
	}
	addFormRow(c, lang.L("File size"), util.BytesToSizeString(t.track.Size))

	if !t.track.DateAdded.IsZero() {
		addFormRow(c, lang.L("Date added"), t.track.DateAdded.Format(time.RFC1123))
	}

	addFormRow(c, lang.L("Play count"), strconv.Itoa(t.track.PlayCount))

	if !t.track.LastPlayed.IsZero() {
		addFormRow(c, lang.L("Last played"), t.track.LastPlayed.Format(time.RFC1123))
	}

	if t.track.ReplayGain.TrackPeak > 0 {
		addFormRow(c, lang.L("Track gain"), fmt.Sprintf("%0.2f dB", t.track.ReplayGain.TrackGain))
		addFormRow(c, lang.L("Track peak"), fmt.Sprintf("%0.6f", t.track.ReplayGain.TrackPeak))
	}
	if t.track.ReplayGain.AlbumPeak > 0 {
		addFormRow(c, lang.L("Album gain"), fmt.Sprintf("%0.2f dB", t.track.ReplayGain.AlbumGain))
		addFormRow(c, lang.L("Album peak"), fmt.Sprintf("%0.6f", t.track.ReplayGain.AlbumPeak))
	}

	title := widget.NewRichTextWithText(lang.L("Track Info"))
	title.Segments[0].(*widget.TextSegment).Style.TextStyle.Bold = true
	dismissBtn := widget.NewButton(lang.L("Close"), func() {
		if t.OnDismiss != nil {
			t.OnDismiss()
		}
	})

	return widget.NewSimpleRenderer(
		container.NewBorder(
			/*top*/ container.NewHBox(layout.NewSpacer(), title, layout.NewSpacer()),
			/*bottom*/ container.NewVBox(
				widget.NewSeparator(),
				container.NewHBox(layout.NewSpacer(), dismissBtn),
			),
			/*left/right*/ nil, nil,
			/*center*/ container.New(layout.NewCustomPaddedLayout(10, 10, 15, 15),
				container.NewScroll(c)),
		),
	)
}

func addFormRow(c *fyne.Container, left, right string) {
	if right == "" {
		return
	}
	c.Add(newFormText(left, true))
	c.Add(newFormText(right, false))
}

func newFormText(text string, leftCol bool) *widget.RichText {
	alignment := fyne.TextAlignLeading
	if leftCol {
		alignment = fyne.TextAlignTrailing
	}
	rt := widget.NewRichText(
		&widget.TextSegment{
			Text: text,
			Style: widget.RichTextStyle{
				TextStyle: fyne.TextStyle{Bold: leftCol},
				Alignment: alignment,
			},
		},
	)
	if !leftCol {
		rt.Wrapping = fyne.TextWrapWord
	}
	return rt
}
