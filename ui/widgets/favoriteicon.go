package widgets

import (
	"github.com/supersonic-app/supersonic/ui/theme"
)

type FavoriteIcon struct {
	TappableIcon

	Favorite bool
}

func NewFavoriteIcon() *FavoriteIcon {
	f := &FavoriteIcon{}
	f.Resource = theme.NotFavoriteIcon
	f.ExtendBaseWidget(f)
	return f
}

func (f *FavoriteIcon) Refresh() {
	if f.Favorite {
		f.Resource = theme.FavoriteIcon
	} else {
		f.Resource = theme.NotFavoriteIcon
	}
	f.BaseWidget.Refresh()
}
