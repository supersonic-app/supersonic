package widgets

import (
	"github.com/supersonic-app/supersonic/backend"
	myTheme "github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// TrackPosSlider is a custom slider that exposes an additional
// IsDragging() API as well as some other customizations
type TrackPosSlider struct {
	widget.Slider

	// to avoid "data echoes" when slider value is updated as
	// playback position changes
	IgnoreNextChangeEnded bool

	isDragging bool
}

func NewTrackPosSlider() *TrackPosSlider {
	slider := &TrackPosSlider{
		Slider: widget.Slider{
			Value:       0,
			Min:         0,
			Max:         1,
			Step:        0.002,
			Orientation: widget.Horizontal,
		},
	}
	slider.ExtendBaseWidget(slider)
	return slider
}

func (t *TrackPosSlider) SetValue(value float64) {
	t.IgnoreNextChangeEnded = true
	t.Slider.SetValue(value)
}

func (t *TrackPosSlider) Tapped(e *fyne.PointEvent) {
	t.isDragging = false
	t.IgnoreNextChangeEnded = false
	t.Slider.Tapped(e)

	// don't keep focus after being tapped
	if c := fyne.CurrentApp().Driver().CanvasForObject(t); c != nil {
		c.Unfocus()
	}
}

// override to increase the distance moved by keyboard control
func (t *TrackPosSlider) TypedKey(e *fyne.KeyEvent) {
	switch e.Name {
	case fyne.KeyLeft:
		t.Slider.SetValue(t.Value - 0.05)
	case fyne.KeyRight:
		t.Slider.SetValue(t.Value + 0.05)
	default:
		t.Slider.TypedKey(e)
	}
}

func (t *TrackPosSlider) DragEnd() {
	t.isDragging = false
	t.IgnoreNextChangeEnded = false
	t.Slider.DragEnd()
}

func (t *TrackPosSlider) Dragged(e *fyne.DragEvent) {
	t.isDragging = true
	t.Slider.Dragged(e)
}

func (t *TrackPosSlider) IsDragging() bool {
	return t.isDragging
}

type PlayerControls struct {
	widget.BaseWidget

	UseWaveformSeekbar bool

	OnChangeShuffle func(shuffle bool)

	slider         *TrackPosSlider
	waveform       *WaveformSeekbar
	curTimeLabel   *labelMinSize
	totalTimeLabel *labelMinSize
	shuffle        *IconButton
	prev           *IconButton
	playpause      *IconButton
	next           *IconButton
	loop           *IconButton
	container      *fyne.Container

	totalTime float64
}

var _ fyne.Widget = (*PlayerControls)(nil)

type labelMinSize struct {
	widget.Label
	MinWidth float32
}

func (l *labelMinSize) MinSize() fyne.Size {
	return fyne.NewSize(l.MinWidth, l.Label.MinSize().Height)
}

func NewLabelMinSize(text string, minWidth float32) *labelMinSize {
	l := &labelMinSize{MinWidth: minWidth, Label: widget.Label{Text: text}}
	l.ExtendBaseWidget(l)
	return l
}

// NewPlayerControls sets up the seek bar, and transport buttons.
func NewPlayerControls(useWaveformSeekbar bool, initialLoopMode backend.LoopMode, initialShuffle bool) *PlayerControls {
	pc := &PlayerControls{UseWaveformSeekbar: useWaveformSeekbar}
	pc.ExtendBaseWidget(pc)

	pc.slider = NewTrackPosSlider()
	pc.slider.Disable()
	pc.waveform = NewWaveformSeekbar()
	pc.waveform.Disable()
	if useWaveformSeekbar {
		pc.slider.Hidden = true
	} else {
		pc.waveform.Hidden = true
	}
	pc.curTimeLabel = NewLabelMinSize(util.SecondsToMMSS(0), 55)
	pc.curTimeLabel.Alignment = fyne.TextAlignTrailing
	pc.totalTimeLabel = NewLabelMinSize(util.SecondsToMMSS(0), 55)
	pc.totalTimeLabel.Alignment = fyne.TextAlignTrailing

	pc.slider.OnChanged = func(f float64) {
		if pc.slider.IsDragging() {
			time := f * pc.totalTime
			pc.curTimeLabel.SetText(util.SecondsToMMSS(time))
		}
	}

	pc.shuffle = NewIconButton(myTheme.ShuffleIcon, nil)
	pc.shuffle.IconSize = IconButtonSizeSmallest
	pc.shuffle.SetToolTip(lang.L("Shuffle"))
	pc.shuffle.OnTapped = func() {
		pc.SetShuffle(!pc.shuffle.Highlighted)
		if pc.OnChangeShuffle != nil {
			pc.OnChangeShuffle(pc.shuffle.Highlighted)
		}
	}

	pc.loop = NewIconButton(myTheme.RepeatIcon, nil)
	pc.loop.IconSize = IconButtonSizeSmallest
	pc.loop.SetToolTip(lang.L("Repeat"))

	pc.shuffle.Highlighted = initialShuffle
	pc.SetLoopMode(initialLoopMode)

	pc.prev = NewIconButton(theme.MediaSkipPreviousIcon(), func() {})
	pc.prev.SetToolTip(lang.L("Previous"))
	pc.prev.IconSize = IconButtonSizeSlightlyBigger
	pc.next = NewIconButton(theme.MediaSkipNextIcon(), func() {})
	pc.next.SetToolTip(lang.L("Next"))
	pc.next.IconSize = IconButtonSizeSlightlyBigger
	pc.playpause = NewIconButton(theme.MediaPlayIcon(), func() {})
	pc.playpause.SetToolTip(lang.L("Play"))
	pc.playpause.IconSize = IconButtonSizeBigger

	buttons := container.NewHBox(layout.NewSpacer(), pc.shuffle, util.NewHSpace(2), pc.prev, pc.playpause, pc.next, util.NewHSpace(2), pc.loop, layout.NewSpacer())

	seekCtrl := container.NewStack(
		pc.slider,
		pc.waveform,
	)
	c := container.NewBorder(nil, nil, pc.curTimeLabel, pc.totalTimeLabel, seekCtrl)
	pc.container = container.New(layout.NewCustomPaddedVBoxLayout(0), c, buttons)

	return pc
}

func (pc *PlayerControls) OnSeek(f func(float64)) {
	pc.slider.OnChangeEnded = func(pos float64) {
		if pc.slider.IgnoreNextChangeEnded {
			pc.slider.IgnoreNextChangeEnded = false
		} else {
			f(pos)
		}
	}
	pc.waveform.OnSeeked = f
}

func (pc *PlayerControls) OnSeekPrevious(f func()) {
	pc.prev.OnTapped = f
}

func (pc *PlayerControls) OnChangeLoopMode(f func()) {
	pc.loop.OnTapped = f
}

func (pc *PlayerControls) SetLoopMode(mode backend.LoopMode) {
	switch mode {
	case backend.LoopAll:
		pc.loop.Highlighted = true
		pc.loop.SetIcon(myTheme.RepeatIcon)
	case backend.LoopOne:
		pc.loop.Highlighted = true
		pc.loop.SetIcon(myTheme.RepeatOneIcon)
	case backend.LoopNone:
		pc.loop.Highlighted = false
		pc.loop.SetIcon(myTheme.RepeatIcon)
	}
}

func (pc *PlayerControls) SetShuffle(isShuffle bool) {
	if isShuffle == pc.shuffle.Highlighted {
		return
	}
	pc.shuffle.Highlighted = isShuffle
	pc.shuffle.Refresh()
}

func (pc *PlayerControls) OnSeekNext(f func()) {
	pc.next.OnTapped = f
}

func (pc *PlayerControls) OnPlayPause(f func()) {
	pc.playpause.OnTapped = f
}

func (pc *PlayerControls) SetPlaying(playing bool) {
	if playing {
		pc.playpause.SetIcon(theme.MediaPauseIcon())
		pc.playpause.SetToolTip(lang.L("Pause"))
	} else {
		pc.playpause.SetIcon(theme.MediaPlayIcon())
		pc.playpause.SetToolTip(lang.L("Play"))
	}
}

func (pc *PlayerControls) UpdatePlayTime(curTime, totalTime float64) {
	pc.totalTime = totalTime
	v := 0.0
	if totalTime > 0 {
		v = curTime / totalTime
	}

	updated := false
	tt := util.SecondsToMMSS(totalTime)
	if tt != pc.totalTimeLabel.Text {
		pc.totalTimeLabel.SetText(tt)
		updated = true
	}
	if totalTime > 0 {
		pc.slider.Enable()
		pc.waveform.Enable()
	} else {
		pc.slider.Disable()
		pc.waveform.Disable()
	}
	if !pc.slider.IsDragging() {
		ct := util.SecondsToMMSS(curTime)
		if ct != pc.curTimeLabel.Text {
			pc.curTimeLabel.SetText(ct)
			updated = true
		}
		pc.waveform.SetProgress(v)
		if updated {
			// Only update slider once a second when time label changes
			pc.slider.SetValue(v)
		}
	}
}

func (p *PlayerControls) UpdateWaveformImg(img *backend.WaveformImage) {
	p.waveform.UpdateImage(img)
}

func (p *PlayerControls) Refresh() {
	p.waveform.Hidden = !p.UseWaveformSeekbar
	p.slider.Hidden = p.UseWaveformSeekbar
	p.BaseWidget.Refresh()
}

func (p *PlayerControls) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.container)
}
