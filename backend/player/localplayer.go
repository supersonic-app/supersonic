package player

// AudioDevice describes a system audio output device.
type AudioDevice struct {
	// Name is the identifier passed to SetAudioDevice.
	Name string
	// Description is the human-friendly label shown in the UI.
	Description string
}

// MediaInfo holds information about the currently playing stream.
type MediaInfo struct {
	// Codec name (e.g. "flac", "mp3", "aac").
	Codec string
	// Samplerate in Hz.
	Samplerate int
	// Number of audio channels.
	ChannelCount int
	// Bitrate in bits per second (instantaneous for VBR codecs).
	Bitrate int
}

// LocalPlayer is implemented by any local audio playback engine
// (currently mpv and localav).  All methods used by the UI and backend
// that go beyond the core BasePlayer / URLPlayer interfaces are listed here.
type LocalPlayer interface {
	URLPlayer
	ReplayGainPlayer

	SetEqualizer(Equalizer) error
	Equalizer() Equalizer

	SetAudioExclusive(bool)
	SetPauseFade(bool)

	ListAudioDevices() ([]AudioDevice, error)
	SetAudioDevice(string) error

	GetMediaInfo() (MediaInfo, error)

	SetPeaksEnabled(bool) error
	GetPeaks() (float64, float64, float64, float64)

	ObserveIcyRadioTitle(func(string))
	UnobserveIcyRadioTitle()
}
