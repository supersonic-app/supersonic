package ui

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"time"

	fynetooltip "github.com/dweymouth/fyne-tooltip"
	"github.com/dweymouth/supersonic/backend"
	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/backend/windows"
	"github.com/dweymouth/supersonic/res"
	"github.com/dweymouth/supersonic/ui/browsing"
	uicontainer "github.com/dweymouth/supersonic/ui/container"
	"github.com/dweymouth/supersonic/ui/controller"
	"github.com/dweymouth/supersonic/ui/dialogs"
	"github.com/dweymouth/supersonic/ui/shortcuts"
	myTheme "github.com/dweymouth/supersonic/ui/theme"
	"github.com/dweymouth/supersonic/ui/util"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type MainWindow struct {
	Window fyne.Window

	App          *backend.App
	Router       browsing.Router
	Controller   *controller.Controller
	BrowsingPane *browsing.BrowsingPane
	Sidebar      *Sidebar
	Toolbar      *Toolbar
	BottomPanel  *BottomPanel
	ToastOverlay *ToastOverlay

	splitContainer   *uicontainer.Split
	theme            *myTheme.MyTheme
	haveSystemTray   bool
	alreadyConnected bool // tracks if we have already connected to a server before
	content          *mainWindowContent

	// updated when changing servers or libraries
	librarySubmenu *fyne.Menu

	isScreensaverDisabled bool
}

func NewMainWindow(fyneApp fyne.App, appName, displayAppName, appVersion string, app *backend.App) MainWindow {
	m := MainWindow{
		App:    app,
		Window: fyneApp.NewWindow(displayAppName),
		theme:  myTheme.NewMyTheme(&app.Config.Theme, app.ThemesDir()),
	}
	installReopenHandler(m.Window)
	installDockMenu(m.createSystemTrayAndDockMenu(false))
	fynetooltip.SetToolTipTextSizeName(myTheme.SizeNameSubText)

	m.theme.NormalFont = app.Config.Application.FontNormalTTF
	m.theme.BoldFont = app.Config.Application.FontBoldTTF
	fyneApp.Settings().SetTheme(m.theme)

	if app.Config.Application.EnableSystemTray {
		m.SetupSystemTrayMenu(displayAppName, fyneApp)
	}
	m.Controller = controller.New(app, appVersion, m.Window)
	m.BrowsingPane = browsing.NewBrowsingPane(app.PlaybackManager, m.Controller, func() { m.Router.NavigateTo(m.StartupPage()) })
	m.Sidebar = NewSidebar(m.Controller, m.App.PlaybackManager, m.App.ImageManager, m.App.LyricsManager, &m.App.Config.Application)
	if m.App.Config.Application.SidebarTab == "Lyrics" {
		m.Sidebar.SetSelectedIndex(1)
	}
	m.ToastOverlay = NewToastOverlay()
	m.Router = browsing.NewRouter(app, m.Controller, m.BrowsingPane)
	goHomeFn := func() { m.Router.NavigateTo(m.StartupPage()) }
	m.Toolbar = NewToolbar(m.BrowsingPane, m.Router.NavigateTo, goHomeFn, m.Controller.ShowQuickSearch, m.toggleSidebar)

	// inject controller dependencies
	m.Controller.NavHandler = m.Router.NavigateTo
	m.Controller.ReloadFunc = m.BrowsingPane.Reload
	m.Controller.CurPageFunc = m.BrowsingPane.CurrentPage
	m.Controller.RefreshPageFunc = func() {
		m.BrowsingPane.RefreshPage()
		m.BottomPanel.Refresh()
	}
	m.Controller.SelectAllPageFunc = m.BrowsingPane.SelectAll
	m.Controller.UnselectAllPageFunc = m.BrowsingPane.UnselectAll
	m.Controller.ToastProvider = m.ToastOverlay

	if runtime.GOOS == "darwin" {
		// Fyne will extract out an "About" menu item and
		// assign its action to the native Mac "About Supersonic" menu item
		m.Window.SetMainMenu(fyne.NewMainMenu(
			fyne.NewMenu("File", /*name doesn't matter*/
				fyne.NewMenuItem("About", func() {
					m.Window.Show()
					m.Controller.ShowAboutDialog()
				})),
		))
	}

	m.BottomPanel = NewBottomPanel(app.PlaybackManager, app.ImageManager, m.Controller, app.Config)
	app.PlaybackManager.OnSongChange(func(item mediaprovider.MediaItem, _ *mediaprovider.Track) {
		fyne.Do(func() { m.UpdateOnTrackChange(item) })
	})
	app.PlaybackManager.OnRadioMetadataChange(func(radioName, title, artist string) {
		fyne.Do(func() {
			if title != "" {
				m.Window.SetTitle(fmt.Sprintf("%s – %s · %s", title, artist, res.DisplayName))
			} else {
				m.Window.SetTitle(fmt.Sprintf("%s · %s", radioName, res.DisplayName))
			}
		})
	})
	app.PlaybackManager.OnQueueChange(func() {
		fyne.Do(func() {
			m.Sidebar.SetQueueTracks(app.PlaybackManager.GetActivePlayQueue(), app.PlaybackManager.NowPlayingIndex())
		})
	})
	app.ServerManager.OnServerConnected(func(conf *backend.ServerConfig) {
		go m.RunOnServerConnectedTasks(conf, app, displayAppName)
	})
	app.ServerManager.OnLogout(func() {
		m.Toolbar.DisableNavigationButtons()
		m.BrowsingPane.SetPage(nil)
		m.BrowsingPane.ClearHistory()
		m.Controller.PromptForLoginAndConnect()
	})
	m.Toolbar.AddSettingsMenuItem(lang.L("Log Out"), theme.LogoutIcon(), func() { app.ServerManager.Logout(true) })
	m.Toolbar.AddSettingsMenuItem(lang.L("Switch Servers"), theme.LoginIcon(), func() { app.ServerManager.Logout(false) })
	m.Toolbar.AddSettingsSubmenu(lang.L("Select Library"), myTheme.LibraryIcon, fyne.NewMenu("",
		fyne.NewMenuItem(lang.L("All Libraries"), func() { /* dummy - will get replaced on server login */ })))
	m.Toolbar.AddSettingsMenuItem(lang.L("Rescan Library"), theme.ViewRefreshIcon(), func() { app.ServerManager.Server.RescanLibrary() })
	m.Toolbar.AddSettingsMenuSeparator()
	m.Toolbar.AddSettingsSubmenu(lang.L("Visualizations"), myTheme.VisualizationIcon,
		fyne.NewMenu("", []*fyne.MenuItem{
			fyne.NewMenuItem(lang.L("Peak Meter"), m.Controller.ShowPeakMeter),
		}...))
	m.Toolbar.AddSettingsMenuSeparator()
	m.Toolbar.AddSettingsMenuItem(lang.L("Check for Updates"), theme.DownloadIcon(), func() {
		go func() {
			if t := app.UpdateChecker.CheckLatestVersionTag(); t != "" && t != app.VersionTag() {
				fyne.Do(func() { m.ShowNewVersionDialog(displayAppName, t) })
			} else {
				fyne.Do(func() {
					dialog.ShowInformation(lang.L("No new version found"),
						lang.L("You are running the latest version of")+" "+displayAppName,
						m.Window)
				})
			}
		}()
	})
	m.Toolbar.AddSettingsMenuItem(lang.L("Settings")+"...", theme.SettingsIcon(), m.showSettingsDialog)
	m.Toolbar.AddSettingsMenuItem(lang.L("About")+"...", theme.InfoIcon(), m.Controller.ShowAboutDialog)
	m.Toolbar.DisableNavigationButtons()
	m.addShortcuts()

	m.splitContainer = uicontainer.NewHSplit(m.BrowsingPane, m.Sidebar)
	m.splitContainer.Offset = min(1.0, max(0.0, m.App.Config.Application.SidebarWidthFraction))
	m.Sidebar.Hidden = !m.App.Config.Application.ShowSidebar
	center := container.NewStack(m.splitContainer, m.ToastOverlay)
	toolbarWrapper := container.New(
		&layout.CustomPaddedLayout{LeftPadding: -theme.Padding(), RightPadding: -theme.Padding()}, m.Toolbar)
	m.content = newMainWindowContent(container.NewBorder(toolbarWrapper, m.BottomPanel, nil, nil, center),
		m.Controller.UnselectAll)
	m.Window.SetContent(fynetooltip.AddWindowToolTipLayer(m.content, m.Window.Canvas()))
	m.setInitialSize()

	m.Router.OnNavigateTo = func(r controller.Route) {
		if r.Page == controller.NowPlayingRoute().Page {
			if m.App.Config.Application.PreventScreensaverOnNowPlayingPage && !m.isScreensaverDisabled {
				fyne.CurrentApp().Driver().SetDisableScreenBlanking(true)
				m.isScreensaverDisabled = true
			}
		} else if m.isScreensaverDisabled {
			fyne.CurrentApp().Driver().SetDisableScreenBlanking(false)
			m.isScreensaverDisabled = false
		}
	}

	m.Window.SetCloseIntercept(func() {
		m.SaveWindowSettings()
		if (runtime.GOOS == "darwin" && !isRealQuit()) || (app.Config.Application.CloseToSystemTray && m.HaveSystemTray()) {
			m.Window.Hide()
		} else {
			m.Window.Close()
		}
	})

	return m
}

func (m *MainWindow) UpdateOnTrackChange(item mediaprovider.MediaItem) {
	if item == nil {
		m.Window.SetTitle(res.DisplayName)
		m.Sidebar.SetNowPlaying(nil, -1)
		return
	}

	meta := item.Metadata()
	m.Sidebar.SetNowPlaying(item, m.App.PlaybackManager.NowPlayingIndex())
	artistDisp := ""
	if tr, ok := item.(*mediaprovider.Track); ok {
		artistDisp = " – " + strings.Join(tr.ArtistNames, ", ")
	}
	m.Window.SetTitle(fmt.Sprintf("%s%s · %s", meta.Name, artistDisp, res.DisplayName))
	if m.App.Config.Application.ShowTrackChangeNotification {
		switch runtime.GOOS {
		case "linux":
			if notifySend, err := exec.LookPath("notify-send"); err == nil {
				go func() {
					args := []string{
						"--app-name", "supersonic",
						"--urgency", "low",
						"--expire-time", "10000",
						// replace previous notification
						"--hint", "string:x-canonical-private-synchronous:supersonic-track",
						meta.Name, strings.TrimPrefix(artistDisp, " – "),
					}

					m.App.ImageManager.GetCoverThumbnail(meta.CoverArtID)
					if path, err := m.App.ImageManager.GetCoverArtPath(meta.CoverArtID); err == nil {
						args = append([]string{"--icon", path}, args...)
					}

					if out, err := exec.Command(notifySend, args...).CombinedOutput(); err != nil {
						log.Printf("notify-send error: %s %s", strings.TrimSpace(string(out)), err)
					}
				}()
				return
			}
		case "windows":
			go func() {
				// ensure cover thumbnail is cached locally
				m.App.ImageManager.GetCoverThumbnail(meta.CoverArtID)
				path, _ := m.App.ImageManager.GetCoverArtPath(meta.CoverArtID)
				fyne.Do(func() {
					windows.SendNotification(&fyne.Notification{
						Title:   meta.Name,
						Content: artistDisp,
					}, path)
				})
			}()
			return
		}

		// fallback for not handled above
		// TODO: Once Fyne issue #2935 is resolved, show album cover on other platforms
		fyne.CurrentApp().SendNotification(&fyne.Notification{
			Title:   meta.Name,
			Content: artistDisp,
		})
	}
}

func (m *MainWindow) DesiredSize() fyne.Size {
	w := float32(m.App.Config.Application.WindowWidth)
	if w <= 1 {
		w = 1000
	}
	h := float32(m.App.Config.Application.WindowHeight)
	if h <= 1 {
		h = 800
	}
	return fyne.NewSize(w, h)
}

func (m *MainWindow) setInitialSize() {
	m.Window.Resize(m.DesiredSize())
}

func (m *MainWindow) ReloadTheme() {
	m.theme.ReloadThemeFile()
	fyne.CurrentApp().Settings().SetTheme(m.theme)
}

func (m *MainWindow) StartupPage() controller.Route {
	switch m.App.Config.Application.StartupPage {
	case "All Tracks":
		return controller.AllTracksRoute()
	case "Artists":
		return controller.ArtistsRoute()
	case "Favorites":
		return controller.FavoritesRoute()
	case "Playlists":
		return controller.PlaylistsRoute()
	default:
		return controller.AlbumsRoute()
	}
}

func (m *MainWindow) RunOnServerConnectedTasks(serverConf *backend.ServerConfig, app *backend.App, displayAppName string) {
	time.Sleep(1 * time.Millisecond) // ensure this runs after sync tasks

	if app.Config.Application.SavePlayQueue {
		go func() {
			if err := app.LoadSavedPlayQueue(); err != nil {
				log.Printf("failed to load saved play queue: %s", err.Error())
			}
		}()
	}

	doSetLibrary := func(libraryID string, menuIdx int) {
		serverConf.SelectedLibrary = libraryID
		fyne.Do(func() {
			m.App.ServerManager.Server.SetLibrary(libraryID)
			// Pages in the history could contain content
			// outside the new library, so clear history
			m.BrowsingPane.ClearHistory()
			// ... and reload current page for the same reason
			m.BrowsingPane.Reload()
			// check only the menu item for the new library
			for i, menuItem := range m.librarySubmenu.Items {
				menuItem.Checked = (i == menuIdx)
			}
		})
	}

	libraries, err := app.ServerManager.Server.GetLibraries()
	if err != nil {
		log.Printf("error loading server libraries: %s", err.Error())
	}
	libraryMenu := fyne.NewMenu("")
	libraryMenuItemOffset := 0
	if len(libraries) != 1 {
		// If there is exactly one library in the list,
		// we just want to have one menu entry with that library's name.
		// Otherwise, add the "All Libraries" menu item at the top.
		libraryMenu.Items = append(libraryMenu.Items,
			fyne.NewMenuItem(lang.L("All Libraries"), func() {
				doSetLibrary("", 0)
			}))
		libraryMenuItemOffset = 1
	}

	initialLibraryMenuIdx := 0
	initialLibraryID := ""
	for i, l := range libraries {
		_l := l
		_i := i + libraryMenuItemOffset
		libraryMenu.Items = append(libraryMenu.Items,
			fyne.NewMenuItem(_l.Name, func() {
				doSetLibrary(_l.ID, _i)
			}))
		if _l.ID == serverConf.SelectedLibrary {
			initialLibraryMenuIdx = _i
			initialLibraryID = _l.ID
		}
	}
	m.librarySubmenu = libraryMenu
	m.librarySubmenu.Items[initialLibraryMenuIdx].Checked = true
	m.Toolbar.SetSubmenuForMenuItem(lang.L("Select Library"), libraryMenu)

	if initialLibraryID != "" {
		m.App.ServerManager.Server.SetLibrary(initialLibraryID)
	}

	fyne.Do(func() {
		m.Toolbar.EnableNavigationButtons()
		m.Router.NavigateTo(m.StartupPage())
		_, canRate := m.App.ServerManager.Server.(mediaprovider.SupportsRating)
		m.BottomPanel.NowPlaying.DisableRating = !canRate

		_, supportsRadio := m.App.ServerManager.Server.(mediaprovider.RadioProvider)
		m.Toolbar.SetRadioButtonVisible(supportsRadio)
	})

	m.App.SaveConfigFile()

	if m.alreadyConnected {
		// below tasks only need to run on first time connecting to a server since launch
		return
	}

	// check if launching new version, else if found available update on startup
	fyne.Do(func() {
		if l := app.Config.Application.LastLaunchedVersion; app.VersionTag() != l {
			if !app.IsFirstLaunch() {
				m.ShowWhatsNewDialog()
			}
			m.App.Config.Application.LastLaunchedVersion = app.VersionTag()
		} else if t := app.UpdateChecker.VersionTagFound(); t != "" && t != app.Config.Application.LastCheckedVersion {
			if t != app.VersionTag() {
				m.ShowNewVersionDialog(displayAppName, t)
			}
			m.App.Config.Application.LastCheckedVersion = t
		}
	})

	// register callback for the ongoing periodic update check
	m.App.UpdateChecker.OnUpdatedVersionFound = func() {
		t := m.App.UpdateChecker.VersionTagFound()
		if t != app.VersionTag() {
			fyne.Do(func() { m.ShowNewVersionDialog(displayAppName, t) })
		}
		m.App.Config.Application.LastCheckedVersion = t
	}
	m.alreadyConnected = true
}

func (m *MainWindow) SetupSystemTrayMenu(appName string, fyneApp fyne.App) {
	if desk, ok := fyneApp.(desktop.App); ok {
		desk.SetSystemTrayMenu(m.createSystemTrayAndDockMenu(true))
		desk.SetSystemTrayIcon(res.ResAppicon256Png)
		if runtime.GOOS != "darwin" {
			// Left-click opening systray menu instead of raising window
			// is standard behavior on Mac.
			desk.SetSystemTrayWindow(m.Window)
		}
		m.haveSystemTray = true
	}
}

func (m *MainWindow) createSystemTrayAndDockMenu(includeShowAndHide bool) *fyne.Menu {
	menu := fyne.NewMenu("",
		fyne.NewMenuItem(fmt.Sprintf("%s/%s", lang.L("Play"), lang.L("Pause")), func() {
			m.App.PlaybackManager.PlayPause()
		}),
		fyne.NewMenuItem(lang.L("Previous"), func() {
			m.App.PlaybackManager.SeekBackOrPrevious()
		}),
		fyne.NewMenuItem(lang.L("Next"), func() {
			m.App.PlaybackManager.SeekNext()
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(lang.L("Volume")+" +10%", func() {
			vol := m.App.PlaybackManager.Volume()
			vol = vol + int(float64(vol)*0.1)
			// will clamp to range for us
			m.App.PlaybackManager.SetVolume(vol)
		}),
		fyne.NewMenuItem(lang.L("Volume")+" -10%", func() {
			vol := m.App.PlaybackManager.Volume()
			vol = vol - int(float64(vol)*0.1)
			m.App.PlaybackManager.SetVolume(vol)
		}),
	)
	if includeShowAndHide {
		menu.Items = append(menu.Items, fyne.NewMenuItemSeparator())
		menu.Items = append(menu.Items, fyne.NewMenuItem(lang.L("Show"), m.Window.Show))
		menu.Items = append(menu.Items, fyne.NewMenuItem(lang.L("Hide"), m.Window.Hide))
	}
	return menu
}

func (m *MainWindow) HaveSystemTray() bool {
	return m.haveSystemTray
}

func (m *MainWindow) ShowNewVersionDialog(appName, versionTag string) {
	contentStr := fmt.Sprintf("A new version of %s (%s) is available",
		appName, versionTag)
	m.Controller.QueueShowModalFunc(func() {
		dialog.ShowCustomConfirm(lang.L("A new version is available"),
			lang.L("Go to release page"), lang.L("Skip this version"),
			widget.NewLabel(contentStr), func(show bool) {
				if show {
					fyne.CurrentApp().OpenURL(m.App.UpdateChecker.LatestReleaseURL())
				}
				m.App.Config.Application.LastCheckedVersion = versionTag
			}, m.Window)
	})
}

func (m *MainWindow) ShowWhatsNewDialog() {
	dialog.ShowCustom("What's new in "+res.AppVersion, lang.L("Close"), dialogs.NewWhatsNewDialog(), m.Window)
}

func (m *MainWindow) toggleSidebar() {
	if m.Sidebar.Visible() {
		m.Sidebar.Hide()
	} else {
		m.Sidebar.Show()
	}
	m.splitContainer.Refresh()
}

func (m *MainWindow) addShortcuts() {
	for _, sh := range shortcuts.BackShortcuts {
		m.Canvas().AddShortcut(&sh, func(_ fyne.Shortcut) {
			m.BrowsingPane.GoBack()
		})
	}
	for _, sh := range shortcuts.ForwardShortcuts {
		m.Canvas().AddShortcut(&sh, func(_ fyne.Shortcut) {
			m.BrowsingPane.GoForward()
		})
	}
	if shortcuts.SettingsShortcut != nil {
		m.Canvas().AddShortcut(shortcuts.SettingsShortcut, func(_ fyne.Shortcut) {
			m.showSettingsDialog()
		})
	}
	if shortcuts.QuitShortcut != nil {
		m.Canvas().AddShortcut(shortcuts.QuitShortcut, func(_ fyne.Shortcut) {
			m.Quit()
		})
	}

	m.Canvas().AddShortcut(&shortcuts.ShortcutReload, func(_ fyne.Shortcut) {
		m.BrowsingPane.Reload()
	})
	m.Canvas().AddShortcut(&shortcuts.ShortcutSearch, func(_ fyne.Shortcut) {
		if m.Controller.HaveModal() {
			// Do not focus search widget behind modal dialog
			return
		}
		if s := m.BrowsingPane.GetSearchBarIfAny(); s != nil {
			m.Window.Canvas().Focus(s)
		}
	})
	m.Canvas().AddShortcut(&shortcuts.ShortcutQuickSearch, func(_ fyne.Shortcut) {
		if !m.Controller.HaveModal() {
			m.Controller.ShowQuickSearch()
		}
	})
	m.Canvas().AddShortcut(&fyne.ShortcutSelectAll{}, func(_ fyne.Shortcut) {
		m.Controller.SelectAll()
	})
	m.Canvas().AddShortcut(&shortcuts.ShortcutCloseWindow, func(_ fyne.Shortcut) {
		if runtime.GOOS == "darwin" || (m.App.Config.Application.CloseToSystemTray && m.HaveSystemTray()) {
			m.Window.Hide()
		}
	})

	for i, ns := range shortcuts.NavShortcuts {
		m.Canvas().AddShortcut(&ns, func(i int) func(fyne.Shortcut) {
			return func(fyne.Shortcut) {
				m.Toolbar.ActivateNavigationButton(i)
			}
		}(i))
	}

	m.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		switch e.Name {
		case fyne.KeyUp:
			m.BrowsingPane.ScrollUp()
		case fyne.KeyDown:
			m.BrowsingPane.ScrollDown()
		case fyne.KeyPageUp:
			m.BrowsingPane.PageUp()
		case fyne.KeyPageDown:
			m.BrowsingPane.PageDown()
		case fyne.KeyEscape:
			m.Controller.CloseEscapablePopUp()
		case fyne.KeySpace:
			m.App.PlaybackManager.PlayPause()
		case fyne.KeyLeft:
			m.App.PlaybackManager.SeekBySeconds(-10)
		case fyne.KeyRight:
			m.App.PlaybackManager.SeekBySeconds(10)
		}
	})
	m.Canvas().SetOnMouseBack(m.BrowsingPane.GoBack)
	m.Canvas().SetOnMouseForward(m.BrowsingPane.GoForward)
}

func (m *MainWindow) showSettingsDialog() {
	m.Controller.ShowSettingsDialog(func() {
		fyne.CurrentApp().Settings().SetTheme(m.theme)
		for _, w := range fyne.CurrentApp().Driver().AllWindows() {
			controller.SetWindowThemeMode(w, m.theme.AppearanceMode())
		}
	}, m.theme.ListThemeFiles())
}

func (m *MainWindow) Show() {
	m.Window.Show()
}

func (m *MainWindow) ShowAndRun() {
	m.Window.ShowAndRun()
}

func (m *MainWindow) Canvas() fyne.Canvas {
	return m.Window.Canvas()
}

func (m *MainWindow) SetTitle(title string) {
	m.Window.SetTitle(title)
}

func (m *MainWindow) SetContent(c fyne.CanvasObject) {
	m.Window.SetContent(c)
}

func (m *MainWindow) Quit() {
	m.SaveWindowSettings()
	fyne.CurrentApp().Quit()
}

func (m *MainWindow) SaveWindowSettings() {
	util.SaveWindowSize(m.Window,
		&m.App.Config.Application.WindowWidth,
		&m.App.Config.Application.WindowHeight)
	m.App.Config.Application.ShowSidebar = !m.Sidebar.Hidden
	m.App.Config.Application.SidebarWidthFraction = m.splitContainer.Offset
	switch m.Sidebar.SelectedIndex() {
	case 1:
		m.App.Config.Application.SidebarTab = "Lyrics"
	default:
		m.App.Config.Application.SidebarTab = "Play Queue"
	}
}

// widget just so we can catch a tap event that doesn't land anywhere else
// and call BrowsingPane.UnselectAll()
type mainWindowContent struct {
	widget.BaseWidget

	content  fyne.CanvasObject
	onTapped func()
}

func newMainWindowContent(content fyne.CanvasObject, onTapped func()) *mainWindowContent {
	w := &mainWindowContent{content: content, onTapped: onTapped}
	w.ExtendBaseWidget(w)
	return w
}

func (m *mainWindowContent) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(m.content)
}

func (m *mainWindowContent) Tapped(*fyne.PointEvent) {
	m.onTapped()
}
