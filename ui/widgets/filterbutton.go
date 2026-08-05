package widgets

import (
	"fyne.io/fyne/v2"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

type FilterButton[M, F any] interface {
	fyne.CanvasObject

	Filter() mediaprovider.MediaFilter[M, F]
	SetOnChanged(func())
}
