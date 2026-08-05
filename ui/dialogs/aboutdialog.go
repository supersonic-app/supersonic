package dialogs

import (
	"fmt"
	"net/url"

	"github.com/supersonic-app/supersonic/res"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type AboutDialog struct {
	widget.BaseWidget

	OnDismiss func()

	content fyne.CanvasObject
}

func NewAboutDialog(version string) *AboutDialog {
	a := &AboutDialog{}
	a.ExtendBaseWidget(a)

	a.content = container.NewVBox(
		container.NewAppTabs(
			container.NewTabItem("Supersonic", a.buildMainTabContainer(version)),
			container.NewTabItem("Credits", a.buildCreditsContainer()),
			container.NewTabItem("GPL v3", container.NewScroll(a.licenseLabel(res.ResLICENSE))),
			container.NewTabItem("BSD-3-Clause", container.NewScroll(a.licenseLabel(res.ResBSDLICENSE))),
			container.NewTabItem("MIT", container.NewScroll(a.licenseLabel(res.ResMITLICENSE))),
		),
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
	return a
}

func (a *AboutDialog) MinSize() fyne.Size {
	return fyne.NewSize(420, a.BaseWidget.MinSize().Height)
}

func (a *AboutDialog) buildMainTabContainer(version string) *fyne.Container {
	iconImage := canvas.NewImageFromResource(res.ResAppicon256Png)
	iconImage.FillMode = canvas.ImageFillContain
	iconImage.SetMinSize(fyne.NewSize(64, 64))
	title := widget.NewRichTextWithText("Supersonic")
	title.Segments[0].(*widget.TextSegment).Style.TextStyle.Bold = true
	title.Segments[0].(*widget.TextSegment).Style.SizeName = theme.SizeNameSubHeadingText
	title.Segments[0].(*widget.TextSegment).Style.Alignment = fyne.TextAlignCenter
	versionLbl := newCenterAlignLabel(fmt.Sprintf("%s %s", lang.L("version"), version))
	copyright := newCenterAlignLabel(res.Copyright)
	license := widget.NewRichTextWithText("GNU General Public License version 3 (GPL v3)")
	ts := license.Segments[0].(*widget.TextSegment)
	ts.Style.TextStyle.Bold = true
	ts.Style.Alignment = fyne.TextAlignCenter
	ghUrl, _ := url.Parse(res.GithubURL)
	kofiUrl, _ := url.Parse(res.KofiURL)
	githubKofi := container.NewCenter(
		container.New(layout.NewCustomPaddedHBoxLayout(-10),
			widget.NewHyperlink(lang.L("Github page"), ghUrl),
			widget.NewLabel("·"),
			widget.NewHyperlink(lang.L("Support the project"), kofiUrl)),
	)

	return container.New(&layout.CustomPaddedLayout{TopPadding: 10, BottomPadding: 10},
		container.NewVBox(iconImage,
			container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-10), title, versionLbl, copyright, license, githubKofi)))
}

func (a *AboutDialog) licenseLabel(res *fyne.StaticResource) *widget.Label {
	str := string(res.StaticContent)
	lbl := widget.NewLabel(str)
	lbl.Wrapping = fyne.TextWrapWord
	return lbl
}

func (a *AboutDialog) buildCreditsContainer() fyne.CanvasObject {
	fyneURL, _ := url.Parse("https://fyne.io")
	goSubsonicURL, _ := url.Parse("https://github.com/delucks/go-subsonic")
	goTomlURL, _ := url.Parse("https://github.com/pelletier/go-toml")
	freepikURL, _ := url.Parse("https://www.flaticon.com/authors/freepik")
	appIconCredit := widget.NewLabel("The Supersonic app icon is a derivative of a work created by Piotr Siedlecki and placed in the public domain.")
	appIconCredit.Wrapping = fyne.TextWrapWord
	return container.New(layout.NewCustomPaddedVBoxLayout(theme.Padding()-10),
		widget.NewLabel("Major frameworks and modules used in this application include:"),
		container.NewHBox(widget.NewHyperlink("Fyne toolkit", fyneURL), widget.NewLabel("BSD 3-Clause License")),
		container.NewHBox(widget.NewHyperlink("go-subsonic", goSubsonicURL), widget.NewLabel("GPL v3 License")),
		container.NewHBox(widget.NewHyperlink("go-toml", goTomlURL), widget.NewLabel("MIT License")),
		appIconCredit,
		container.NewHBox(widget.NewLabel("Additional icons by:"), widget.NewHyperlink("Freepik on flaticon.com", freepikURL)),
	)
}

func (a *AboutDialog) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.content)
}

func newCenterAlignLabel(text string) *widget.Label {
	lbl := widget.NewLabel(text)
	lbl.Alignment = fyne.TextAlignCenter
	return lbl
}
