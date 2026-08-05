package player

import (
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

type URLPlayer interface {
	BasePlayer
	PlayFile(url string, metadata mediaprovider.MediaItemMetadata, startTime float64) error
	SetNextFile(url string, metadata mediaprovider.MediaItemMetadata) error
}

type TrackPlayer interface {
	BasePlayer
	PlayTrack(track *mediaprovider.Track, startTime float64) error
	SetNextTrack(track *mediaprovider.Track) error
}

type BasePlayer interface {
	Continue() error
	Pause() error
	Stop(force bool) error

	SeekSeconds(secs float64) error
	IsSeeking() bool

	SetVolume(int) error
	GetVolume() int

	GetStatus() Status

	Destroy()

	// Event API
	OnPaused(func())
	OnStopped(func())
	OnPlaying(func())
	OnSeek(func())
	OnTrackChange(func())
}

type ReplayGainPlayer interface {
	SetReplayGainOptions(ReplayGainOptions) error
}

// The playback state (Stopped, Paused, or Playing).
type State int

const (
	Stopped State = iota
	Paused
	Playing
)

// The current status of the player.
// Includes playback state, current time, total track time, and playlist position.
type Status struct {
	State    State
	TimePos  float64
	Duration float64
}

type ReplayGainMode int

const (
	ReplayGainNone ReplayGainMode = iota
	ReplayGainTrack
	ReplayGainAlbum
)

// Replay Gain options (argument to SetReplayGainOptions).
type ReplayGainOptions struct {
	Mode            ReplayGainMode
	PreampGain      float64
	PreventClipping bool
	// Fallback gain intentionally omitted
}

func (r ReplayGainMode) String() string {
	switch r {
	case ReplayGainTrack:
		return "track"
	case ReplayGainAlbum:
		return "album"
	default:
		return "no"
	}
}

type BasePlayerCallbackImpl struct {
	onPaused      func()
	onStopped     func()
	onPlaying     func()
	onSeek        func()
	onTrackChange func()
}

// Sets a callback which is invoked when the player transitions to the Paused state.
func (p *BasePlayerCallbackImpl) OnPaused(cb func()) {
	p.onPaused = cb
}

// Sets a callback which is invoked when the player transitions to the Stopped state.
func (p *BasePlayerCallbackImpl) OnStopped(cb func()) {
	p.onStopped = cb
}

// Sets a callback which is invoked when the player transitions to the Playing state.
func (p *BasePlayerCallbackImpl) OnPlaying(cb func()) {
	p.onPlaying = cb
}

// Registers a callback which is invoked whenever a seek event occurs.
func (p *BasePlayerCallbackImpl) OnSeek(cb func()) {
	p.onSeek = cb
}

// Registers a callback which is invoked when the currently playing track changes,
// or when playback begins at any time from the Stopped state.
// Callback is invoked with the index of the currently playing track (zero-based).
func (p *BasePlayerCallbackImpl) OnTrackChange(cb func()) {
	p.onTrackChange = cb
}

func (p *BasePlayerCallbackImpl) InvokeOnPaused() {
	if p.onPaused != nil {
		p.onPaused()
	}
}

func (p *BasePlayerCallbackImpl) InvokeOnPlaying() {
	if p.onPlaying != nil {
		p.onPlaying()
	}
}

func (p *BasePlayerCallbackImpl) InvokeOnStopped() {
	if p.onStopped != nil {
		p.onStopped()
	}
}

func (p *BasePlayerCallbackImpl) InvokeOnSeek() {
	if p.onSeek != nil {
		p.onSeek()
	}
}

func (p *BasePlayerCallbackImpl) InvokeOnTrackChange() {
	if p.onTrackChange != nil {
		p.onTrackChange()
	}
}
