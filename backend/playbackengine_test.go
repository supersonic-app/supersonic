package backend

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/player"
)

type networkErrorPlayer struct {
	player.BasePlayerCallbackImpl
	status        player.Status
	playedURL     string
	playStartTime float64
}

func (p *networkErrorPlayer) Continue() error                                           { return nil }
func (p *networkErrorPlayer) Pause() error                                              { return nil }
func (p *networkErrorPlayer) SeekSeconds(float64) error                                 { return nil }
func (p *networkErrorPlayer) IsSeeking() bool                                           { return false }
func (p *networkErrorPlayer) SetVolume(int) error                                       { return nil }
func (p *networkErrorPlayer) GetVolume() int                                            { return 50 }
func (p *networkErrorPlayer) GetStatus() player.Status                                  { return p.status }
func (p *networkErrorPlayer) Destroy()                                                  {}
func (p *networkErrorPlayer) SetNextFile(string, mediaprovider.MediaItemMetadata) error { return nil }

func (p *networkErrorPlayer) Stop(bool) error {
	p.status.State = player.Stopped
	p.InvokeOnStopped()
	return nil
}

func (p *networkErrorPlayer) PlayFile(url string, metadata mediaprovider.MediaItemMetadata, startTime float64) error {
	p.playedURL = url
	p.playStartTime = startTime
	p.status = player.Status{State: player.Playing, TimePos: startTime, Duration: metadata.Duration.Seconds()}
	p.InvokeOnTrackChange()
	return nil
}

type streamURLProvider struct {
	mediaprovider.MediaProvider
	reports chan string
}

func (s *streamURLProvider) GetStreamURL(trackID string, _ *mediaprovider.TranscodeSettings, _ bool) (string, error) {
	return "https://music.example/stream/" + trackID, nil
}

func (s *streamURLProvider) ReportPlayback(_ string, _ int64, state string) error {
	s.reports <- state
	return nil
}

func newNetworkErrorEngine(p *networkErrorPlayer) (*playbackEngine, []*mediaprovider.Track) {
	tracks := []*mediaprovider.Track{
		{ID: "first", Duration: 4 * time.Minute},
		{ID: "interrupted", Duration: 5 * time.Minute},
	}
	items := []mediaprovider.MediaItem{tracks[0], tracks[1]}
	engine := &playbackEngine{
		ctx:                       context.Background(),
		sm:                        &ServerManager{Server: &streamURLProvider{reports: make(chan string, 4)}},
		player:                    p,
		playbackCfg:               &PlaybackConfig{},
		scrobbleCfg:               &ScrobbleConfig{},
		transcodeCfg:              &TranscodingConfig{},
		playQueue:                 items,
		nowPlayingIdx:             1,
		pendingTrackChangeNum:     -1,
		curTrackDuration:          tracks[1].Duration.Seconds(),
		latestTrackPosition:       180,
		lastObservedTrackPosition: 70,
	}
	engine.registerPlayerCallbacks(p)
	return engine, tracks
}

func TestPlaybackErrorRetriesCurrentTrackAtLastObservedPosition(t *testing.T) {
	p := &networkErrorPlayer{status: player.Status{State: player.Playing, TimePos: 70, Duration: 300}}
	engine, tracks := newNetworkErrorEngine(p)

	p.InvokeOnPlaybackError(errors.New("network connection lost"))
	p.status = player.Status{State: player.Stopped}
	p.InvokeOnStopped()

	if got := engine.NowPlayingIndex(); got != 1 {
		t.Fatalf("NowPlayingIndex after playback error = %d, want 1", got)
	}
	status := engine.PlaybackStatus()
	if status.State != player.Paused || status.TimePos != 70 {
		t.Fatalf("PlaybackStatus after playback error = %+v, want paused at 70 seconds", status)
	}
	provider := engine.sm.Server.(*streamURLProvider)
	select {
	case state := <-provider.reports:
		if state != "paused" {
			t.Fatalf("reported playback state = %q, want paused", state)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for paused playback report")
	}

	if err := engine.Continue(); err != nil {
		t.Fatal(err)
	}
	if want := "https://music.example/stream/" + tracks[1].ID; p.playedURL != want {
		t.Fatalf("retried URL = %q, want %q", p.playedURL, want)
	}
	if p.playStartTime != 70 {
		t.Fatalf("retry start time = %v, want 70", p.playStartTime)
	}
	if got := len(engine.getActivePlayQueue()); got != 2 {
		t.Fatalf("queue length after retry = %d, want 2", got)
	}
}

func TestRequestedStopResetsCurrentTrack(t *testing.T) {
	p := &networkErrorPlayer{status: player.Status{State: player.Playing, TimePos: 70, Duration: 300}}
	engine, _ := newNetworkErrorEngine(p)

	if err := engine.Stop(); err != nil {
		t.Fatal(err)
	}
	if got := engine.NowPlayingIndex(); got != -1 {
		t.Fatalf("NowPlayingIndex after requested stop = %d, want -1", got)
	}
	if engine.pendingLoadPaused {
		t.Fatal("requested stop must not create resumable playback state")
	}
}

func TestStoppedWithoutPlaybackErrorDoesNotCreateResumeState(t *testing.T) {
	p := &networkErrorPlayer{status: player.Status{State: player.Playing, TimePos: 70, Duration: 300}}
	engine, _ := newNetworkErrorEngine(p)

	p.status = player.Status{State: player.Stopped}
	p.InvokeOnStopped()

	if got := engine.NowPlayingIndex(); got != -1 {
		t.Fatalf("NowPlayingIndex after normal stop = %d, want -1", got)
	}
	if engine.pendingLoadPaused {
		t.Fatal("normal stop must not create resumable playback state")
	}
}

func TestRequestedStopCancelsPendingPlaybackErrorResume(t *testing.T) {
	p := &networkErrorPlayer{status: player.Status{State: player.Playing, TimePos: 70, Duration: 300}}
	engine, _ := newNetworkErrorEngine(p)

	p.InvokeOnPlaybackError(errors.New("network connection lost"))
	if err := engine.Stop(); err != nil {
		t.Fatal(err)
	}

	if engine.pendingLoadPaused {
		t.Fatal("requested stop must cancel pending playback error recovery")
	}
	if got := engine.NowPlayingIndex(); got != -1 {
		t.Fatalf("NowPlayingIndex after requested stop = %d, want -1", got)
	}
}
