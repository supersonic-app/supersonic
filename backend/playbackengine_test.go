package backend

import (
	"context"
	"testing"
	"time"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/player"
)

// streamURLServer is the only part of a MediaProvider the engine touches
// when handing tracks to a player.
type streamURLServer struct {
	mediaprovider.MediaProvider
}

func (streamURLServer) GetStreamURL(trackID string, _ *mediaprovider.TranscodeSettings, _ bool) (string, error) {
	return "http://server/stream/" + trackID, nil
}

// fakeURLPlayer reports whatever status it was last told to be in, and
// fires the callbacks a real player would when handed a track.
type fakeURLPlayer struct {
	player.BasePlayerCallbackImpl
	status player.Status
	played []string
}

func (f *fakeURLPlayer) PlayFile(url string, meta mediaprovider.MediaItemMetadata, startTime float64) error {
	f.played = append(f.played, meta.ID)
	f.status = player.Status{State: player.Playing, TimePos: startTime, Duration: meta.Duration.Seconds()}
	f.InvokeOnPlaying()
	f.InvokeOnTrackChange()
	return nil
}

func (f *fakeURLPlayer) SetNextFile(string, mediaprovider.MediaItemMetadata) error { return nil }
func (f *fakeURLPlayer) Continue() error                                           { return nil }
func (f *fakeURLPlayer) Pause() error                                              { return nil }
func (f *fakeURLPlayer) Stop(bool) error                                           { return nil }
func (f *fakeURLPlayer) SeekSeconds(float64) error                                 { return nil }
func (f *fakeURLPlayer) IsSeeking() bool                                           { return false }
func (f *fakeURLPlayer) SetVolume(int) error                                       { return nil }
func (f *fakeURLPlayer) GetVolume() int                                            { return 0 }
func (f *fakeURLPlayer) GetStatus() player.Status                                  { return f.status }
func (f *fakeURLPlayer) Destroy()                                                  {}

func newTestEngine(t *testing.T, p player.BasePlayer) *playbackEngine {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e := NewPlaybackEngine(ctx, &ServerManager{Server: streamURLServer{}}, nil, p,
		&PlaybackConfig{}, &ScrobbleConfig{}, &TranscodingConfig{})
	e.setPlayQueue([]mediaprovider.MediaItem{
		&mediaprovider.Track{ID: "a", Duration: 4 * time.Minute},
		&mediaprovider.Track{ID: "b", Duration: 4 * time.Minute},
	})
	return e
}

// Switching players while paused defers loading the track until the user
// presses play, and reports the old player's paused position meanwhile.
// Starting anything else on the new player has to end that, or the engine
// keeps reporting the stale snapshot instead of what is actually playing.
func TestPlayerChangeWhilePausedThenPlayTrack(t *testing.T) {
	local := &fakeURLPlayer{status: player.Status{State: player.Paused, TimePos: 155}}
	e := newTestEngine(t, local)
	e.nowPlayingIdx = 0

	remote := &fakeURLPlayer{}
	if err := e.SetPlayer(remote); err != nil {
		t.Fatal(err)
	}
	if s := e.PlaybackStatus(); s.State != player.Paused || s.TimePos != 155 {
		t.Fatalf("before play: status %+v, want paused at 155", s)
	}

	if err := e.PlayTrackAt(1); err != nil {
		t.Fatal(err)
	}
	if len(remote.played) != 1 || remote.played[0] != "b" {
		t.Fatalf("remote player was handed %v, want [b]", remote.played)
	}
	remote.status.TimePos = 30
	if s := e.PlaybackStatus(); s.State != player.Playing || s.TimePos != 30 {
		t.Errorf("after play: status %+v, want the remote player's playing at 30", s)
	}
}

// The track restored paused at startup goes through the same deferral
// when the player is switched before it is first played.
func TestPlayerChangeWhileLoadedPausedThenContinue(t *testing.T) {
	local := &fakeURLPlayer{}
	e := newTestEngine(t, local)
	if err := e.loadTrackPaused(0, 155); err != nil {
		t.Fatal(err)
	}

	remote := &fakeURLPlayer{}
	if err := e.SetPlayer(remote); err != nil {
		t.Fatal(err)
	}
	if err := e.Continue(); err != nil {
		t.Fatal(err)
	}
	if len(remote.played) != 1 || remote.played[0] != "a" {
		t.Fatalf("remote player was handed %v, want [a]", remote.played)
	}
	remote.status.TimePos = 160
	if s := e.PlaybackStatus(); s.State != player.Playing || s.TimePos != 160 {
		t.Errorf("status %+v, want the remote player's playing at 160", s)
	}
}

func TestPlayerChangeWhilePausedThenStop(t *testing.T) {
	local := &fakeURLPlayer{status: player.Status{State: player.Paused, TimePos: 155}}
	e := newTestEngine(t, local)
	e.nowPlayingIdx = 0

	stopped := false
	e.onStopped = append(e.onStopped, func() { stopped = true })
	if err := e.SetPlayer(&fakeURLPlayer{}); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	if s := e.PlaybackStatus(); s.State != player.Stopped {
		t.Errorf("status %+v, want stopped", s)
	}
	if !stopped {
		t.Error("stopping did not notify listeners")
	}
	if e.NowPlaying() != nil {
		t.Error("a track is still reported as playing")
	}
}
