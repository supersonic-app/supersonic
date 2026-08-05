package widgets

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/supersonic-app/supersonic/backend"
)

type WaveformSeekbar struct {
	widget.DisableableWidget

	OnSeeked func(float64)

	imgColorL        color.Color
	imgColorR        color.Color
	imgProgressPixel int

	focused bool

	img    *canvas.Image
	cursor *canvas.Rectangle
	focus  *canvas.Rectangle
}

func NewWaveformSeekbar() *WaveformSeekbar {
	w := &WaveformSeekbar{
		img: &canvas.Image{
			ScaleMode: canvas.ImageScaleFastest,
			Image:     backend.NewWaveformImage(),
		},
		cursor: canvas.NewRectangle(color.Transparent),
		focus:  canvas.NewRectangle(color.Transparent),
	}
	w.ExtendBaseWidget(w)
	w.cursor.Hidden = true
	w.focus.Hidden = true
	return w
}

func (w *WaveformSeekbar) UpdateImage(img *backend.WaveformImage) {
	prm, fg, _ := w.getThemeColors()
	recolorWaveformImage(img, prm, fg, 0, w.imgProgressPixel, true)
	w.img.Image = img
	w.img.Refresh()
}

func (w *WaveformSeekbar) Refresh() {
	w.cursor.Resize(fyne.NewSize(1, w.Size().Height-4))
	w.focus.Resize(fyne.NewSize(3, w.Size().Height-2))
	prm, fg, focus := w.getThemeColors()
	img := w.img.Image.(*image.NRGBA)
	recolorWaveformImage(img, prm, fg, w.imgProgressPixel, w.imgProgressPixel, true)
	w.recolorCursor(prm, fg, w.cursor.Position().X)
	w.focus.FillColor = focus

	w.BaseWidget.Refresh()
}

var _ desktop.Hoverable = (*WaveformSeekbar)(nil)

func (w *WaveformSeekbar) MouseIn(e *desktop.MouseEvent) {
	if w.Disabled() {
		return
	}
	prm, fg, _ := w.getThemeColors()
	w.recolorCursor(prm, fg, e.Position.X)
	w.cursor.Resize(fyne.NewSize(1, w.Size().Height-4))
	w.cursor.Move(fyne.NewPos(e.Position.X, 2))
	w.cursor.Show()
}

func (w *WaveformSeekbar) MouseMoved(e *desktop.MouseEvent) {
	if w.Disabled() {
		return
	}
	prm, fg, _ := w.getThemeColors()
	w.recolorCursor(prm, fg, e.Position.X)
	w.cursor.Move(fyne.NewPos(e.Position.X, 2))
}

func (w *WaveformSeekbar) MouseOut() {
	if !w.focused {
		w.cursor.Hide()
	}
}

var _ fyne.Focusable = (*WaveformSeekbar)(nil)

func (w *WaveformSeekbar) FocusGained() {
	w.focused = true
	prm, fg, _ := w.getThemeColors()
	w.recolorCursor(prm, fg, w.cursor.Position().X)
	w.cursor.Resize(fyne.NewSize(1, w.Size().Height-4))
	w.moveCursorAndFocusToCurrentPosition()
	w.cursor.Show()
	w.focus.Show()
}

func (w *WaveformSeekbar) FocusLost() {
	w.focused = false
	w.cursor.Hide()
	w.focus.Hide()
}

func (w *WaveformSeekbar) TypedKey(e *fyne.KeyEvent) {
	progress := float32(w.imgProgressPixel) / 1024
	switch e.Name {
	case fyne.KeyLeft:
		progress = max(progress-0.05, 0)
	case fyne.KeyRight:
		progress = min(progress+0.05, 1)
	default:
		return
	}
	w.Tapped(&fyne.PointEvent{Position: fyne.NewPos(w.Size().Width*progress, 0)})
}

func (w *WaveformSeekbar) TypedRune(r rune) {
}

var _ fyne.Tappable = (*WaveformSeekbar)(nil)

func (w *WaveformSeekbar) Tapped(e *fyne.PointEvent) {
	if !w.Disabled() && w.OnSeeked != nil {
		w.OnSeeked(float64(e.Position.X / w.Size().Width))
	}
}

func (w *WaveformSeekbar) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(
		container.NewStack(
			container.New(layout.NewCustomPaddedLayout(4, 4, 0, 0), w.img),
			container.NewWithoutLayout(w.cursor, w.focus),
		),
	)
}

// SetProgress sets how much of the seekbar has been played
// (ratio from 0 to 1)
func (w *WaveformSeekbar) SetProgress(v float64) {
	prm, fg, _ := w.getThemeColors()
	thresholdPixel := int(math.Round(1024.0 /*pixel width of waveform*/ * v))
	if w.updateImageProgress(prm, fg, thresholdPixel) {
		w.img.Refresh()
		if w.focused {
			w.moveCursorAndFocusToCurrentPosition()
		}
	}
}

func (w *WaveformSeekbar) getThemeColors() (primary, foreground, focus color.Color) {
	th := w.Theme()
	vnt := fyne.CurrentApp().Settings().ThemeVariant()
	primary = th.Color(theme.ColorNamePrimary, vnt)
	foreground = th.Color(theme.ColorNameForeground, vnt)
	focus = th.Color(theme.ColorNameFocus, vnt)
	return primary, foreground, focus
}

func (w *WaveformSeekbar) recolorCursor(prm, fg color.Color, posX float32) {
	progress := float32(w.imgProgressPixel) / 1024 /*waveform image width*/
	if posX/w.Size().Width < progress {
		w.cursor.FillColor = fg
	} else {
		w.cursor.FillColor = prm
	}
}

func (w *WaveformSeekbar) moveCursorAndFocusToCurrentPosition() {
	progress := float32(w.imgProgressPixel) / 1024
	pos := w.Size().Width * progress
	w.cursor.Move(fyne.NewPos(pos, 2))
	w.focus.Move(fyne.NewPos(pos-1, 1))
}

func (w *WaveformSeekbar) updateImageProgress(cL, cR color.Color, progress int) (updated bool) {
	if w.img.Image == nil {
		return false
	}
	if w.imgColorL == cL && w.imgColorR == cR && w.imgProgressPixel == progress {
		return false
	}

	img := w.img.Image.(*image.NRGBA)
	recolorWaveformImage(img, cL, cR, w.imgProgressPixel, progress, false)
	w.imgColorL, w.imgColorR = cL, cR
	w.imgProgressPixel = progress
	return true
}

func recolorWaveformImage(img *image.NRGBA, cL, cR color.Color, oldProgress, newProgress int, fullRecolor bool) {
	_r, _g, _b, _ := cL.RGBA()
	rL, gL, bL := byte(_r>>8), byte(_g>>8), byte(_b>>8)
	_r, _g, _b, _ = cR.RGBA()
	rR, gR, bR := byte(_r>>8), byte(_g>>8), byte(_b>>8)

	bnds := img.Rect.Bounds()
	xMin, xMax := 0, bnds.Dx()
	if !fullRecolor {
		xMin = max(0, min(oldProgress, newProgress))
		xMax = min(bnds.Dx(), max(oldProgress, newProgress))
	}
	for x := xMin; x < xMax; x++ {
		for y := 0; y < bnds.Dy(); y++ {
			if x < newProgress {
				setPixelRGB(img, x, y, rL, gL, bL)
			} else {
				setPixelRGB(img, x, y, rR, gR, bR)
			}
		}
	}
}

func setPixelRGB(img *image.NRGBA, x, y int, r, g, b byte) {
	offset := img.PixOffset(x, y)
	img.Pix[offset+0] = r
	img.Pix[offset+1] = g
	img.Pix[offset+2] = b
}
