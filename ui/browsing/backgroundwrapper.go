package browsing

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// BackgroundWrapper wraps content with a background layer that can display
// blurred or gradient backgrounds based on cover/artist images.
type BackgroundWrapper struct {
	widget.BaseWidget

	bgManager *BackgroundManager
	Content   fyne.CanvasObject
	container *fyne.Container
}

// NewBackgroundWrapper creates a new wrapper with the given content.
func NewBackgroundWrapper(content fyne.CanvasObject) *BackgroundWrapper {
	bw := &BackgroundWrapper{
		Content:   content,
		bgManager: NewBackgroundManager(),
	}
	bw.ExtendBaseWidget(bw)
	return bw
}

// ApplyBackground applies a background image with the given mode.
// Mode can be "blur", "gradient", or "disabled".
func (bw *BackgroundWrapper) ApplyBackground(img image.Image, mode string) {
	if mode == "disabled" || img == nil {
		bw.bgManager.HideImages()
		bw.bgManager.BackgroundGradient.Hide()
	} else {
		bw.bgManager.BackgroundGradient.Show()
		bw.bgManager.ApplyBackground(img, mode)
	}
}

// BgManager returns the internal BackgroundManager for advanced usage.
func (bw *BackgroundWrapper) BgManager() *BackgroundManager {
	return bw.bgManager
}

// Refresh updates the background gradient color when theme changes.
func (bw *BackgroundWrapper) Refresh() {
	if bw.bgManager != nil {
		c := theme.Color(theme.ColorNameBackground)
		if c != bw.bgManager.BackgroundGradient.EndColor {
			bw.bgManager.SetGradientEndColor(c)
		}
	}
	bw.BaseWidget.Refresh()
}

// CreateRenderer creates the renderer for this widget.
func (bw *BackgroundWrapper) CreateRenderer() fyne.WidgetRenderer {
	if bw.container == nil {
		bw.bgManager.SetGradientEndColor(theme.Color(theme.ColorNameBackground))
		bw.container = container.NewStack(
			bw.bgManager.BackgroundImgA,
			bw.bgManager.BackgroundImgB,
			bw.bgManager.BackgroundGradient,
			bw.Content,
		)
	}
	return widget.NewSimpleRenderer(bw.container)
}
