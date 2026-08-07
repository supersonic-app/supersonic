package main

import (
	"bytes"
	"encoding/json"
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

func addTranslationsForConfiguredLanguage(content []byte, tr res.TranslationInfo) error {
	systemLocale := lang.SystemLocale()
	if err := lang.AddTranslations(fyne.NewStaticResource(systemLocale.LanguageString()+".json", content)); err != nil {
		return err
	}

	locale := strings.TrimSuffix(tr.TranslationFileName, ".json")
	var aliasLocale fyne.Locale
	switch locale {
	case "zh-Hans":
		aliasLocale = "zh-Hant"
	case "zh-Hant":
		aliasLocale = "zh-Hans"
	default:
		return nil
	}

	// Register the full translation under its canonical locale. Also register
	// translated application messages under the other Chinese script locale so
	// an explicit choice overrides the OS script. Identity translations are
	// omitted from the alias to preserve Fyne's built-in translations.
	if err := lang.AddTranslationsForLocale(content, fyne.Locale(locale)); err != nil {
		return err
	}
	aliasContent, err := withoutIdentityTranslations(content)
	if err != nil {
		return err
	}
	return lang.AddTranslationsForLocale(aliasContent, aliasLocale)
}

func withoutIdentityTranslations(content []byte) ([]byte, error) {
	var messages map[string]json.RawMessage
	if err := json.Unmarshal(content, &messages); err != nil {
		return nil, err
	}
	for key, raw := range messages {
		var translation string
		if err := json.Unmarshal(raw, &translation); err == nil && translation == key {
			delete(messages, key)
		}
	}
	return json.Marshal(messages)
}

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
			// Register the configured translation for the system locale so an
			// explicit language choice overrides the OS language.
			if err := addTranslationsForConfiguredLanguage(content, tr); err != nil {
				log.Printf("Error loading configured translation %s: %s\n", tr.TranslationFileName, err.Error())
			} else {
				success = true
			}
		} else {
			log.Printf("Error loading translation file %s: %s\n", tr.TranslationFileName, err.Error())
		}
	}
	if !success {
		if err := lang.AddTranslationsFS(res.Translations, "translations"); err != nil {
			log.Printf("Error loading translations: %s", err.Error())
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
