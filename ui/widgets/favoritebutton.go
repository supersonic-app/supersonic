package widgets

import (
	"github.com/supersonic-app/supersonic/ui/theme"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type FavoriteButton struct {
	widget.Button

	IsFavorited bool
}

func NewFavoriteButton(onTapped func()) *FavoriteButton {
	f := &FavoriteButton{
		Button: widget.Button{
			OnTapped: onTapped,
			Icon:     theme.NotFavoriteIcon,
		},
	}
	f.ExtendBaseWidget(f)
	return f
}

func (f *FavoriteButton) Tapped(e *fyne.PointEvent) {
	f.IsFavorited = !f.IsFavorited
	f.Button.Tapped(e)
	f.Refresh()
}

func (f *FavoriteButton) Refresh() {
	if f.IsFavorited {
		f.Icon = theme.FavoriteIcon
	} else {
		f.Icon = theme.NotFavoriteIcon
	}
	f.Button.Refresh()
}
