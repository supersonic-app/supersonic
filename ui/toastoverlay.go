package ui

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
	"github.com/supersonic-app/supersonic/ui/widgets"
)

const (
	toastAutoDismissalTime = 3 * time.Second
	toastAnimationDuration = myTheme.AnimationDurationShort
)

type ToastOverlay struct {
	widget.BaseWidget

	currentToast     *toast
	currentToastAnim *fyne.Animation
	dismissCancel    context.CancelFunc

	container *fyne.Container
}

func NewToastOverlay() *ToastOverlay {
	t := &ToastOverlay{container: container.NewWithoutLayout()}
	t.container.Objects = make([]fyne.CanvasObject, 0, 1)
	t.ExtendBaseWidget(t)
	return t
}

func (t *ToastOverlay) ShowSuccessToast(message string) {
	t.showToast(false, message)
}

func (t *ToastOverlay) ShowErrorToast(message string) {
	t.showToast(true, message)
}

func (t *ToastOverlay) showToast(isErr bool, message string) {
	t.cancelPreviousToast()

	t.currentToast = newToast(isErr, message, t.dismissToast)
	t.container.Objects = append(t.container.Objects, t.currentToast)

	s := t.Size()
	min := t.currentToast.MinSize()
	pad := theme.Padding()
	t.currentToast.Resize(min)
	endPos := fyne.NewPos(s.Width-min.Width-pad, s.Height-min.Height-pad)
	startPos := fyne.NewPos(s.Width, endPos.Y)
	f := t.makeToastAnimFunc(endPos, false)
	t.currentToastAnim = canvas.NewPositionAnimation(startPos, endPos, toastAnimationDuration, f)
	t.currentToastAnim.Curve = fyne.AnimationEaseOut
	t.currentToastAnim.Start()
	t.Refresh()

	ctx, cancel := context.WithCancel(context.Background())
	t.dismissCancel = cancel // always canceled by dismissToast
	go func() {
		time.Sleep(toastAutoDismissalTime)
		select {
		case <-ctx.Done():
			return
		default:
			fyne.Do(t.dismissToast)
		}
	}()
}

func (t *ToastOverlay) makeToastAnimFunc(endPos fyne.Position, dismissal bool) func(fyne.Position) {
	return func(p fyne.Position) {
		if ct := t.currentToast; ct != nil {
			ct.Move(p)
		}
		if p == endPos {
			t.currentToastAnim = nil
			if dismissal {
				t.cancelPreviousToast()
			}
		}
	}
}

func (t *ToastOverlay) Resize(size fyne.Size) {
	if t.currentToast != nil && t.currentToastAnim == nil {
		// move to the anchor position
		min := t.currentToast.MinSize()
		pad := theme.Padding()
		t.currentToast.Move(fyne.NewPos(size.Width-min.Width-pad, size.Height-min.Height-pad))
	} // else if animation is running -- well, hope this doesn't happen ;)

	t.BaseWidget.Resize(size)
}

func (t *ToastOverlay) cancelPreviousToast() {
	if t.currentToast == nil {
		return
	}
	if t.currentToastAnim != nil {
		t.currentToastAnim.Stop()
		t.currentToastAnim = nil
	}
	t.container.Objects[0] = nil
	t.container.Objects = t.container.Objects[:0]
	t.currentToast = nil
}

func (t *ToastOverlay) dismissToast() {
	if t.currentToast == nil {
		return
	}
	if t.dismissCancel != nil {
		t.dismissCancel()
		t.dismissCancel = nil
	}
	if t.currentToastAnim != nil {
		t.currentToastAnim.Stop()
	}
	s := t.Size()
	min := t.currentToast.MinSize()
	pad := theme.Padding()
	startPos := fyne.NewPos(s.Width-min.Width-pad, s.Height-min.Height-pad)
	endPos := fyne.NewPos(s.Width, startPos.Y)
	f := t.makeToastAnimFunc(endPos, true)
	t.currentToastAnim = canvas.NewPositionAnimation(startPos, endPos, toastAnimationDuration, f)
	t.currentToastAnim.Curve = fyne.AnimationEaseIn
	t.currentToastAnim.Start()
}

func (t *ToastOverlay) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.container)
}

type toast struct {
	widget.BaseWidget

	isErr     bool
	message   string
	onDismiss func()
}

func newToast(isErr bool, message string, onDismiss func()) *toast {
	t := &toast{isErr: isErr, message: message, onDismiss: onDismiss}
	t.ExtendBaseWidget(t)
	return t
}

func (t *toast) CreateRenderer() fyne.WidgetRenderer {
	return newToastRenderer(t)
}

func (t *toast) Dismiss() {
	if t.onDismiss != nil {
		t.onDismiss()
	}
}

// swallow all tap/mouse events because toast is transparent
var (
	_ fyne.Tappable          = (*toast)(nil)
	_ fyne.SecondaryTappable = (*toast)(nil)
	_ desktop.Hoverable      = (*toast)(nil)
	_ desktop.Mouseable      = (*toast)(nil)
)

func (*toast) Tapped(*fyne.PointEvent)          {}
func (*toast) TappedSecondary(*fyne.PointEvent) {}
func (*toast) MouseIn(*desktop.MouseEvent)      {}
func (*toast) MouseOut()                        {}
func (*toast) MouseMoved(*desktop.MouseEvent)   {}
func (*toast) MouseUp(*desktop.MouseEvent)      {}
func (*toast) MouseDown(*desktop.MouseEvent)    {}

type toastRenderer struct {
	container   *fyne.Container
	background  *canvas.Rectangle
	accent      *canvas.Rectangle
	accentColor fyne.ThemeColorName
}

func newToastRenderer(t *toast) *toastRenderer {
	title := lang.L("Success")
	accentColor := theme.ColorNamePrimary
	if t.isErr {
		title = lang.L("Error")
		accentColor = theme.ColorNameError
	}

	th := fyne.CurrentApp().Settings().Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	background := canvas.NewRectangle(th.Color(theme.ColorNameOverlayBackground, v))
	background.CornerRadius = th.Size(theme.SizeNameInputRadius)
	background.StrokeColor = th.Color(theme.ColorNameInputBorder, v)
	background.StrokeWidth = th.Size(theme.SizeNameInputBorder) * 2
	accent := canvas.NewRectangle(th.Color(accentColor, v))
	accent.SetMinSize(fyne.NewSize(4, 1))

	close := widgets.NewIconButton(theme.CancelIcon(), t.Dismiss)
	close.IconSize = widgets.IconButtonSizeSmaller

	titleText := widget.NewRichTextWithText(title)
	titleText.Segments[0].(*widget.TextSegment).Style = widget.RichTextStyleSubHeading

	pad := theme.Padding()
	return &toastRenderer{
		background:  background,
		accent:      accent,
		accentColor: accentColor,
		container: container.NewStack(
			background,
			container.New(&layout.CustomPaddedLayout{
				TopPadding:    2 * pad,
				BottomPadding: 2 * pad,
				LeftPadding:   2 * pad,
				RightPadding:  pad,
			},
				container.NewBorder(nil, nil, accent, nil,
					container.NewVBox(
						container.NewBorder(nil, nil, nil, container.NewHBox(close, util.NewHSpace(2)), titleText),
						widget.NewLabel(t.message),
					),
				),
			),
		),
	}
}

var _ fyne.WidgetRenderer = (*toastRenderer)(nil)

func (*toastRenderer) Destroy() {}

func (t *toastRenderer) Layout(s fyne.Size) {
	t.container.Layout.Layout(t.container.Objects, s)
}

func (t *toastRenderer) MinSize() fyne.Size {
	return t.container.MinSize()
}

func (t *toastRenderer) Objects() []fyne.CanvasObject {
	return t.container.Objects
}

func (t *toastRenderer) Refresh() {
	th := fyne.CurrentApp().Settings().Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()

	t.background.FillColor = th.Color(theme.ColorNameOverlayBackground, v)
	t.background.CornerRadius = th.Size(theme.SizeNameInputRadius)
	t.background.StrokeColor = th.Color(theme.ColorNameInputBorder, v)
	t.background.StrokeWidth = th.Size(theme.SizeNameInputBorder) * 2
	t.background.Refresh()

	t.accent.FillColor = th.Color(t.accentColor, v)
	t.accent.Refresh()

	canvas.Refresh(t.container)
}
