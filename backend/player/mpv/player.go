package mpv

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/backend/player"
	"github.com/supersonic-app/go-mpv"
)

// Error returned by many Player functions if called before the player has not been initialized.
var ErrUnitialized error = errors.New("mpv player uninitialized")

// AudioDevice and MediaInfo are now in the parent player package.
// Type aliases are provided for backwards compatibility.
type (
	AudioDevice = player.AudioDevice
	MediaInfo   = player.MediaInfo
)

var _ player.LocalPlayer = (*Player)(nil)

// Player encapsulates the mpv instance and provides functions
// to control it and to check its status.
type Player struct {
	player.BasePlayerCallbackImpl

	mpv            *mpv.Mpv
	initialized    bool
	vol            int
	replayGainOpts player.ReplayGainOptions
	haveRGainOpts  bool
	audioExclusive bool
	status         player.Status
	seeking        bool
	curPlaylistPos int64
	lenPlaylist    int64
	prePausedState player.State
	clientName     string
	equalizer      Equalizer
	peaksEnabled   bool
	pauseFade      bool

	icyTitleCb func(string)

	fileLoadedLock sync.Mutex
	fileLoadedSig  *sync.Cond

	fadePauseCancel context.CancelFunc
	bgCancel        context.CancelFunc
}

// Returns a new player.
// Must call Init on the player before it is ready for playback.
func New() *Player {
	return NewWithClientName("")
}

// Same as New, but sets the application name that mpv
// reports to the system audio API.
func NewWithClientName(c string) *Player {
	p := &Player{
		vol:        -1, // use 100 in Init
		clientName: c,
	}
	p.fileLoadedSig = sync.NewCond(&p.fileLoadedLock)
	return p
}

// Initializes the Player and makes it ready for playback.
// Most Player functions will return ErrUnitialized if called before Init.
func (p *Player) Init(maxCacheMB int) error {
	if !p.initialized {
		m := mpv.Create()

		m.SetOptionString("idle", "yes")
		m.SetOptionString("video", "no")
		m.SetOptionString("audio-display", "no")
		m.SetOptionString("gapless-audio", "weak")
		m.SetOptionString("prefetch-playlist", "yes")
		m.SetOptionString("force-seekable", "yes")
		m.SetOptionString("terminal", "no")

		// limit in-memory cache size
		maxBackMB := maxCacheMB / 3
		maxForwardMB := maxBackMB + maxBackMB
		m.SetOptionString("demuxer-max-bytes", fmt.Sprintf("%dMiB", maxForwardMB))
		m.SetOptionString("demuxer-max-back-bytes", fmt.Sprintf("%dMiB", maxBackMB))

		if p.vol < 0 {
			p.vol = 100
		}
		m.SetOption("volume", mpv.FORMAT_INT64, p.vol)

		p.SetAudioExclusive(p.audioExclusive)
		if p.haveRGainOpts {
			p.SetReplayGainOptions(p.replayGainOpts)
		}

		if p.clientName != "" {
			m.SetOptionString("audio-client-name", p.clientName)
		}

		m.ObserveProperty(0, "metadata", mpv.FORMAT_NODE)

		if err := m.Initialize(); err != nil {
			return fmt.Errorf("error initializing mpv: %s", err.Error())
		}

		p.mpv = m
	}
	ctx, cancel := context.WithCancel(context.Background())
	go p.eventHandler(ctx)
	p.bgCancel = cancel
	p.initialized = true
	return nil
}

// Plays the specified file, clearing the previous play queue, if any.
func (p *Player) PlayFile(url string, _ mediaprovider.MediaItemMetadata, startTime float64) error {
	if !p.initialized {
		return ErrUnitialized
	}
	err := p.mpv.Command([]string{"loadfile", url, "replace"})
	if err != nil {
		return err
	}
	p.lenPlaylist = 1
	if p.status.State == player.Paused {
		err = p.Continue()
	} else {
		p.setState(player.Playing)
	}
	if startTime > 0 {
		p.fileLoadedLock.Lock()
		p.fileLoadedSig.Wait()
		p.fileLoadedLock.Unlock()
		p.SeekSeconds(startTime)
	}

	return err
}

// Stops playback and clears the play queue.
func (p *Player) Stop(_ bool) error {
	if !p.initialized {
		return ErrUnitialized
	}
	var err error
	if p.status.State == player.Stopped {
		err = p.mpv.Command([]string{"playlist-clear"})
	} else {
		if err = p.mpv.Command([]string{"stop"}); err == nil {
			// if player was paused, stop command actually doesn't clear pause state
			err = p.setPaused(false)
		}
	}
	if err == nil {
		p.lenPlaylist = 0
		p.setState(player.Stopped)
	}
	return err
}

func (p *Player) SetNextFile(url string, _ mediaprovider.MediaItemMetadata) error {
	if p.lenPlaylist > p.curPlaylistPos+1 {
		if err := p.mpv.Command([]string{"playlist-remove", strconv.Itoa(int(p.curPlaylistPos) + 1)}); err != nil {
			return err
		}
		p.lenPlaylist--
	}
	if url == "" {
		return nil
	}

	err := p.mpv.Command([]string{"loadfile", url, "append"})
	if err == nil {
		p.lenPlaylist++
	}
	return err
}

// Seeks within the currently playing track.
// See MPV seek command documentation for more details.
func (p *Player) SeekSeconds(secs float64) error {
	if !p.initialized {
		return ErrUnitialized
	}
	target := fmt.Sprintf("%0.1f", secs)
	p.seeking = true
	err := p.mpv.Command([]string{"seek", target, "absolute"})
	return err
}

// Sets the volume of the player (0-100).
// Unlike most Player functions, SetVolume can be called before Init,
// to set the initial volume of the player on startup.
func (p *Player) SetVolume(vol int) error {
	if vol > 100 {
		vol = 100
	} else if vol < 0 {
		vol = 0
	}
	if p.initialized {
		err := p.mpv.SetProperty("volume", mpv.FORMAT_INT64, vol)
		if err == nil {
			p.vol = vol
		}
		return err
	}
	p.vol = vol
	return nil
}

// Sets the ReplayGain options of the player.
// Unlike most Player functions, SetReplayGainOptions can be called
// before Init, to set the initial replaygain options of the player on startup.
func (p *Player) SetReplayGainOptions(options player.ReplayGainOptions) error {
	p.replayGainOpts = options
	p.haveRGainOpts = true
	mode := "no"
	switch options.Mode {
	case player.ReplayGainAlbum:
		mode = "album"
	case player.ReplayGainTrack:
		mode = "track"
	}

	if p.initialized {
		if err := p.mpv.SetPropertyString("replaygain", mode); err != nil {
			return err
		}
		if err := p.mpv.SetProperty("replaygain-preamp", mpv.FORMAT_DOUBLE, options.PreampGain); err != nil {
			return err
		}
		clip := "yes"
		if options.PreventClipping {
			clip = "no"
		}
		if err := p.mpv.SetPropertyString("replaygain-clip", clip); err != nil {
			return err
		}
	}
	return nil
}

// Sets the audio exclusive option of the player.
// Unlike most Player functions, SetAudioExclusive can be called
// before Init, to set the initial option of the player on startup.
func (p *Player) SetAudioExclusive(tf bool) {
	p.audioExclusive = tf
	if p.initialized {
		val := "no"
		if tf {
			val = "yes"
		}
		p.mpv.SetOptionString("audio-exclusive", val)
	}
}

func (p *Player) SetPauseFade(pauseFade bool) {
	p.pauseFade = pauseFade
}

// Gets the current volume of the player.
func (p *Player) GetVolume() int {
	return p.vol
}

// sets paused status and ensures that audio exlusive is false while paused
// (releases audio device to other players)
func (p *Player) setPaused(paused bool) error {
	if !paused && p.audioExclusive {
		if err := p.mpv.SetOptionString("audio-exclusive", "yes"); err != nil {
			return err
		}
	}
	err := p.mpv.SetProperty("pause", mpv.FORMAT_FLAG, paused)
	if err == nil && paused && p.audioExclusive {
		err = p.mpv.SetOptionString("audio-exclusive", "no")
	}
	return err
}

// Pause playback and update the player state
func (p *Player) Pause() error {
	if p.status.State != player.Playing {
		return nil
	}

	if p.pauseFade {
		p.prePausedState = p.status.State
		p.setState(player.Paused)

		v := p.vol
		ctx, cancel := context.WithCancel(context.Background())
		p.fadePauseCancel = cancel
		go func() {
			t := time.NewTicker(2 * time.Millisecond)
			for c := 0; c < 100; c++ {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					p.mpv.SetProperty("volume", mpv.FORMAT_INT64, int64(v*(100-c)/100))
				}
			}
			t.Stop()
			p.SetVolume(p.vol)
			p.setPaused(true)
		}()
		return nil
	} else {
		err := p.setPaused(true)
		if err == nil {
			p.prePausedState = p.status.State
			p.setState(player.Paused)
		}
		return err
	}
}

// Continue playback and update the player state
func (p *Player) Continue() error {
	if p.status.State == player.Paused {
		if p.fadePauseCancel != nil {
			p.fadePauseCancel()
			p.fadePauseCancel = nil
			p.SetVolume(p.vol)
		}
		err := p.setPaused(false)
		if err == nil {
			p.setState(p.prePausedState)
		}
		return err
	}

	return nil
}

func (p *Player) ForceRestartPlayback(isPaused bool) error {
	p.mpv.SetProperty("pause", mpv.FORMAT_FLAG, true)
	p.mpv.SetProperty("pause", mpv.FORMAT_FLAG, false)
	return p.mpv.SetProperty("pause", mpv.FORMAT_FLAG, isPaused)
}

// Get the current status of the player.
func (p *Player) GetStatus() player.Status {
	if !p.initialized {
		return p.status
	}

	pos, _ := p.mpv.GetProperty("playback-time", mpv.FORMAT_DOUBLE)
	dur, _ := p.mpv.GetProperty("duration", mpv.FORMAT_DOUBLE)
	if pos != nil {
		p.status.TimePos = pos.(float64)
	}
	if dur != nil {
		p.status.Duration = dur.(float64)
	}
	return p.status
}

// List available audio devices.
func (p *Player) ListAudioDevices() ([]AudioDevice, error) {
	n, err := p.mpv.GetProperty("audio-device-list", mpv.FORMAT_NODE)
	if err != nil {
		return nil, err
	}
	nodeArr := n.(*mpv.Node).Data.([]*mpv.Node)

	devices := make([]AudioDevice, len(nodeArr))
	for i, node := range nodeArr {
		dev := node.Data.(map[string]*mpv.Node)
		name := dev["name"].Data.(string)
		desc := dev["description"].Data.(string)
		devices[i] = AudioDevice{Name: name, Description: desc}
	}
	return devices, nil
}

func (p *Player) SetAudioDevice(deviceName string) error {
	return p.mpv.SetPropertyString("audio-device", deviceName)
}

func (p *Player) SetEqualizer(eq Equalizer) error {
	p.equalizer = eq
	return p.setAF()
}

func (p *Player) Equalizer() Equalizer {
	return p.equalizer
}

func (p *Player) GetMediaInfo() (MediaInfo, error) {
	var info MediaInfo
	n, err := p.mpv.GetProperty("audio-params", mpv.FORMAT_NODE)
	if err != nil {
		return info, err
	}
	nodeMap := n.(*mpv.Node).Data.(map[string]*mpv.Node)
	info.Samplerate = int(nodeMap["samplerate"].Data.(int64))
	info.ChannelCount = int(nodeMap["channel-count"].Data.(int64))

	br, err := p.mpv.GetProperty("audio-bitrate", mpv.FORMAT_INT64)
	if err == nil {
		info.Bitrate = int(br.(int64))
	}
	codec, err := p.mpv.GetProperty("track-list/0/codec", mpv.FORMAT_STRING)
	if err == nil {
		info.Codec = codec.(string)
	}

	return info, nil
}

func (p *Player) ObserveIcyRadioTitle(cb func(string)) {
	p.icyTitleCb = cb
	p.mpv.ObserveProperty(1, "metadata/icy-title", mpv.FORMAT_STRING)
}

func (p *Player) UnobserveIcyRadioTitle() {
	p.icyTitleCb = nil
	p.mpv.UnobserveProperty(1)
}

func (p *Player) getInt64Property(propName string) (int64, error) {
	playpos, err := p.mpv.GetProperty(propName, mpv.FORMAT_INT64)
	if err != nil {
		return -1, err
	}
	if playpos != nil {
		return playpos.(int64), nil
	}
	return -1, errors.New("mpv did not report playlist pos")
}

// Returns true if a seek is currently in progress.
func (p *Player) IsSeeking() bool {
	return p.seeking && p.status.State == player.Playing
}

// Destroy the player.
func (p *Player) Destroy() {
	if p.bgCancel != nil {
		p.bgCancel()
	}
	if p.initialized {
		p.mpv.Command([]string{"stop"})
		p.mpv.TerminateDestroy()
		p.initialized = false
	}
}

func (p *Player) SetPeaksEnabled(enabled bool) error {
	if p.peaksEnabled == enabled {
		return nil
	}
	p.peaksEnabled = enabled
	return p.setAF()
}

func (p *Player) GetPeaks() (float64, float64, float64, float64) {
	nInf := math.Inf(-1)
	if p.status.State != player.Playing {
		return nInf, nInf, nInf, nInf
	}
	lPeak, rPeak, lRMS, rRMS, err := p.getPeaks()
	if err != nil {
		return nInf, nInf, nInf, nInf
	}
	return lPeak, rPeak, lRMS, rRMS
}

// sets the state and invokes callbacks, if triggered
func (p *Player) setState(s player.State) {
	switch {
	case s == player.Playing && p.status.State != player.Playing:
		defer p.InvokeOnPlaying()
	case s == player.Paused && p.status.State != player.Paused:
		defer p.InvokeOnPaused()
	case s == player.Stopped && p.status.State != player.Stopped:
		defer p.InvokeOnStopped()
	}
	p.status.State = s
}

func (p *Player) setAF() error {
	var filters []string
	if p.peaksEnabled {
		filters = append(filters, "@astats:astats=metadata=1:reset=1:measure_overall=none")
	}
	if eq := p.equalizer; eq != nil && eq.IsEnabled() {
		if math.Abs(eq.Preamp()) > 0.01 {
			filters = append(filters, fmt.Sprintf("volume=volume=%0.1fdB", eq.Preamp()))
		}
		if eqAF := eq.Curve().String(); eqAF != "" {
			filters = append(filters, eqAF)
		}
	}
	return p.mpv.SetPropertyString("af", strings.Join(filters, ","))
}

func (p *Player) eventHandler(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			e := p.mpv.WaitEvent(1 /*timeout seconds*/)
			if e.Event_Id != mpv.EVENT_NONE {
				// log.Printf("mpv event: %+v\n", e)
			}
			switch e.Event_Id {
			case mpv.EVENT_PLAYBACK_RESTART:
				fallthrough
			case mpv.EVENT_SEEK:
				p.seeking = false
				p.InvokeOnSeek()
			case mpv.EVENT_FILE_LOADED:
				p.curPlaylistPos, _ = p.getInt64Property("playlist-pos")
				if p.status.State == player.Paused {
					// seek while paused switches to a new file
					// mpv does not fire seek event in this case
					p.InvokeOnSeek()
				}
				p.InvokeOnTrackChange()
				p.fileLoadedSig.Signal()
			case mpv.EVENT_IDLE:
				p.status.Duration = 0
				p.status.TimePos = 0
				p.setState(player.Stopped)
			case mpv.EVENT_PROPERTY_CHANGE:
				if e.Reply_Userdata == 1 && p.icyTitleCb != nil {
					p.icyTitleCb(p.mpv.GetPropertyString("metadata/icy-title"))
				}

			}
		}
	}
}
