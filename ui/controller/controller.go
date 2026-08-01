package controller

import (
	"archive/zip"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	fynetooltip "github.com/dweymouth/fyne-tooltip"
	"github.com/dweymouth/supersonic/backend"
	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/backend/player"
	"github.com/dweymouth/supersonic/backend/player/mpv"
	"github.com/dweymouth/supersonic/sharedutil"
	"github.com/dweymouth/supersonic/ui/dialogs"
	myTheme "github.com/dweymouth/supersonic/ui/theme"
	"github.com/dweymouth/supersonic/ui/util"
	"github.com/dweymouth/supersonic/ui/widgets"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type NavigationHandler func(Route)

type CurPageFunc func() Route

type ToastProvider interface {
	ShowSuccessToast(string)
	ShowErrorToast(string)
}

type Controller struct {
	visualizationData
	AppVersion string
	App        *backend.App
	MainWindow fyne.Window

	// dependencies injected from MainWindow
	NavHandler          NavigationHandler
	CurPageFunc         CurPageFunc
	ReloadFunc          func()
	RefreshPageFunc     func()
	SelectAllPageFunc   func()
	UnselectAllPageFunc func()
	ToastProvider       ToastProvider

	popUpQueue         *widget.PopUp
	popUpQueueList     *widgets.PlayQueueList
	pauseAfterCurrent  *widget.Check
	hidePlayedTracks   *widget.Check
	popUpQueueLastUsed int64
	escapablePopUp     fyne.CanvasObject
	haveModal          bool
	runOnModalClosed   func()
}

func New(app *backend.App, appVersion string, mainWindow fyne.Window) *Controller {
	c := &Controller{
		AppVersion: appVersion,
		MainWindow: mainWindow,
		App:        app,
	}
	c.initVisualizations()
	c.App.PlaybackManager.OnQueueChange(util.FyneDoFunc(func() {
		if c.popUpQueue != nil {
			c.applyPopUpQueueItems()
		}
	}))
	c.App.PlaybackManager.OnSongChange(func(track mediaprovider.MediaItem, _ *mediaprovider.Track) {
		fyne.Do(func() {
			if c.popUpQueue == nil {
				return
			}
			c.applyPopUpQueueItems()
			if track == nil {
				c.popUpQueueList.SetNowPlaying("")
			} else {
				c.popUpQueueList.SetNowPlaying(track.Metadata().ID)
			}
		})
	})
	return c
}

// applyPopUpQueueItems applies the hide-played filter to the popup queue.
func (m *Controller) applyPopUpQueueItems() {
	items := m.App.PlaybackManager.GetActivePlayQueue()
	nowIdx := m.App.PlaybackManager.NowPlayingIndex()
	displayItems, offset := util.FilterQueueForHidePlayed(items, nowIdx, m.App.Config.Application.HidePlayedQueueTracks)
	m.popUpQueueList.SetPlayIndexOffset(offset)
	m.popUpQueueList.SetItems(displayItems)
}

func (m *Controller) SelectAll() {
	if m.popUpQueue != nil && m.popUpQueue.Visible() {
		m.popUpQueueList.SelectAll()
		return
	}
	if m.SelectAllPageFunc != nil {
		m.SelectAllPageFunc()
	}
}

func (m *Controller) UnselectAll() {
	if m.popUpQueue != nil && m.popUpQueue.Visible() {
		m.popUpQueueList.UnselectAll()
		return
	}
	if m.SelectAllPageFunc != nil {
		m.UnselectAllPageFunc()
	}
}

func (m *Controller) NavigateTo(route Route) {
	m.NavHandler(route)
}

func (m *Controller) ClosePopUpOnEscape(pop fyne.CanvasObject) {
	m.escapablePopUp = pop
}

func (m *Controller) CloseEscapablePopUp() {
	if m.escapablePopUp != nil {
		m.escapablePopUp.Hide()
		m.escapablePopUp = nil
		m.doModalClosed()
	}
}

// If there is currently no modal popup managed by the Controller visible,
// then run f (which should create and show a modal dialog) immediately.
// else run f when the current modal dialog workflow has ended.
func (m *Controller) QueueShowModalFunc(f func()) {
	if m.haveModal {
		m.runOnModalClosed = f
	} else {
		f()
	}
}

func (m *Controller) HaveModal() bool {
	return m.haveModal
}

func (m *Controller) ShowCastMenu(onPendingPlayerChange func()) {
	rp := m.App.PlaybackManager.CurrentRemotePlayer()
	devices := m.App.PlaybackManager.RemotePlayers()
	local := fyne.NewMenuItem(lang.L("This computer"), func() {
		if rp == nil {
			return // no-op. already not casting
		}
		onPendingPlayerChange()
		go func() {
			if err := m.App.PlaybackManager.SetRemotePlayer(nil); err != nil {
				fyne.Do(func() { m.ToastProvider.ShowErrorToast("Failed to disconnect from remote player") })
			}
		}()
	})
	local.Icon = theme.ComputerIcon()
	local.Checked = rp == nil

	menu := fyne.NewMenu("", local)
	for _, d := range devices {
		_d := d
		isCurrent := rp != nil && _d.URL == rp.URL
		item := fyne.NewMenuItem(d.Name, func() {
			if isCurrent {
				return // no-op.
			}
			onPendingPlayerChange()
			go func() {
				if err := m.App.PlaybackManager.SetRemotePlayer(&_d); err != nil {
					fyne.Do(func() { m.ToastProvider.ShowErrorToast("Failed to connect to " + _d.Name) })
				}
			}()
		})
		item.Icon = myTheme.CastIcon
		item.Checked = isCurrent
		menu.Items = append(menu.Items, item)
	}
	pop := widget.NewPopUpMenu(menu, m.MainWindow.Canvas())
	canvasSize := m.MainWindow.Canvas().Size()
	pop.ShowAtPosition(fyne.NewPos(
		canvasSize.Width-pop.MinSize().Width-10,
		canvasSize.Height-pop.MinSize().Height-100,
	))
}

func (m *Controller) ShowPopUpPlayQueue() {
	if m.popUpQueue == nil {
		m.popUpQueueList = widgets.NewPlayQueueList(m.App.ImageManager, false)
		m.popUpQueueList.Reorderable = true
		m.applyPopUpQueueItems()
		m.ConnectPlayQueuelistActions(m.popUpQueueList)

		title := widget.NewRichTextWithText(lang.L("Play Queue"))
		title.Segments[0].(*widget.TextSegment).Style.Alignment = fyne.TextAlignCenter
		title.Segments[0].(*widget.TextSegment).Style.TextStyle.Bold = true
		m.pauseAfterCurrent = widget.NewCheck(lang.L("Pause after current track"), func(b bool) {
			m.App.PlaybackManager.SetPauseAfterCurrent(b)
		})
		m.hidePlayedTracks = widget.NewCheck(lang.L("Hide played tracks"), func(b bool) {
			m.App.Config.Application.HidePlayedQueueTracks = b
			m.applyPopUpQueueItems()
			m.App.PlaybackManager.TriggerQueueChangeCallback()
		})
		bottomRow := container.NewHBox(layout.NewSpacer(), m.hidePlayedTracks, m.pauseAfterCurrent)
		ctr := container.NewBorder(title, bottomRow, nil, nil,
			container.NewPadded(m.popUpQueueList),
		)
		m.popUpQueue = widget.NewPopUp(ctr, m.MainWindow.Canvas())
		fynetooltip.AddPopUpToolTipLayer(m.popUpQueue)

		container.NewThemeOverride(m.popUpQueue, myTheme.WithColorTransformOverride(
			theme.ColorNameOverlayBackground,
			func(c color.Color) color.Color {
				if nrgba, ok := c.(color.NRGBA); ok {
					nrgba.A = 245
					return nrgba
				}
				c_ := c.(color.RGBA)
				c_.A = 245
				return c_
			},
		))

		// free popUpQueue if it hasn't been used in awhile
		go func() {
			t := time.NewTicker(1 * time.Minute)
			for range t.C {
				fyne.Do(func() {
					now := time.Now().UnixMilli()
					if m.popUpQueueLastUsed < now-120_000 /*2 min*/ {
						fynetooltip.DestroyPopUpToolTipLayer(m.popUpQueue)
						m.popUpQueue = nil
						m.popUpQueueList = nil
						m.pauseAfterCurrent = nil
						m.hidePlayedTracks = nil
						m.popUpQueueLastUsed = 0
						t.Stop()
						return
					}
				})
			}
		}()
	}
	m.popUpQueueLastUsed = time.Now().UnixMilli()
	popUpQueueList := m.popUpQueueList
	pop := m.popUpQueue

	// Re-apply filter in case setting changed since last open
	m.applyPopUpQueueItems()

	npID := ""
	if np := m.App.PlaybackManager.NowPlaying(); np != nil {
		npID = np.Metadata().ID
	}
	popUpQueueList.SetNowPlaying(npID)
	popUpQueueList.UnselectAll()

	m.ClosePopUpOnEscape(pop)
	minSize := fyne.NewSize(300, 400)
	maxSize := fyne.NewSize(800, 1000)
	canvasSize := m.MainWindow.Canvas().Size()
	size := minSize.Max(maxSize.Min(
		fyne.NewSize(canvasSize.Width*0.4, canvasSize.Height*0.5),
	))
	pop.Resize(size)
	popUpQueueList.ScrollToNowPlaying() // must come after resize
	m.pauseAfterCurrent.SetChecked(m.App.PlaybackManager.IsPauseAfterCurrent())
	m.hidePlayedTracks.SetChecked(m.App.Config.Application.HidePlayedQueueTracks)
	pop.ShowAtPosition(fyne.NewPos(
		canvasSize.Width-size.Width-10,
		canvasSize.Height-size.Height-100,
	))
}

func (m *Controller) ShowPopUpImage(img image.Image) {
	s := m.MainWindow.Canvas().Size()
	var popS fyne.Size
	if asp := util.ImageAspect(img); s.Width/s.Height > asp {
		// window height is limiting factor
		h := s.Height * 0.8
		popS = fyne.NewSize(h*asp, h)
	} else {
		w := s.Width * 0.8
		popS = fyne.NewSize(w, w*(1/asp))
	}

	pop := widgets.NewImagePopUp(img, m.MainWindow.Canvas(), popS)

	m.ClosePopUpOnEscape(pop)
	pop.Show()
}

func (m *Controller) GetArtistTracks(artistID string) []*mediaprovider.Track {
	if mp := m.App.ServerManager.Server; mp != nil {
		if tr, err := mp.GetArtistTracks(artistID); err != nil {
			log.Println(err.Error())
			return nil
		} else {
			return tr
		}
	}
	return nil
}

func (c *Controller) ShowAboutDialog() {
	dlg := dialogs.NewAboutDialog(c.AppVersion)
	pop := widget.NewModalPopUp(dlg, c.MainWindow.Canvas())
	dlg.OnDismiss = func() {
		pop.Hide()
		c.doModalClosed()
	}
	c.ClosePopUpOnEscape(pop)
	c.haveModal = true
	pop.Show()
}

func (c *Controller) ShowSettingsDialog(themeUpdateCallbk func(), themeFiles map[string]string) {
	devs, err := c.App.LocalPlayer.ListAudioDevices()
	if err != nil {
		log.Printf("error listing audio devices: %v", err)
		devs = []mpv.AudioDevice{{Name: "auto", Description: lang.L("Autoselect device")}}
	}

	curPlayer := c.App.PlaybackManager.CurrentPlayer()
	_, isReplayGainPlayer := curPlayer.(player.ReplayGainPlayer)
	_, isEqualizerPlayer := curPlayer.(*mpv.Player)
	_, canSavePlayQueue := c.App.ServerManager.Server.(mediaprovider.CanSavePlayQueue)
	isLocalPlayer := isEqualizerPlayer
	bands := c.App.LocalPlayer.Equalizer().BandFrequencies()

	dlg := dialogs.NewSettingsDialog(c.App.Config,
		devs, themeFiles, bands,
		c.App.ServerManager.Server.ClientDecidesScrobble(),
		isLocalPlayer, isReplayGainPlayer, isEqualizerPlayer, canSavePlayQueue,
		c.App.EQPresetManager,
		c.MainWindow,
		c.App.AutoEQManager,
		c.App.ImageManager,
		c.ToastProvider)
	dlg.OnReplayGainSettingsChanged = func() {
		c.App.PlaybackManager.SetReplayGainOptions(c.App.Config.ReplayGain)
	}
	dlg.OnAudioExclusiveSettingChanged = func() {
		c.App.LocalPlayer.SetAudioExclusive(c.App.Config.LocalPlayback.AudioExclusive)
	}
	dlg.OnPauseFadeSettingsChanged = func() {
		c.App.LocalPlayer.SetPauseFade(c.App.Config.LocalPlayback.PauseFade)
	}
	dlg.OnAudioDeviceSettingChanged = func() {
		c.App.LocalPlayer.SetAudioDevice(c.App.Config.LocalPlayback.AudioDeviceName)
	}
	dlg.OnThemeSettingChanged = themeUpdateCallbk
	dlg.OnEqualizerSettingsChanged = func() {
		// Create the appropriate equalizer type based on config
		var eq mpv.Equalizer
		if c.App.Config.LocalPlayback.EqualizerType == "ISO10Band" {
			eq10 := &mpv.ISO10BandEqualizer{
				Disabled: !c.App.Config.LocalPlayback.EqualizerEnabled,
				EQPreamp: c.App.Config.LocalPlayback.EqualizerPreamp,
			}
			// Copy up to 10 bands
			numBands := min(len(c.App.Config.LocalPlayback.GraphicEqualizerBands), 10)
			for i := 0; i < numBands; i++ {
				eq10.BandGains[i] = c.App.Config.LocalPlayback.GraphicEqualizerBands[i]
			}
			eq = eq10
		} else {
			eq15 := &mpv.ISO15BandEqualizer{
				Disabled: !c.App.Config.LocalPlayback.EqualizerEnabled,
				EQPreamp: c.App.Config.LocalPlayback.EqualizerPreamp,
			}
			// Copy up to 15 bands
			numBands := min(len(c.App.Config.LocalPlayback.GraphicEqualizerBands), 15)
			for i := 0; i < numBands; i++ {
				eq15.BandGains[i] = c.App.Config.LocalPlayback.GraphicEqualizerBands[i]
			}
			eq = eq15
		}
		c.App.LocalPlayer.SetEqualizer(eq)
	}
	dlg.OnPageNeedsRefresh = c.RefreshPageFunc
	dlg.OnClearCaches = func() { go c.App.ClearCaches() }
	dlg.OnHidePlayedQueueTracksChanged = func() {
		if c.popUpQueue != nil {
			c.applyPopUpQueueItems()
		}
		c.App.PlaybackManager.TriggerQueueChangeCallback()
	}
	pop := widget.NewModalPopUp(dlg, c.MainWindow.Canvas())
	fynetooltip.AddPopUpToolTipLayer(pop)
	dlg.OnDismiss = func() {
		pop.Hide()
		fynetooltip.DestroyPopUpToolTipLayer(pop)
		c.doModalClosed()
		c.App.SaveConfigFile()
	}
	c.ClosePopUpOnEscape(pop)
	c.haveModal = true
	pop.Show()
}

func (c *Controller) doModalClosed() {
	c.haveModal = false
	if c.runOnModalClosed != nil {
		c.runOnModalClosed()
		c.runOnModalClosed = nil
	}
}

func (c *Controller) SetTrackFavorites(trackIDs []string, favorite bool) {
	go c.App.ServerManager.Server.SetFavorite(mediaprovider.RatingFavoriteParameters{
		TrackIDs: trackIDs,
	}, favorite)

	for _, id := range trackIDs {
		c.App.PlaybackManager.OnTrackFavoriteStatusChanged(id, favorite)
	}
}

func (c *Controller) SetTrackRatings(trackIDs []string, rating int) {
	r, ok := c.App.ServerManager.Server.(mediaprovider.SupportsRating)
	if !ok {
		return
	}
	go r.SetRating(mediaprovider.RatingFavoriteParameters{
		TrackIDs: trackIDs,
	}, rating)

	// Notify PlaybackManager of rating change to update
	// the in-memory track models
	for _, id := range trackIDs {
		c.App.PlaybackManager.OnTrackRatingChanged(id, rating)
	}
}

func (c *Controller) ShowShareDialog(id string) {
	sh, ok := c.App.ServerManager.Server.(mediaprovider.SupportsSharing)
	if !ok {
		// this should not happen since interface checks are done
		// before showing share menu items, but just in case:
		c.ToastProvider.ShowErrorToast(lang.L("server does not support sharing"))
		return
	}
	go func() {
		shareUrl, err := sh.CreateShareURL(id)
		if err != nil {
			log.Printf("error creating share URL: %v", err)
			fyne.Do(func() {
				c.showError(
					"Failed to share content. This commonly occurs when the server does not support sharing, " +
						"or has the feature disabled.\nPlease check the server's settings and try again.",
				)
			})
			return
		}
		fyne.Do(func() {
			hyperlink := widget.NewHyperlink(shareUrl.String(), shareUrl)
			dismissed := false
			dlg := dialog.NewCustom(lang.L("Share content"), lang.L("OK"),
				container.NewHBox(
					hyperlink,
					widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
						fyne.CurrentApp().Clipboard().SetContent(hyperlink.Text)
					}),
					widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
						go func() {
							shareUrl, err := sh.CreateShareURL(id)
							if err != nil {
								log.Printf("error creating share URL: %v", err)
								return
							}
							fyne.Do(func() {
								if !dismissed {
									hyperlink.Text = shareUrl.String()
									hyperlink.URL = shareUrl
									hyperlink.Refresh()
								}
							})
						}()
					}),
				),
				c.MainWindow,
			)
			dlg.SetOnClosed(func() {
				dismissed = true
			})
			dlg.Show()
		})
	}()
}

func (c *Controller) ShowDownloadDialog(tracks []*mediaprovider.Track, downloadName string) {
	numTracks := len(tracks)
	var fileName string
	if numTracks == 1 {
		fileName = filepath.Base(tracks[0].FilePath)
	} else {
		fileName = "downloaded_tracks.zip"
	}

	dg := dialog.NewFileSave(
		func(file fyne.URIWriteCloser, err error) {
			if err != nil {
				log.Println(err)
				return
			}

			if file == nil {
				return
			}
			if numTracks == 1 {
				go c.downloadTrack(tracks[0], file.URI().Path())
			} else {
				go c.downloadTracks(tracks, file.URI().Path(), downloadName)
			}
		},
		c.MainWindow)
	dg.SetFileName(fileName)
	dg.Show()
}

func (c *Controller) downloadTrack(track *mediaprovider.Track, filePath string) {
	reader, err := c.App.ServerManager.Server.DownloadTrack(track.ID)
	if err != nil {
		log.Println(err)
		return
	}

	file, err := os.Create(filePath)
	if err != nil {
		log.Println(err)
		return
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("Saved track %q to: %s\n", track.Title, filePath)
	fyne.Do(func() {
		c.sendNotification(fmt.Sprintf(lang.L("Download completed")+": %s", track.Title), fmt.Sprintf(lang.L("Saved at")+": %s", filePath))
	})
}

func (c *Controller) downloadTracks(tracks []*mediaprovider.Track, filePath, downloadName string) {
	zipFile, err := os.Create(filePath)
	if err != nil {
		log.Println(err)
		return
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, track := range tracks {
		reader, err := c.App.ServerManager.Server.DownloadTrack(track.ID)
		if err != nil {
			log.Println(err)
			continue
		}

		fileName := filepath.Base(track.FilePath)

		fileWriter, err := zipWriter.Create(fileName)
		if err != nil {
			log.Println(err)
			continue
		}

		_, err = io.Copy(fileWriter, reader)
		if err != nil {
			log.Println(err)
			continue
		}

		log.Printf("Saved song %s to: %s\n", track.Title, filePath)
	}

	fyne.Do(func() {
		c.sendNotification(fmt.Sprintf(lang.L("Download completed")+": %s", downloadName), fmt.Sprintf("Saved at: %s", filePath))
	})
}

func (c *Controller) sendNotification(title, content string) {
	fyne.CurrentApp().SendNotification(&fyne.Notification{
		Title:   title,
		Content: content,
	})
}

func (c *Controller) showError(content string) {
	// TODO: display an in-app toast message instead of a dialog.
	dialog.ShowError(errors.New(content), c.MainWindow)
}

func (c *Controller) ShowAlbumInfoDialog(albumID, albumName string, albumCover image.Image) {
	go func() {
		albumInfo, err := c.App.ServerManager.Server.GetAlbumInfo(albumID)
		if err != nil {
			log.Print("Error getting album info: ", err)
			return
		}
		fyne.Do(func() {
			dlg := dialogs.NewAlbumInfoDialog(albumInfo, albumName, albumCover)
			pop := widget.NewModalPopUp(dlg, c.MainWindow.Canvas())
			dlg.OnDismiss = func() {
				pop.Hide()
				c.doModalClosed()
			}
			c.ClosePopUpOnEscape(pop)
			c.haveModal = true
			pop.Resize(pop.Content.MinSize()) // needed so that NonScrollingMinHeight can consider the width
			h := fyne.Min(dlg.NonScrollingMinHeight(), c.MainWindow.Canvas().Size().Height*0.85)
			pop.Resize(fyne.NewSize(dlg.MinSize().Width, h))
			pop.Show()
		})
	}()
}

func (c *Controller) ShowTrackInfoDialog(track *mediaprovider.Track) {
	info := dialogs.NewTrackInfoDialog(track)
	pop := widget.NewModalPopUp(info, c.MainWindow.Canvas())
	info.OnDismiss = func() {
		pop.Hide()
		c.doModalClosed()
	}
	info.OnNavigateToAlbum = func(albumID string) {
		info.OnDismiss()
		c.NavigateTo(AlbumRoute(albumID))
	}
	info.OnNavigateToArtist = func(artistID string) {
		info.OnDismiss()
		c.NavigateTo(ArtistRoute(artistID))
	}
	info.OnNavigateToGenre = func(genre string) {
		info.OnDismiss()
		c.NavigateTo(GenreRoute(genre))
	}
	info.OnCopyFilePath = func() {
		c.MainWindow.Clipboard().SetContent(track.FilePath)
	}
	c.ClosePopUpOnEscape(pop)
	winSize := c.MainWindow.Canvas().Size()
	popMin := pop.MinSize()
	width := fyne.Min(750, fyne.Max(popMin.Width, winSize.Width*0.8))
	height := fyne.Min(650, fyne.Max(popMin.Height, winSize.Height*0.8))
	pop.Resize(fyne.NewSize(width, height))
	c.haveModal = true
	pop.Show()
}

func (c *Controller) GetSongRadioTracks(sourceTrack *mediaprovider.Track) ([]*mediaprovider.Track, error) {
	radioTracks, err := c.App.ServerManager.Server.GetSongRadio(sourceTrack.ID, 100)
	if err != nil {
		return nil, fmt.Errorf("error getting song radio: %s", err.Error())
	}

	// The goal of this implementation is to place the source track first in the queue.
	filteredTracks := sharedutil.FilterSlice(radioTracks, func(track *mediaprovider.Track) bool {
		return track.ID != sourceTrack.ID
	})
	tracks := []*mediaprovider.Track{sourceTrack}
	tracks = append(tracks, filteredTracks...)
	return tracks, nil
}

func SetWindowThemeMode(win fyne.Window, mode myTheme.AppearanceMode) {
	arg := 0
	switch fyne.CurrentApp().Settings().Theme().(*myTheme.MyTheme).AppearanceMode() {
	case myTheme.AppearanceDark:
		arg = 1
	case myTheme.AppearanceLight:
		arg = 2
	}
	win.(driver.NativeWindow).RunNative(func(ctx any) {
		switch runtime.GOOS {
		case "darwin":
			// ensure dark mode setting is applied to window controls in title bar on Mac
			setWindowDarkTheme(ctx.(driver.MacWindowContext).NSWindow, arg)
		case "windows":
			setWindowDarkTheme(ctx.(driver.WindowsWindowContext).HWND, arg)
		}
	})
}
