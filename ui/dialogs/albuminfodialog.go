package dialogs

import (
	"fmt"
	"image"
	"net/url"
	"strings"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"

	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const musicBrainzReleaseUrl = "https://musicbrainz.org/release"

type AlbumInfoDialog struct {
	widget.BaseWidget

	OnDismiss func()

	mainContainer   *fyne.Container
	bottomContainer *fyne.Container
	content         fyne.CanvasObject
}

func NewAlbumInfoDialog(albumInfo *mediaprovider.AlbumInfo, albumName string, albumCover image.Image) *AlbumInfoDialog {
	a := &AlbumInfoDialog{}
	a.ExtendBaseWidget(a)

	a.content = container.NewBorder(nil, /*top*/
		a.buildBottomContainer(),
		nil /*left*/, nil, /*right*/
		a.buildMainContainer(albumInfo, albumName, albumCover), /*content*/
	)
	return a
}

func (a *AlbumInfoDialog) MinSize() fyne.Size {
	return fyne.NewSize(550, a.BaseWidget.MinSize().Height)
}

func (a *AlbumInfoDialog) buildBottomContainer() *fyne.Container {
	a.bottomContainer = container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(
			layout.NewSpacer(),
			widget.NewButton(lang.L("Close"), func() {
				if a.OnDismiss != nil {
					a.OnDismiss()
				}
			}),
		),
	)
	return a.bottomContainer
}

func (a *AlbumInfoDialog) buildMainContainer(albumInfo *mediaprovider.AlbumInfo, albumName string, albumCover image.Image) *fyne.Container {
	iconImage := canvas.NewImageFromImage(albumCover)
	iconImage.FillMode = canvas.ImageFillContain
	iconImage.SetMinSize(fyne.NewSize(100, 100))
	iconImage.Hidden = albumCover == nil
	title := widget.NewRichTextWithText(albumName)
	title.Segments[0].(*widget.TextSegment).Style.TextStyle.Bold = true
	title.Segments[0].(*widget.TextSegment).Style.SizeName = theme.SizeNameSubHeadingText
	title.Segments[0].(*widget.TextSegment).Style.Alignment = fyne.TextAlignCenter
	title.Truncation = fyne.TextTruncateEllipsis

	infoContent := widget.NewLabel(lang.L("Album info not available"))

	if albumInfo.Notes != "" {
		infoContent = a.infoLabel(albumInfo.Notes)
	}

	urlContainer := a.buildUrlContainer(albumInfo.LastFmUrl, albumInfo.MusicBrainzID)

	a.mainContainer = container.New(
		&layout.CustomPaddedLayout{LeftPadding: 15, RightPadding: 10, TopPadding: 15, BottomPadding: 10},
		container.NewScroll(
			container.NewVBox(
				iconImage,
				title,
				infoContent,
				urlContainer,
			),
		),
	)
	return a.mainContainer
}

func (a *AlbumInfoDialog) NonScrollingMinHeight() float32 {
	l := a.mainContainer.Layout.(*layout.CustomPaddedLayout)
	s := a.mainContainer.Objects[0].(*container.Scroll)
	return l.TopPadding + l.BottomPadding + s.Content.MinSize().Height + theme.Padding()*3 + a.bottomContainer.MinSize().Height
}

func (a *AlbumInfoDialog) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.content)
}

func (a *AlbumInfoDialog) infoLabel(info string) *widget.Label {
	if last := strings.LastIndex(info, "<a href"); last > 0 {
		info = strings.TrimSpace(info[:last])
	}
	lbl := widget.NewLabel(info)
	lbl.Wrapping = fyne.TextWrapWord
	lbl.Alignment = fyne.TextAlignLeading
	return lbl
}

func (a *AlbumInfoDialog) buildUrlContainer(lastFm, musicBrainzID string) *fyne.Container {
	urls := make([]*widget.Hyperlink, 0)

	if lastFm != "" {
		if lastFmUrl, err := url.Parse(lastFm); err == nil {
			urls = append(urls, widget.NewHyperlink("Last.fm", lastFmUrl))
		}
	}

	if musicBrainzID != "" {
		if musicBrainzUrl, err := url.Parse(fmt.Sprintf("%s/%s", musicBrainzReleaseUrl, musicBrainzID)); err == nil {
			urls = append(urls, widget.NewHyperlink("MusicBrainz", musicBrainzUrl))
		}
	}

	urlContainer := container.New(layout.NewCustomPaddedHBoxLayout(-10))
	for index, url := range urls {
		if index > 0 {
			urlContainer.Add(widget.NewLabel("·"))
		}
		urlContainer.Add(url)
	}

	return container.NewCenter(urlContainer)
}
