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

	"github.com/jeandeaual/go-locale"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/windows"
	"github.com/supersonic-app/supersonic/res"
	"github.com/supersonic-app/supersonic/res/wintaskbarthumbs"
	"github.com/supersonic-app/supersonic/ui"
	"github.com/supersonic-app/supersonic/ui/controller"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
	"golang.org/x/term"
	"golang.org/x/text/language"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/lang"
)

func addTranslationsForConfiguredLanguage(content []byte, tr res.TranslationInfo) error {
	// Fyne resolves its active bundle from this complete preference list.
	preferredLocales, err := locale.GetLocales()
	if err != nil || len(preferredLocales) == 0 {
		preferredLocales = []string{lang.SystemLocale().String()}
	}
	locales, err := configuredLanguageRegistrationPlan(tr.TranslationFileName, preferredLocales)
	if err != nil {
		return err
	}
	aliasContent, err := translationAliasContent(content, func(key string) string {
		return lang.L(key)
	})
	if err != nil {
		return err
	}
	for _, locale := range locales {
		if err := lang.AddTranslationsForLocale(aliasContent, locale); err != nil {
			return err
		}
	}
	return nil
}

// configuredLanguageRegistrationPlan covers every tag Fyne can select from a
// preferred system locale: the exact region, base language, or likely script.
func configuredLanguageRegistrationPlan(translationFileName string, preferredLocales []string) ([]fyne.Locale, error) {
	configured, err := language.Parse(strings.TrimSuffix(translationFileName, ".json"))
	if err != nil {
		return nil, err
	}

	locales := make([]fyne.Locale, 0, 1+len(preferredLocales)*3)
	seen := make(map[fyne.Locale]struct{}, cap(locales))
	add := func(tag language.Tag) {
		locale := fyne.Locale(tag.String())
		if tag.IsRoot() {
			return
		}
		if _, exists := seen[locale]; !exists {
			seen[locale] = struct{}{}
			locales = append(locales, locale)
		}
	}
	add(configured)

	for _, preferred := range preferredLocales {
		tag, err := language.Parse(preferred)
		if err != nil {
			continue
		}
		base, confidence := tag.Base()
		if confidence == language.No {
			continue
		}
		_, _, region := tag.Raw()
		if region.String() != "ZZ" {
			add(language.Make(base.String() + "-" + region.String()))
		}
		add(language.Make(base.String()))
		if script, confidence := tag.Script(); confidence != language.No {
			add(language.Make(base.String() + "-" + script.String()))
		}
	}
	return locales, nil
}

// go-i18n resolves one bundle tag for every lookup. Preserve the current Fyne
// value for placeholders, because a new exact alias cannot fall through to a
// Fyne base or script bundle when an individual message is missing.
func translationAliasContent(content []byte, localize func(string) string) ([]byte, error) {
	var messages map[string]json.RawMessage
	if err := json.Unmarshal(content, &messages); err != nil {
		return nil, err
	}
	for key, raw := range messages {
		var translation string
		if err := json.Unmarshal(raw, &translation); err != nil || translation != key {
			continue
		}

		localized := localize(key)
		if localized == key {
			delete(messages, key)
			continue
		}
		localizedRaw, err := json.Marshal(localized)
		if err != nil {
			return nil, err
		}
		messages[key] = localizedRaw
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

	if runtime.GOOS == "linux" {
		ui.SetCursorThemeEnvIfMissing()
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
