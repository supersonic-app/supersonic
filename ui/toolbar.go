package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/ui/browsing"
	"github.com/supersonic-app/supersonic/ui/controller"
	"github.com/supersonic-app/supersonic/ui/layouts"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

type Toolbar struct {
	widget.BaseWidget

	browsingPane *browsing.BrowsingPane

	home    *ttwidget.Button
	forward *ttwidget.Button
	back    *ttwidget.Button
	reload  *ttwidget.Button

	navBtnsContainer *fyne.Container
	navBtnsPageMap   map[controller.PageName]fyne.Resource
	radioBtn         fyne.CanvasObject

	quickSearchBtn *ttwidget.Button
	sidebarBtn     *ttwidget.Button
	settingsBtn    *ttwidget.Button
	settingsMenu   *fyne.Menu
}

func NewToolbar(browsingPane *browsing.BrowsingPane, navigateFn func(controller.Route), homeFunc, showSearchFn, toggleSidebarFn func()) *Toolbar {
	t := &Toolbar{
		browsingPane:     browsingPane,
		navBtnsContainer: container.NewHBox(),
		navBtnsPageMap:   make(map[controller.PageName]fyne.Resource),
		settingsMenu:     fyne.NewMenu(""),
	}
	t.ExtendBaseWidget(t)

	browsingPane.OnHistoryChanged = t.updateNavButtons

	t.home = ttwidget.NewButtonWithIcon("", theme.HomeIcon(), homeFunc)
	t.home.SetToolTip(lang.L("Home"))
	t.back = ttwidget.NewButtonWithIcon("", theme.NavigateBackIcon(), browsingPane.GoBack)
	t.back.SetToolTip(lang.L("Back"))
	t.forward = ttwidget.NewButtonWithIcon("", theme.NavigateNextIcon(), browsingPane.GoForward)
	t.forward.SetToolTip(lang.L("Forward"))
	t.reload = ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), browsingPane.Reload)
	t.reload.SetToolTip(lang.L("Reload"))

	t.setupNavigationButtons(navigateFn)

	t.quickSearchBtn = ttwidget.NewButtonWithIcon("", theme.SearchIcon(), showSearchFn)
	t.quickSearchBtn.SetToolTip(lang.L("Search Everywhere"))
	t.sidebarBtn = ttwidget.NewButtonWithIcon("", myTheme.SidebarIcon, toggleSidebarFn)
	t.sidebarBtn.SetToolTip(lang.L("Toggle sidebar"))
	t.settingsBtn = ttwidget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
		p := widget.NewPopUpMenu(t.settingsMenu,
			fyne.CurrentApp().Driver().CanvasForObject(t.settingsBtn))
		p.ShowAtPosition(fyne.NewPos(t.Size().Width-p.MinSize().Width+4,
			t.navBtnsContainer.MinSize().Height+theme.Padding()))
	})
	t.settingsBtn.SetToolTip(lang.L("Menu"))

	t.updateNavButtons()
	return t
}

// SetRadioButtonVisible sets whether the radio button is visible
func (t *Toolbar) SetRadioButtonVisible(vis bool) {
	if vis {
		t.radioBtn.Show()
	} else {
		t.radioBtn.Hide()
	}
}

// AddSettingsMenuItem adds an item to the Settings menu
func (t *Toolbar) AddSettingsMenuItem(label string, icon fyne.Resource, action func()) {
	item := fyne.NewMenuItem(label, action)
	item.Icon = icon
	t.settingsMenu.Items = append(t.settingsMenu.Items, item)
}

// AddSettingsSubmenu adds a submenu to the Settings menu
func (t *Toolbar) AddSettingsSubmenu(label string, icon fyne.Resource, menu *fyne.Menu) {
	item := fyne.NewMenuItem(label, nil)
	item.ChildMenu = menu
	item.Icon = icon
	t.settingsMenu.Items = append(t.settingsMenu.Items, item)
}

// AddSettingsMenuSeparator adds a separator to the settings menu
func (t *Toolbar) AddSettingsMenuSeparator() {
	t.settingsMenu.Items = append(t.settingsMenu.Items,
		fyne.NewMenuItemSeparator())
}

// SetSubmenuForMenuItem sets (changes) the submenu for the first menu item matching the label
func (t *Toolbar) SetSubmenuForMenuItem(label string, submenu *fyne.Menu) {
	for _, item := range t.settingsMenu.Items {
		if item.Label == label {
			item.ChildMenu = submenu
			return
		}
	}
}

// DisableNavigationButtons disables all navigation buttons
func (t *Toolbar) DisableNavigationButtons() {
	for _, obj := range t.navBtnsContainer.Objects {
		obj.(fyne.Disableable).Disable()
	}
}

// EnableNavigationButtons enables all navigation buttons
func (t *Toolbar) EnableNavigationButtons() {
	for _, obj := range t.navBtnsContainer.Objects {
		obj.(fyne.Disableable).Enable()
	}
}

// ActivateNavigationButton executes the button action for the nth navigation button
func (t *Toolbar) ActivateNavigationButton(n int) {
	if n < len(t.navBtnsContainer.Objects) {
		btn := t.navBtnsContainer.Objects[n].(*ttwidget.Button)
		if !btn.Disabled() && !btn.Hidden {
			btn.OnTapped()
		}
	}
}

func (t *Toolbar) CreateRenderer() fyne.WidgetRenderer {
	content := container.New(layouts.NewLeftMiddleRightLayout(0, 0),
		container.NewHBox(t.home, t.back, t.forward, t.reload),
		t.navBtnsContainer,
		container.NewHBox(layout.NewSpacer(), t.quickSearchBtn, t.sidebarBtn, t.settingsBtn))
	return widget.NewSimpleRenderer(content)
}

func (t *Toolbar) setupNavigationButtons(navigateFn func(controller.Route)) {
	t.addNavigationButton(myTheme.HeadphonesIcon, controller.NowPlaying, func() {
		navigateFn(controller.NowPlayingRoute())
	})
	t.addNavigationButton(myTheme.FavoriteIcon, controller.Favorites, func() {
		navigateFn(controller.FavoritesRoute())
	})
	t.addNavigationButton(myTheme.AlbumIcon, controller.Albums, func() {
		navigateFn(controller.AlbumsRoute())
	})
	t.addNavigationButton(myTheme.ArtistIcon, controller.Artists, func() {
		navigateFn(controller.ArtistsRoute())
	})
	t.addNavigationButton(myTheme.GenreIcon, controller.Genres, func() {
		navigateFn(controller.GenresRoute())
	})
	t.addNavigationButton(myTheme.PlaylistIcon, controller.Playlists, func() {
		navigateFn(controller.PlaylistsRoute())
	})
	t.addNavigationButton(myTheme.TracksIcon, controller.Tracks, func() {
		navigateFn(controller.TracksRoute())
	})
	t.radioBtn = t.addNavigationButton(myTheme.RadioIcon, controller.Radios, func() {
		navigateFn(controller.RadiosRoute())
	})
}

func (t *Toolbar) addNavigationButton(icon fyne.Resource, pageName controller.PageName, action func()) *ttwidget.Button {
	// make a copy of the icon, because it can change the color
	browsingPaneIcon := theme.NewThemedResource(icon)
	btn := ttwidget.NewButtonWithIcon("", browsingPaneIcon, action)
	btn.SetToolTip(lang.L(pageName.String()))
	t.navBtnsContainer.Add(btn)
	t.navBtnsPageMap[pageName] = browsingPaneIcon
	return btn
}

func (t *Toolbar) updateNavButtons() {
	if t.browsingPane.CanGoBack() {
		t.back.Enable()
	} else {
		t.back.Disable()
	}
	if t.browsingPane.CanGoForward() {
		t.forward.Enable()
	} else {
		t.forward.Disable()
	}

	p := t.browsingPane.CurrentPage()
	for pageName, icon := range t.navBtnsPageMap {
		if pageName == p.Page {
			icon.(*theme.ThemedResource).ColorName = theme.ColorNamePrimary
		} else {
			icon.(*theme.ThemedResource).ColorName = theme.ColorNameForeground
		}
	}
	t.Refresh()
}
