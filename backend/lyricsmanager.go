package backend

import (
	"context"
	"log"
	"sync"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

type LyricsManager struct {
	sm     *ServerManager
	lrclib *LrcLibFetcher

	// right now only one song can have lyrics being fetched
	// at any given time (b/c we only show lyrics for the currently playing song)
	lock                  sync.Mutex
	fetchInProgressID     string
	fetchInProgressCancel context.CancelFunc
	cbs                   []func(string, *mediaprovider.Lyrics)
}

func NewLyricsManager(sm *ServerManager, lrclib *LrcLibFetcher) *LyricsManager {
	return &LyricsManager{
		sm:     sm,
		lrclib: lrclib,
	}
}

func (lm *LyricsManager) FetchLyricsAsync(song *mediaprovider.Track, cb func(string, *mediaprovider.Lyrics)) {
	lm.lock.Lock()
	defer lm.lock.Unlock()

	if lm.fetchInProgressID == song.ID {
		lm.cbs = append(lm.cbs, cb)
		return
	}
	lm.fetchInProgressID = song.ID

	if lm.fetchInProgressCancel != nil {
		lm.fetchInProgressCancel()
	}
	lm.cbs = []func(string, *mediaprovider.Lyrics){cb}
	ctx, cancel := context.WithCancel(context.Background())
	lm.fetchInProgressCancel = cancel
	go lm.fetchLyrics(ctx, song, func(id string, lyrics *mediaprovider.Lyrics) {
		lm.lock.Lock()
		defer lm.lock.Unlock()
		lm.fetchInProgressID = ""
		lm.fetchInProgressCancel()
		for _, cb := range lm.cbs {
			cb(id, lyrics)
		}
	})
}

func (lm *LyricsManager) fetchLyrics(ctx context.Context, song *mediaprovider.Track, cb func(string, *mediaprovider.Lyrics)) {
	var lyrics *mediaprovider.Lyrics
	var err error
	if lp, ok := lm.sm.Server.(mediaprovider.LyricsProvider); ok {
		if lyrics, err = lp.GetLyrics(song); err != nil {
			log.Printf("Error fetching lyrics: %v", err)
		}
	}
	if lyrics == nil && lm.lrclib != nil {
		artist := ""
		if len(song.ArtistNames) > 0 {
			artist = song.ArtistNames[0]
		}
		lyrics, err = lm.lrclib.FetchLrcLibLyrics(song.Title, artist, song.Album, int(song.Duration.Seconds()))
		if err != nil {
			log.Println(err.Error())
		}
	}
	select {
	case <-ctx.Done():
		return
	default:
		cb(song.ID, lyrics)
	}
}
