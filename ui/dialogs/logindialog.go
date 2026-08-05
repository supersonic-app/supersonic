package dialogs

import (
	"github.com/google/uuid"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/sharedutil"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type PasswordFetchFunc func(serverID uuid.UUID) (string, error)

type LoginDialog struct {
	widget.BaseWidget

	OnSubmit       func(server *backend.ServerConfig, password string)
	OnEditServer   func(server *backend.ServerConfig)
	OnDeleteServer func(server *backend.ServerConfig)
	OnNewServer    func()

	servers []*backend.ServerConfig

	serverSelect *widget.Select
	passField    *widget.Entry
	promptText   *widget.RichText
	submitBtn    *widget.Button

	container *fyne.Container
}

var _ fyne.Widget = (*LoginDialog)(nil)

func NewLoginDialog(servers []*backend.ServerConfig, pwFetch PasswordFetchFunc) *LoginDialog {
	l := &LoginDialog{servers: servers}
	l.ExtendBaseWidget(l)
	titleLabel := widget.NewLabel(lang.L("Login to Server"))
	titleLabel.TextStyle.Bold = true
	l.passField = widget.NewPasswordEntry()
	l.passField.OnSubmitted = func(_ string) { l.onSubmit() }

	serverNames := sharedutil.MapSlice(servers, func(s *backend.ServerConfig) string { return s.Nickname })
	l.serverSelect = widget.NewSelect(serverNames, func(_ string) {
		if pwFetch != nil {
			if pw, err := pwFetch(servers[l.serverSelect.SelectedIndex()].ID); err == nil {
				l.passField.SetText(pw)
				return
			}
		}
		l.passField.SetText("")
	})
	l.serverSelect.SetSelectedIndex(0)

	editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), l.onEditServer)
	newBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), l.onNewServer)
	deleteBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { l.onDeleteServer(l.serverSelect.SelectedIndex()) })
	l.submitBtn = widget.NewButtonWithIcon(lang.L("OK"), theme.ConfirmIcon(), l.onSubmit)
	l.submitBtn.Importance = widget.HighImportance
	l.promptText = widget.NewRichTextWithText("")
	l.promptText.Segments[0].(*widget.TextSegment).Style.ColorName = theme.ColorNameError
	l.promptText.Hidden = true

	l.container = container.NewVBox(
		container.NewHBox(layout.NewSpacer(), titleLabel, layout.NewSpacer()),
		container.New(layout.NewFormLayout(),
			widget.NewLabel(lang.L("Server")),
			container.NewBorder(nil, nil, nil, container.NewHBox(editBtn, newBtn, deleteBtn), l.serverSelect),
			widget.NewLabel(lang.L("Password")),
			l.passField),
		widget.NewSeparator(),
		container.NewHBox(l.promptText, layout.NewSpacer(), l.submitBtn),
	)
	return l
}

func (l *LoginDialog) SetInfoText(text string) {
	l.doSetPromptText(text, theme.ColorNameForeground)
}

func (l *LoginDialog) SetErrorText(text string) {
	l.doSetPromptText(text, theme.ColorNameError)
}

func (l *LoginDialog) SetServers(servers []*backend.ServerConfig) {
	l.servers = servers
	l.serverSelect.Options = sharedutil.MapSlice(servers, func(s *backend.ServerConfig) string { return s.Nickname })
	if len(servers) > 0 {
		l.serverSelect.SetSelectedIndex(0)
	}
}

func (l *LoginDialog) EnableSubmit() {
	l.submitBtn.Enable()
	l.submitBtn.Refresh()
}

func (l *LoginDialog) DisableSubmit() {
	l.submitBtn.Disable()
	l.submitBtn.Refresh()
}

func (l *LoginDialog) doSetPromptText(text string, color fyne.ThemeColorName) {
	ts := l.promptText.Segments[0].(*widget.TextSegment)
	ts.Text = text
	ts.Style.ColorName = color
	if text != "" {
		l.promptText.Show()
	} else {
		l.promptText.Hide()
	}
	l.promptText.Refresh()
}

func (l *LoginDialog) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(l.container)
}

func (l *LoginDialog) MinSize() fyne.Size {
	l.ExtendBaseWidget(l)
	return fyne.NewSize(375, l.container.MinSize().Height)
}

func (l *LoginDialog) onSubmit() {
	if l.OnSubmit != nil {
		l.OnSubmit(l.servers[l.serverSelect.SelectedIndex()], l.passField.Text)
	}
}

func (l *LoginDialog) onEditServer() {
	if l.OnEditServer != nil {
		l.OnEditServer(l.servers[l.serverSelect.SelectedIndex()])
	}
}

func (l *LoginDialog) onNewServer() {
	if l.OnNewServer != nil {
		l.OnNewServer()
	}
}

func (l *LoginDialog) onDeleteServer(idx int) {
	if l.OnDeleteServer != nil {
		l.OnDeleteServer(l.servers[idx])
	}
}
