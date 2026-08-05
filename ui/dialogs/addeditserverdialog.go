package dialogs

import (
	"fmt"

	"github.com/supersonic-app/supersonic/backend"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type AddEditServerDialog struct {
	widget.BaseWidget

	ServerType    backend.ServerType
	Nickname      string
	Host          string
	AltHost       string
	Username      string
	Password      string
	LegacyAuth    bool
	SkipSSLVerify bool
	OnSubmit      func()
	OnCancel      func()

	passField  *widget.Entry
	submitBtn  *widget.Button
	promptText *widget.RichText
	container  *fyne.Container
}

var _ fyne.Widget = (*AddEditServerDialog)(nil)

func NewAddEditServerDialog(title string, cancelable bool, prefillServer *backend.ServerConfig, focusHandler func(fyne.Focusable)) *AddEditServerDialog {
	a := &AddEditServerDialog{}
	a.ExtendBaseWidget(a)
	if prefillServer != nil {
		a.ServerType = prefillServer.ServerType
		a.Nickname = prefillServer.Nickname
		a.Host = prefillServer.Hostname
		a.AltHost = prefillServer.AltHostname
		a.Username = prefillServer.Username
		a.LegacyAuth = prefillServer.LegacyAuth
		a.SkipSSLVerify = prefillServer.SkipSSLVerify
	}

	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle.Bold = true
	legacyAuthCheck := widget.NewCheckWithData(lang.L("Use legacy authentication"), binding.BindBool(&a.LegacyAuth))
	serverTypeChoice := widget.NewRadioGroup([]string{"Subsonic", "Jellyfin"}, func(s string) {
		a.ServerType = backend.ServerType(s)
		if s == string(backend.ServerTypeSubsonic) {
			legacyAuthCheck.Show()
		} else {
			legacyAuthCheck.Hide()
		}
	})
	skipSSLCheck := widget.NewCheckWithData(lang.L("Skip SSL certificate verification"), binding.BindBool(&a.SkipSSLVerify))
	serverTypeChoice.Required = true
	serverTypeChoice.Horizontal = true
	selected := backend.ServerTypeSubsonic
	if a.ServerType == backend.ServerTypeJellyfin {
		selected = backend.ServerTypeJellyfin
	}
	serverTypeChoice.Selected = string(selected)
	a.passField = widget.NewPasswordEntry()
	a.passField.OnSubmitted = func(_ string) { a.doSubmit() }
	userField := widget.NewEntryWithData(binding.BindString(&a.Username))
	userField.OnSubmitted = func(_ string) { focusHandler(a.passField) }
	altHostField := widget.NewEntryWithData(binding.BindString(&a.AltHost))
	altHostField.SetPlaceHolder(fmt.Sprintf("(%s)", lang.L("optional")) + " https://my-external-domain.net/music")
	altHostField.OnSubmitted = func(_ string) { focusHandler(userField) }
	hostField := widget.NewEntryWithData(binding.BindString(&a.Host))
	hostField.SetPlaceHolder("http://localhost:4533")
	hostField.OnSubmitted = func(_ string) { focusHandler(altHostField) }
	nickField := widget.NewEntryWithData(binding.BindString(&a.Nickname))
	nickField.SetPlaceHolder(lang.L("My Server"))
	nickField.OnSubmitted = func(_ string) { focusHandler(hostField) }
	a.submitBtn = widget.NewButtonWithIcon(lang.L("Enter"), theme.ConfirmIcon(), a.doSubmit)
	a.submitBtn.Importance = widget.HighImportance
	a.promptText = widget.NewRichTextWithText("")
	a.promptText.Hidden = true

	var bottomRow *fyne.Container
	if cancelable {
		bottomRow = container.NewHBox(
			a.promptText,
			layout.NewSpacer(),
			widget.NewButtonWithIcon(lang.L("Cancel"), theme.CancelIcon(), a.onCancel),
			a.submitBtn)
	} else {
		bottomRow = container.NewHBox(
			a.promptText,
			layout.NewSpacer(),
			a.submitBtn)
	}

	a.container = container.NewVBox(
		container.NewHBox(layout.NewSpacer(), titleLabel, layout.NewSpacer()),
		container.New(layout.NewFormLayout(),
			widget.NewLabel(lang.L("Server Type")),
			serverTypeChoice,
			widget.NewLabel(lang.L("Nickname")),
			nickField,
			widget.NewLabel(lang.L("URL")),
			hostField,
			widget.NewLabel(lang.L("Alt. URL")),
			altHostField,
			widget.NewLabel(lang.L("Username")),
			userField,
			widget.NewLabel(lang.L("Password")),
			a.passField,
		),
		container.NewHBox(layout.NewSpacer(), legacyAuthCheck, skipSSLCheck),
		widget.NewSeparator(),
		bottomRow,
	)
	return a
}

func (a *AddEditServerDialog) SetInfoText(text string) {
	a.doSetPromptText(text, theme.ColorNameForeground)
}

func (a *AddEditServerDialog) SetErrorText(text string) {
	a.doSetPromptText(text, theme.ColorNameError)
}

func (a *AddEditServerDialog) EnableSubmit() {
	a.submitBtn.Enable()
	a.submitBtn.Refresh()
}

func (a *AddEditServerDialog) DisableSubmit() {
	a.submitBtn.Disable()
	a.submitBtn.Refresh()
}

func (a *AddEditServerDialog) doSubmit() {
	a.Password = a.passField.Text
	if a.OnSubmit != nil {
		a.OnSubmit()
	}
}

func (a *AddEditServerDialog) onCancel() {
	if a.OnCancel != nil {
		a.OnCancel()
	}
}

func (a *AddEditServerDialog) doSetPromptText(text string, color fyne.ThemeColorName) {
	ts := a.promptText.Segments[0].(*widget.TextSegment)
	ts.Text = text
	ts.Style.ColorName = color
	if text != "" {
		a.promptText.Show()
	} else {
		a.promptText.Hide()
	}
	a.promptText.Refresh()
}

func (a *AddEditServerDialog) MinSize() fyne.Size {
	a.ExtendBaseWidget(a)
	return fyne.NewSize(475, a.container.MinSize().Height)
}

func (a *AddEditServerDialog) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.container)
}
