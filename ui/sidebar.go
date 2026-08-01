package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
	"github.com/dweymouth/supersonic/backend"
	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/ui/controller"
	"github.com/dweymouth/supersonic/ui/theme"
	"github.com/dweymouth/supersonic/ui/util"
	"github.com/dweymouth/supersonic/ui/widgets"
)

type Sidebar struct {
	widget.BaseWidget

	lm     *backend.LyricsManager
	config *backend.AppConfig

	queueList     *widgets.PlayQueueList
	lyricsViewer  *widgets.LyricsViewer
	lyricsLoading *widgets.LoadingDots
	tabs          *container.AppTabs

	nowPlaying    mediaprovider.MediaItem
	nowPlayingID  string
	nowPlayingIdx int
	allQueueItems []mediaprovider.MediaItem

	curLyrics   *mediaprovider.Lyrics
	curLyricsID string
	lastPlayPos float64
}

func NewSidebar(contr *controller.Controller, pm *backend.PlaybackManager, im *backend.ImageManager, lm *backend.LyricsManager, config *backend.AppConfig) *Sidebar {
	s := &Sidebar{
		lm:            lm,
		config:        config,
		queueList:     widgets.NewPlayQueueList(im, false),
		nowPlayingIdx: -1,
	}
	s.queueList.Reorderable = true
	contr.ConnectPlayQueuelistActions(s.queueList)

	s.lyricsViewer = widgets.NewLyricsViewer(func(i int) {
		if s.curLyrics != nil && len(s.curLyrics.Lines) > i-1 {
			time := s.curLyrics.Lines[i-1].Start
			pm.SeekSeconds(time)
		}
	})
	s.lyricsLoading = widgets.NewLoadingDots()

	s.tabs = container.NewAppTabs(
		container.NewTabItem(lang.L("Play Queue"), container.NewPadded(s.queueList)),
		container.NewTabItem(lang.L("Lyrics"), container.NewStack(
			s.lyricsViewer,
			container.NewCenter(s.lyricsLoading))),
	)
	s.tabs.OnSelected = func(*container.TabItem) {
		idx := s.tabs.SelectedIndex()
		if idx == 1 /*lyrics*/ {
			s.updateLyrics()
		}
	}

	pm.OnPlayTimeUpdate(func(curTime, _ float64, seeked bool) {
		s.lastPlayPos = curTime
		if seeked {
			fyne.Do(func() { s.lyricsViewer.OnSeeked(curTime) })
		} else {
			fyne.Do(func() { s.lyricsViewer.UpdatePlayPos(curTime) })
		}
	})

	s.ExtendBaseWidget(s)
	return s
}

func (s *Sidebar) SelectedIndex() int {
	return s.tabs.SelectedIndex()
}

func (s *Sidebar) SetSelectedIndex(idx int) {
	s.tabs.SelectIndex(idx)
}

// applyQueueItems applies the hide-played filter to the queue.
func (s *Sidebar) applyQueueItems() {
	displayItems, offset := util.FilterQueueForHidePlayed(s.allQueueItems, s.nowPlayingIdx, s.config.HidePlayedQueueTracks)
	s.queueList.SetPlayIndexOffset(offset)
	s.queueList.SetItems(displayItems)
}

func (s *Sidebar) SetQueueTracks(items []mediaprovider.MediaItem, nowPlayingIdx int) {
	s.allQueueItems = items
	s.nowPlayingIdx = nowPlayingIdx
	s.applyQueueItems()
}

func (s *Sidebar) SetNowPlaying(item mediaprovider.MediaItem, nowPlayingIdx int) {
	s.nowPlaying = item
	s.nowPlayingIdx = nowPlayingIdx
	id := ""
	if item != nil {
		id = item.Metadata().ID
	}
	// Re-filter the queue whenever the playing track changes
	s.applyQueueItems()
	s.queueList.SetNowPlaying(id)
	s.nowPlayingID = id
	if s.tabs.SelectedIndex() == 1 /*lyrics*/ {
		s.updateLyrics()
	}
}

// TODO: this is more or less copy-paste from the Now Playing page
// refactor this shared logic somewhere else?
func (s *Sidebar) updateLyrics() {
	if s.nowPlayingID == s.curLyricsID {
		if s.nowPlayingID != "" {
			// just need to sync the current time
			s.lyricsViewer.OnSeeked(s.lastPlayPos)
			return
		}
	}
	if s.nowPlaying == nil || s.nowPlaying.Metadata().Type == mediaprovider.MediaItemTypeRadioStation {
		s.lyricsViewer.SetLyrics(nil)
		s.curLyrics = nil
		s.curLyricsID = ""
		return
	}
	s.curLyricsID = s.nowPlayingID
	s.lyricsLoading.Start()
	// set the widget to an empty (not nil) lyric during fetch
	// to keep it from showing "Lyrics not available"
	s.lyricsViewer.DisableTapToSeek()
	s.lyricsViewer.SetLyrics(&mediaprovider.Lyrics{
		Synced: true,
		Lines:  []mediaprovider.LyricLine{{Text: ""}},
	})
	tr, _ := s.nowPlaying.(*mediaprovider.Track)

	s.lm.FetchLyricsAsync(tr, func(id string, lyrics *mediaprovider.Lyrics) {
		if id != s.nowPlayingID {
			return
		}
		fyne.Do(func() {
			s.lyricsLoading.Stop()
			s.lyricsViewer.EnableTapToSeek()
			s.lyricsViewer.SetLyrics(lyrics)
			s.curLyrics = lyrics
			if lyrics != nil {
				s.lyricsViewer.OnSeeked(s.lastPlayPos)
			}
		})
	})
}

func (s *Sidebar) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(
		theme.NewThemedRectangle(theme.ColorNamePageBackground),
		container.NewPadded(s.tabs),
	))
}
