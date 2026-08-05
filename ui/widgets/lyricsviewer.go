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

	noLyricsMsg   fyne.CanvasObject
	viewer        *fynelyrics.LyricsViewer
	lyrics        *mediaprovider.Lyrics
	nextLyricLine int
	lastPlayPos   float64

	// keeps track if UpdatePlayPos has been called yet
	// for the current lyrics
	firstUpdate bool

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
	l.lyrics = lyrics
	l.nextLyricLine = 0
	l.firstUpdate = true
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
	lines := make([]string, len(lyrics.Lines))
	for i, line := range lyrics.Lines {
		lines[i] = line.Text
	}
	l.viewer.SetLyrics(lines, lyrics.Synced)
	if l.isEmpty {
		l.container.Objects[0] = l.viewer
		l.isEmpty = false
		l.Refresh()
	}
}

func (l *LyricsViewer) UpdatePlayPos(timeSecs float64) {
	if l.lyrics == nil || !l.lyrics.Synced {
		return
	}
	if l.firstUpdate || timeSecs < l.lastPlayPos {
		l.OnSeeked(timeSecs)
		l.firstUpdate = false
		return
	}
	l.lastPlayPos = timeSecs
	// advance if needed
	if l.nextLyricLine >= 0 && l.nextLyricLine < len(l.lyrics.Lines) &&
		l.lyrics.Lines[l.nextLyricLine].Start <= timeSecs {
		l.viewer.NextLine()
		if l.nextLyricLine < len(l.lyrics.Lines)-1 {
			l.nextLyricLine++
		}
	}
}

func (l *LyricsViewer) OnSeeked(timeSecs float64) {
	l.lastPlayPos = timeSecs
	if l.lyrics == nil || !l.lyrics.Synced {
		return
	}

	// find first line that starts after timeSecs
	nextLine := -1
	for i, l := range l.lyrics.Lines {
		if l.Start > timeSecs {
			nextLine = i
			break
		}
	}

	if nextLine == -1 {
		// last lyric
		nextLine = len(l.lyrics.Lines)
		l.nextLyricLine = nextLine - 1
	} else {
		l.nextLyricLine = nextLine
	}
	if nextLine >= 0 && nextLine <= len(l.lyrics.Lines) {
		l.viewer.SetCurrentLine(nextLine /*one-indexed*/)
	}
}

func (l *LyricsViewer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(l.container)
}
