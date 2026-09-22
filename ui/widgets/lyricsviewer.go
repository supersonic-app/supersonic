package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
	fynelyrics "github.com/supersonic-app/fyne-lyrics"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/ui/theme"
)

type LyricsViewer struct {
	widget.BaseWidget

	noLyricsMsg fyne.CanvasObject
	viewer      *fynelyrics.LyricsViewer

	onSeekToLine func(int)

	container *fyne.Container
	isEmpty   bool
}

func NewLyricsViewer(onSeekToLine func(int)) *LyricsViewer {
	l := &LyricsViewer{
		noLyricsMsg: container.NewCenter(NewInfoMessage(
			lang.L("Lyrics not available"), "")),
		isEmpty:      true,
		onSeekToLine: onSeekToLine,
	}
	l.ExtendBaseWidget(l)
	l.container = container.NewStack(l.noLyricsMsg)
	return l
}

func (l *LyricsViewer) DisableTapToSeek() {
	if l.viewer != nil {
		l.viewer.OnLyricTapped = nil
	}
}

func (l *LyricsViewer) EnableTapToSeek() {
	if l.viewer != nil {
		l.viewer.OnLyricTapped = l.onSeekToLine
	}
}

func (l *LyricsViewer) SetLyrics(lyrics *mediaprovider.Lyrics) {
	if lyrics == nil || len(lyrics.Lines) == 0 {
		if !l.isEmpty {
			l.container.Objects[0] = l.noLyricsMsg
			l.isEmpty = true
			l.Refresh()
		}
		return
	}

	if l.viewer == nil {
		l.viewer = fynelyrics.NewLyricsViewer()
		l.viewer.ActiveLyricPosition = fynelyrics.ActiveLyricPositionUpperMiddle
		l.viewer.InactiveLyricColorName = theme.ColorNameInactiveLyric
		l.viewer.OnLyricTapped = l.onSeekToLine
	}
	model := make([]fynelyrics.LyricLine, len(lyrics.Lines))
	for i, line := range lyrics.Lines {
		model[i] = fynelyrics.LyricLine{
			Start:    line.Start,
			Segments: parseLyricSegments(line.Text),
		}
	}
	l.viewer.SetLyricLines(model, lyrics.Synced)
	if l.isEmpty {
		l.container.Objects[0] = l.viewer
		l.isEmpty = false
		l.Refresh()
	}
}

// UpdatePlayPos drives the lyric viewer from the current play position.
// Line selection, line scrolling, and word-by-word highlighting are
// all handled by the fyne-lyrics widget from this time value.
func (l *LyricsViewer) UpdatePlayPos(timeSecs float64) {
	if l.viewer != nil {
		l.viewer.SetPlayTime(timeSecs, false)
	}
}

// OnSeeked tells the lyric viewer that playback was seeked to timeSecs,
// so it repositions the active line immediately.
func (l *LyricsViewer) OnSeeked(timeSecs float64) {
	if l.viewer != nil {
		l.viewer.SetPlayTime(timeSecs, true)
	}
}

func (l *LyricsViewer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(l.container)
}
