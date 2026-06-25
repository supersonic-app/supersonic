package main

import (
	"bytes"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/windows"
	"github.com/supersonic-app/supersonic/res"
	"github.com/supersonic-app/supersonic/res/wintaskbarthumbs"
	"github.com/supersonic-app/supersonic/ui"
	"github.com/supersonic-app/supersonic/ui/controller"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
	"golang.org/x/term"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/lang"
)

func main() {
	// parse cmd line flags - see backend/cmdlineoptions.go
	flag.Parse()
	if *backend.FlagVersion {
		fmt.Println(res.AppVersion)
		return
	}
	if *backend.FlagHelp {
		flag.Usage()
		return
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		if *backend.FlagPlayAlbum {
			fmt.Scanln(&backend.PlayAlbumCLIArg)
		} else if *backend.FlagPlayPlaylist {
			fmt.Scanln(&backend.PlayPlaylistCLIArg)
		} else if *backend.FlagPlayTrack {
			fmt.Scanln(&backend.PlayTrackCLIArg)
		}
	}
	// rest of flag actions are handled in backend.StartupApp

	myApp, err := backend.StartupApp(res.AppName, res.DisplayName, res.AppVersion, res.AppVersionTag, res.LatestReleaseURL)
	if err != nil {
		if err != backend.ErrAnotherInstance {
			log.Fatalf("fatal startup error: %v", err.Error())
		}
		return
	}

	if myApp.Config.Application.UIScaleSize == "Smaller" {
		os.Setenv("FYNE_SCALE", "0.85")
	} else if myApp.Config.Application.UIScaleSize == "Larger" {
		os.Setenv("FYNE_SCALE", "1.1")
	}

	if myApp.Config.Application.DisableDPIDetection {
		os.Setenv("FYNE_DISABLE_DPI_DETECTION", "true")
	}

	// load configured app language, or all otherwise
	lIdx := slices.IndexFunc(res.TranslationsInfo, func(t res.TranslationInfo) bool {
		return t.Name == myApp.Config.Application.Language
	})

	success := false
	if lIdx >= 0 {
		tr := res.TranslationsInfo[lIdx]
		content, err := res.Translations.ReadFile("translations/" + tr.TranslationFileName)
		if err == nil {
			sysLocale := lang.SystemLocale().LanguageString()
			_ = lang.AddTranslationsForLocale(content, fyne.Locale(sysLocale))
			success = true

			// Also register under the explicit BCP47 locale so go-i18n's matcher
			// correctly resolves queries (e.g. "zh" matches "zh-Hans" for Chinese)
			if tr.BCP47Locale != "" && tr.BCP47Locale != sysLocale {
				_ = lang.AddTranslationsForLocale(content, fyne.Locale(tr.BCP47Locale))
			}

			// Fyne's localeFromTag produces "language-region-script" (e.g. "zh-CN-Hans")
			// or "language-region" (e.g. "zh-CN"). LanguageString strips script info,
			// but Fyne's built-in uses "language-script" tags (e.g. "zh-Hans" from
			// base.zh_Hans.json). The localizer may match the built-in script tag, so
			// we also register under any known script-variant locale for the system's
			// base language so our content merges with Fyne's built-in translations.
			raw := lang.SystemLocale().String()
			baseLang := raw
			if idx := strings.IndexByte(raw, '-'); idx >= 0 {
				baseLang = raw[:idx]
			}
			for _, ti := range res.TranslationsInfo {
				if ti.BCP47Locale == "" || ti.BCP47Locale == tr.BCP47Locale {
					continue
				}
				bcpparts := strings.Split(ti.BCP47Locale, "-")
				if len(bcpparts) >= 2 && bcpparts[0] == baseLang {
					script := bcpparts[1]
					// BCP47 script codes are 4-letter with uppercase first letter
					if len(script) == 4 && script[0] >= 'A' && script[0] <= 'Z' && ti.BCP47Locale != sysLocale {
						_ = lang.AddTranslationsForLocale(content, fyne.Locale(ti.BCP47Locale))
					}
				}
			}

		} else {
			log.Printf("Error loading translation file %s: %s\n", tr.TranslationFileName, err.Error())
		}
	}
	if !success {
		if err := lang.AddTranslationsFS(res.Translations, "translations"); err != nil {
			log.Printf("Error loading translations: %s", err.Error())
		}
		// Some translation filenames are not valid BCP47 tags
		// (e.g. "zhHans.json" → locale "zhHans" → language.Make returns "und").
		// Re-register them under their canonical BCP47 locale so Fyne's i18n
		// matcher can find them.
		for _, tr := range res.TranslationsInfo {
			if tr.BCP47Locale != "" && tr.BCP47Locale != tr.Name {
				content, err := res.Translations.ReadFile("translations/" + tr.TranslationFileName)
				if err == nil {
					log.Println("[i18n] re-registering", tr.Name, "as", tr.BCP47Locale)
					_ = lang.AddTranslationsForLocale(content, fyne.Locale(tr.BCP47Locale))
				}
			}
		}
	}

	if runtime.GOOS == "windows" {
		if err := initWindowsTaskbarIcons(); err != nil {
			log.Printf("Error initializing taskbar thumbnail icons: %s", err.Error())
		}
		if err := windows.SetTaskbarButtonToolTips(
			lang.L("Previous"),
			lang.L("Next"),
			lang.L("Play"),
			lang.L("Pause"),
		); err != nil {
			log.Printf("error initializing taskbar button tool tips: %s", err.Error())
		}
	}

	fyneApp := app.NewWithID(res.AppID)
	fyneApp.SetIcon(res.ResAppicon256Png)

	mainWindow := ui.NewMainWindow(fyneApp, res.AppName, res.DisplayName, res.AppVersion, myApp)
	mainWindow.Window.SetMaster()
	myApp.OnReactivate = util.FyneDoFunc(mainWindow.Show)
	myApp.OnExit = util.FyneDoFunc(mainWindow.Quit)
	myApp.OnReloadTheme = util.FyneDoFunc(mainWindow.ReloadTheme)

	if runtime.GOOS == "windows" {
		windowStartupTasks := sync.OnceFunc(func() {
			mainWindow.Window.(driver.NativeWindow).RunNative(func(ctx any) {
				hwnd := ctx.(driver.WindowsWindowContext).HWND
				if myApp.Config.Application.EnableOSMediaPlayerAPIs {
					myApp.SetupWindowsSMTC(hwnd)
				}
				myApp.SetupWindowsTaskbarButtons(hwnd)
			})
		})
		fyneApp.Lifecycle().SetOnEnteredForeground(windowStartupTasks)
	}

	if *backend.FlagStartMinimized {
		if err = myApp.LoginToDefaultServer(); err != nil {
			log.Fatalf("failed to connect to server: %v", err.Error())
			return
		}
		fyneApp.Run()
	} else {
		fyneApp.Lifecycle().SetOnStarted(func() {
			if mode := fyne.CurrentApp().Settings().Theme().(*myTheme.MyTheme).AppearanceMode(); mode != myTheme.AppearanceAuto {
				controller.SetWindowThemeMode(mainWindow.Window, mode)
			}
			defaultServer := myApp.ServerManager.GetDefaultServer()
			if defaultServer == nil {
				mainWindow.Controller.PromptForFirstServer()
			} else if !*backend.FlagStartMinimized { // If the minimized start flag was passed, the connection is already established.
				mainWindow.Controller.DoConnectToServerWorkflow(defaultServer)
			}
		})

		mainWindow.ShowAndRun()
	}

	log.Println("Running shutdown tasks...")
	myApp.Shutdown()
}

func initWindowsTaskbarIcons() error {
	play, err := png.Decode(bytes.NewReader(wintaskbarthumbs.MediaPlayPNG))
	if err != nil {
		return err
	}
	pause, err := png.Decode(bytes.NewReader(wintaskbarthumbs.MediaPausePNG))
	if err != nil {
		return err
	}
	prev, err := png.Decode(bytes.NewReader(wintaskbarthumbs.MediaSeekPreviousPNG))
	if err != nil {
		return err
	}
	next, err := png.Decode(bytes.NewReader(wintaskbarthumbs.MediaSeekNextPNG))
	if err != nil {
		return err
	}

	windows.InitializeTaskbarIcons(prev, next, play, pause)

	return nil
}
