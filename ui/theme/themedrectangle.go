package theme

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

const translucentAlpha = 155

type ThemedRectangle struct {
	widget.BaseWidget

	rect *canvas.Rectangle

	ColorName       fyne.ThemeColorName
	Translucent     bool
	BorderWidth     float32
	BorderColorName fyne.ThemeColorName
	CornerRadius    float32
}

func NewThemedRectangle(colorName fyne.ThemeColorName) *ThemedRectangle {
	t := &ThemedRectangle{
		ColorName: colorName,
		rect: canvas.NewRectangle(fyne.CurrentApp().Settings().Theme().Color(colorName,
			fyne.CurrentApp().Settings().ThemeVariant())),
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *ThemedRectangle) Refresh() {
	settings := fyne.CurrentApp().Settings()
	th := settings.Theme()
	// Handle nil theme during theme transitions
	if th == nil {
		return
	}
	fc := th.Color(t.ColorName, settings.ThemeVariant())
	if t.Translucent {
		r, g, b, _ := fc.RGBA()
		fc = color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: translucentAlpha}
	}
	t.rect.FillColor = fc
	t.rect.StrokeWidth = t.BorderWidth
	t.rect.StrokeColor = th.Color(t.BorderColorName, settings.ThemeVariant())
	t.rect.CornerRadius = t.CornerRadius
	t.BaseWidget.Refresh()
}

func (t *ThemedRectangle) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.rect)
}
