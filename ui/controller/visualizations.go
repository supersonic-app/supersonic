package controller

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"github.com/dweymouth/supersonic/backend/player"
	"github.com/dweymouth/supersonic/ui/shortcuts"
	myTheme "github.com/dweymouth/supersonic/ui/theme"
	"github.com/dweymouth/supersonic/ui/util"
	"github.com/dweymouth/supersonic/ui/visualizations"
)

// embedded in parent controller struct
type visualizationData struct {
	peakMeter    *visualizations.PeakMeter
	peakMeterWin fyne.Window

	visualizationAnim *fyne.Animation
}

func (c *Controller) initVisualizations() {
	c.App.PlaybackManager.OnStopped(c.stopVisualizationAnim)
	c.App.PlaybackManager.OnPaused(c.stopVisualizationAnim)
	c.App.PlaybackManager.OnPlaying(func() {
		if _, ok := c.App.PlaybackManager.CurrentPlayer().(player.LocalPlayer); ok {
			if c.peakMeter != nil {
				c.startVisualizationAnim()
			}
		}
	})
}

func (c *Controller) ShowPeakMeter() {
	if c.peakMeterWin != nil {
		c.peakMeterWin.Show()
		return
	}
	c.peakMeterWin = fyne.CurrentApp().NewWindow(lang.L("Peak Meter"))

	onClose := func() {
		c.stopVisualizationAnim()
		c.peakMeter = nil
		util.SaveWindowSize(c.peakMeterWin,
			&c.App.Config.PeakMeter.WindowWidth,
			&c.App.Config.PeakMeter.WindowHeight)
		c.peakMeterWin.Close()
		c.peakMeterWin = nil
	}

	c.peakMeterWin.SetCloseIntercept(onClose)
	c.peakMeterWin.Canvas().AddShortcut(&shortcuts.ShortcutCloseWindow, func(_ fyne.Shortcut) {
		onClose()
	})
	if c.App.Config.PeakMeter.WindowHeight > 0 {
		c.peakMeterWin.Resize(fyne.NewSize(
			float32(c.App.Config.PeakMeter.WindowWidth),
			float32(c.App.Config.PeakMeter.WindowHeight)))
	}
	c.peakMeter = visualizations.NewPeakMeter()
	c.peakMeterWin.SetContent(c.peakMeter)
	if c.App.LocalPlayer.GetStatus().State == player.Playing {
		c.startVisualizationAnim()
	} else {
		// TODO: why is this needed?
		c.peakMeter.Refresh()
	}
	c.peakMeterWin.Show()
	SetWindowThemeMode(c.peakMeterWin, fyne.CurrentApp().Settings().Theme().(*myTheme.MyTheme).AppearanceMode())
}

func (c *Controller) stopVisualizationAnim() {
	if c.visualizationAnim != nil {
		c.visualizationAnim.Stop()
		c.visualizationAnim = nil
		c.App.LocalPlayer.SetPeaksEnabled(false)
	}
}

func (c *Controller) startVisualizationAnim() {
	if c.visualizationAnim == nil {
		c.App.LocalPlayer.SetPeaksEnabled(true)
		c.visualizationAnim = fyne.NewAnimation(
			time.Duration(math.MaxInt64), /*until stopped*/
			c.tickVisualizations)
		c.visualizationAnim.Start()
	}
}

func (c *Controller) tickVisualizations(_ float32) {
	lP, rP, lRMS, rRMS := c.App.LocalPlayer.GetPeaks()
	if c.visualizationData.peakMeter != nil {
		c.visualizationData.peakMeter.UpdatePeaks(lP, rP, lRMS, rRMS)
	}
}
