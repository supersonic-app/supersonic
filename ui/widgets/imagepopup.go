package widgets

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
)

type ImagePopUp struct {
	widget.PopUp

	img         image.Image
	desiredSize fyne.Size
}

func NewImagePopUp(img image.Image, canv fyne.Canvas, size fyne.Size) *ImagePopUp {
	i := &ImagePopUp{
		img:         img,
		desiredSize: size,
		PopUp: widget.PopUp{
			Canvas:  canv,
			Content: canvas.NewImageFromImage(img),
		},
	}
	i.ExtendBaseWidget(i)
	return i
}

func (i *ImagePopUp) Show() {
	i.Content.(*canvas.Image).FillMode = canvas.ImageFillContain
	i.Content.(*canvas.Image).ScaleMode = canvas.ImageScaleFastest
	canvSize := i.Canvas.Size()
	i.ShowAtPosition(fyne.NewPos(
		canvSize.Width/2,
		canvSize.Height/2,
	))
	anim := fyne.NewAnimation(myTheme.AnimationDurationShort, func(f float32) {
		if f == 1 {
			i.Content.(*canvas.Image).ScaleMode = canvas.ImageScaleSmooth
		}
		size := fyne.NewSize(i.desiredSize.Width*f, i.desiredSize.Height*f)
		i.Content.(*canvas.Image).SetMinSize(size)
		size = i.Content.MinSize()
		i.Resize(size)
		i.Move(fyne.NewPos(
			(canvSize.Width-size.Width)/2,
			(canvSize.Height-size.Height)/2,
		))
	})
	anim.Start()
}
