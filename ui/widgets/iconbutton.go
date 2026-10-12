package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	myTheme "github.com/supersonic-app/supersonic/ui/theme"
)

type IconButtonSize int

const (
	IconButtonSizeNormal IconButtonSize = iota
	IconButtonSizeSlightlyBigger
	IconButtonSizeBigger
	IconButtonSizeSmaller
	IconButtonSizeSmallest
)

type IconButton struct {
	ttwidget.ToolTipWidget

	Highlighted bool
	IconSize    IconButtonSize
	OnTapped    func()

	icon     fyne.Resource
	focused  bool
	hovered  bool
	disabled bool

	themed *theme.ThemedResource
	img    *canvas.Image
}

var (
	_ fyne.Tappable     = (*IconButton)(nil)
	_ fyne.Focusable    = (*IconButton)(nil)
	_ fyne.Disableable  = (*IconButton)(nil)
	_ desktop.Hoverable = (*IconButton)(nil)
)

func NewIconButton(icon fyne.Resource, onTapped func()) *IconButton {
	i := &IconButton{icon: icon, OnTapped: onTapped}
	i.ExtendBaseWidget(i)
	return i
}

func (i *IconButton) SetIcon(icon fyne.Resource) {
	i.icon = icon
	if i.img != nil {
		i.themed = theme.NewThemedResource(icon)
		i.img.Resource = i.themed
		i.Refresh()
	}
}

func (i *IconButton) Disable() {
	if !i.disabled {
		i.disabled = true
		i.Refresh()
	}
}

func (i *IconButton) Enable() {
	if i.disabled {
		i.disabled = false
		i.Refresh()
	}
}

func (i *IconButton) Disabled() bool {
	return i.disabled
}

func (i *IconButton) Tapped(*fyne.PointEvent) {
	if !i.disabled && i.OnTapped != nil {
		i.OnTapped()
	}
}

func (i *IconButton) FocusGained() {
	if !i.focused {
		defer i.Refresh()
	}
	i.focused = true
}

func (i *IconButton) FocusLost() {
	if i.focused {
		defer i.Refresh()
	}
	i.focused = false
}

func (i *IconButton) TypedKey(e *fyne.KeyEvent) {
	if !i.disabled && e.Name == fyne.KeySpace {
		i.Tapped(nil)
	}
}

func (i *IconButton) TypedRune(r rune) {
}

func (i *IconButton) MouseIn(e *desktop.MouseEvent) {
	i.ToolTipWidget.MouseIn(e)
	if i.disabled {
		return
	}

	if !i.hovered {
		defer i.Refresh()
	}
	i.hovered = true
}

func (i *IconButton) MouseOut() {
	i.ToolTipWidget.MouseOut()
	if i.disabled {
		return
	}

	if i.hovered {
		defer i.Refresh()
	}
	i.hovered = false
}

func (i *IconButton) MouseMoved(e *desktop.MouseEvent) {
	i.ToolTipWidget.MouseMoved(e)
}

func (i *IconButton) MinSize() fyne.Size {
	return i.iconSize()
}

func (i *IconButton) iconSize() fyne.Size {
	switch i.IconSize {
	case IconButtonSizeBigger:
		return fyne.NewSquareSize(theme.IconInlineSize() * 2)
	case IconButtonSizeSlightlyBigger:
		return fyne.NewSquareSize(theme.IconInlineSize() * 1.37)
	case IconButtonSizeSmaller:
		return fyne.NewSquareSize(theme.IconInlineSize())
	case IconButtonSizeSmallest:
		return fyne.NewSquareSize(theme.IconInlineSize() * 0.9)
	default:
		return fyne.NewSquareSize(theme.IconInlineSize() * 1.3333)
	}
}

func (i *IconButton) updateColor() {
	if i.disabled {
		i.themed.ColorName = theme.ColorNameDisabled
	} else if i.Highlighted || i.focused {
		i.themed.ColorName = myTheme.ColorNameActiveIconButton
	} else if i.hovered {
		i.themed.ColorName = myTheme.ColorNameHoveredIconButton
	} else {
		i.themed.ColorName = myTheme.ColorNameIconButton
	}
}

func (i *IconButton) Refresh() {
	if i.img == nil {
		return
	}
	i.updateColor()
	i.img.SetMinSize(i.iconSize())
	i.img.Refresh()
}

func (i *IconButton) CreateRenderer() fyne.WidgetRenderer {
	if i.img == nil {
		i.themed = theme.NewThemedResource(i.icon)
		i.img = canvas.NewImageFromResource(i.themed)
		i.img.FillMode = canvas.ImageFillContain
		i.img.SetMinSize(i.iconSize())
		i.updateColor()
	}
	return widget.NewSimpleRenderer(container.NewCenter(i.img))
}
