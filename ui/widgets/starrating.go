package widgets

import (
	"math"

	"github.com/supersonic-app/supersonic/res"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	themedResStarFilled       = theme.NewThemedResource(res.ResStarFilledSvg)
	themedResStarOutline      = theme.NewThemedResource(res.ResStarOutlineSvg)
	themedDisabledStarOutline = theme.NewDisabledResource(res.ResStarOutlineSvg)
)

var _ fyne.Disableable = (*StarRating)(nil)

type StarRating struct {
	widget.BaseWidget

	IsDisabled bool
	Rating     int
	StarSize   float32

	OnRatingChanged func(int)

	holdRating       bool // don't render mouseHoverRating
	mouseHoverRating int  // zero if not hovered
	container        *fyne.Container
}

func NewStarRating() *StarRating {
	s := &StarRating{}
	s.ExtendBaseWidget(s)
	return s
}

func (s *StarRating) createContainer() {
	s.container = container.New(layout.NewCustomPaddedHBoxLayout(0))
	var im *canvas.Image
	for i := range 5 {
		if s.IsDisabled {
			im = canvas.NewImageFromResource(themedDisabledStarOutline)
		} else if s.Rating > i {
			im = canvas.NewImageFromResource(themedResStarFilled)
		} else {
			im = canvas.NewImageFromResource(themedResStarOutline)
		}
		im.SetMinSize(fyne.NewSize(s.StarSize, s.StarSize))
		s.container.Add(im)
	}
}

var _ desktop.Hoverable = (*StarRating)(nil)

func (s *StarRating) MouseIn(e *desktop.MouseEvent) {
	s.MouseMoved(e)
}

func (s *StarRating) MouseMoved(e *desktop.MouseEvent) {
	if s.IsDisabled {
		return
	}
	hoverRating := int(math.Ceil(5 * float64(e.Position.X/s.Size().Width)))
	if s.mouseHoverRating != hoverRating {
		s.holdRating = false
		s.mouseHoverRating = hoverRating
		s.Refresh()
	}
}

func (s *StarRating) Disable() {
	s.IsDisabled = true
	s.Refresh()
}

func (s *StarRating) Enable() {
	s.IsDisabled = false
	s.Refresh()
}

func (s *StarRating) Disabled() bool {
	return s.IsDisabled
}

func (s *StarRating) MouseOut() {
	if s.IsDisabled {
		return
	}
	s.mouseHoverRating = 0
	s.holdRating = false
	s.Refresh()
}

var _ fyne.Tappable = (*StarRating)(nil)

func (s *StarRating) Tapped(*fyne.PointEvent) {
	if s.mouseHoverRating <= 0 {
		return // shouldn't happen
	}
	if s.Rating == s.mouseHoverRating {
		s.Rating = 0
	} else {
		s.Rating = s.mouseHoverRating
	}
	s.holdRating = true
	s.Refresh()
	if s.OnRatingChanged != nil {
		s.OnRatingChanged(s.Rating)
	}
}

func (s *StarRating) Refresh() {
	// widget has not had renderer created yet
	if s.container == nil {
		return
	}
	rating := s.Rating
	if !s.holdRating && s.mouseHoverRating > 0 {
		rating = s.mouseHoverRating
	}
	for i := range 5 {
		im := s.container.Objects[i].(*canvas.Image)
		im.SetMinSize(fyne.NewSize(s.StarSize, s.StarSize))
		if s.IsDisabled {
			im.Resource = themedDisabledStarOutline
		} else if rating > i {
			im.Resource = themedResStarFilled
		} else {
			im.Resource = themedResStarOutline
		}
	}
	s.BaseWidget.Refresh()
}

func (s *StarRating) MinSize() fyne.Size {
	return fyne.NewSize(s.StarSize*5, s.StarSize)
}

func (s *StarRating) CreateRenderer() fyne.WidgetRenderer {
	if s.container == nil {
		s.createContainer()
	}
	return widget.NewSimpleRenderer(container.NewCenter(s.container))
}
