package backend

import (
	"encoding/base32"
	"errors"
	"strconv"

	"github.com/godbus/dbus/v5"
	"github.com/quarckster/go-mpris-server/pkg/events"
	"github.com/quarckster/go-mpris-server/pkg/server"
	"github.com/quarckster/go-mpris-server/pkg/types"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/player"
)

const (
	dbusTrackIDPrefix = "/Supersonic/Track/"
	noTrackObjectPath = "/org/mpris/MediaPlayer2/TrackList/NoTrack"
)

var (
	_ types.OrgMprisMediaPlayer2Adapter                 = (*MPRISHandler)(nil)
	_ types.OrgMprisMediaPlayer2PlayerAdapter           = (*MPRISHandler)(nil)
	_ types.OrgMprisMediaPlayer2PlayerAdapterLoopStatus = (*MPRISHandler)(nil)
	_ types.OrgMprisMediaPlayer2PlayerAdapterShuffle    = (*MPRISHandler)(nil)
)

var errNotSupported = errors.New("not supported")

type MPRISHandler struct {
	// Function called if the player is requested to quit through MPRIS.
	// Should *asynchronously* start shutdown and return immediately true if a shutdown will happen.
	OnQuit func() error

	// Function called if the player is requested to bring its UI to the front.
	OnRaise func() error

	// Function to look up the artwork URL for a given track ID
	ArtURLLookup func(trackID string) (string, error)

	connErr      error
	playerName   string
	curTrackPath string // empty for no track
	pm           *PlaybackManager
	s            *server.Server
	evt          *events.EventHandler

	// current radio metadata
	radioStationName string
	radioIcyTitle    string
	radioIcyArtist   string
}

func NewMPRISHandler(playerName string, pm *PlaybackManager) *MPRISHandler {
	m := &MPRISHandler{playerName: playerName, pm: pm, connErr: errors.New("not started")}
	m.s = server.NewServer(playerName, m, m)
	m.evt = events.NewEventHandler(m.s)

	pm.OnSeek(func() {
		if m.connErr == nil {
			pos := secondsToMicroseconds(pm.PlaybackStatus().TimePos)
			m.evt.Player.OnSeek(pos)
		}
	})
	pm.OnSongChange(func(tr mediaprovider.MediaItem, _ *mediaprovider.Track) {
		if tr == nil {
			m.curTrackPath = ""
		} else {
			m.curTrackPath = dbusTrackIDPrefix + encodeTrackId(tr.Metadata().ID)
		}
		if m.connErr == nil {
			m.evt.Player.OnTitle()
		}
	})
	pm.OnRadioMetadataChange(func(radioName, title, artist string) {
		m.radioStationName = radioName
		m.radioIcyTitle = title
		m.radioIcyArtist = artist
		if m.connErr == nil {
			m.evt.Player.OnTitle()
		}
	})
	pm.OnVolumeChange(func(vol int) {
		if m.connErr == nil {
			m.evt.Player.OnVolume()
		}
	})
	pm.OnLoopModeChange(func(loopMode LoopMode) {
		if m.connErr == nil {
			m.evt.Player.OnOptions()
		}
	})
	emitPlayStatus := func() {
		if m.connErr == nil {
			m.evt.Player.OnPlayPause()
		}
	}
	m.pm.OnStopped(emitPlayStatus)
	m.pm.OnPlaying(emitPlayStatus)
	m.pm.OnPaused(emitPlayStatus)

	return m
}

// Starts listening for MPRIS events.
func (m *MPRISHandler) Start() {
	m.connErr = nil
	go func() {
		// exits early with err if unable to establish D-Bus connection
		m.connErr = m.s.Listen()
	}()
}

// Stops listening for MPRIS events and releases any D-Bus resources.
func (m *MPRISHandler) Shutdown() {
	if m.connErr == nil {
		m.s.Stop()
		m.connErr = errors.New("stopped")
	}
}

// OrgMprisMediaPlayer2Adapter implementation

func (m *MPRISHandler) Identity() (string, error) {
	return m.playerName, nil
}

func (m *MPRISHandler) CanQuit() (bool, error) {
	return m.OnQuit != nil, nil
}

func (m *MPRISHandler) Quit() error {
	if m.OnQuit != nil {
		return m.OnQuit()
	}
	return errors.New("no quit handler added")
}

func (m *MPRISHandler) CanRaise() (bool, error) {
	return m.OnRaise != nil, nil
}

func (m *MPRISHandler) Raise() error {
	if m.OnRaise != nil {
		return m.OnRaise()
	}
	return errors.New("no raise handler added")
}

func (m *MPRISHandler) HasTrackList() (bool, error) {
	return false, nil
}

func (m *MPRISHandler) SupportedUriSchemes() ([]string, error) {
	return nil, nil
}

func (m *MPRISHandler) SupportedMimeTypes() ([]string, error) {
	return nil, nil
}

// OrgMprisMediaPlayer2PlayerAdapter implementation

func (m *MPRISHandler) Next() error {
	m.pm.SeekNext()
	return nil
}

func (m *MPRISHandler) Previous() error {
	m.pm.SeekBackOrPrevious()
	return nil
}

func (m *MPRISHandler) Pause() error {
	if m.pm.PlaybackStatus().State == player.Playing {
		m.pm.PlayPause()
	}
	return nil
}

func (m *MPRISHandler) PlayPause() error {
	m.pm.PlayPause()
	return nil
}

func (m *MPRISHandler) Stop() error {
	m.pm.Stop()
	return nil
}

func (m *MPRISHandler) Play() error {
	switch m.pm.PlaybackStatus().State {
	case player.Paused:
		m.pm.PlayPause()
	case player.Stopped:
		m.pm.PlayFromBeginning()
	}
	return nil
}

func (m *MPRISHandler) Seek(offset types.Microseconds) error {
	// MPRIS seek command is relative to current position
	m.pm.SeekBySeconds(microsecondsToSeconds(offset))
	return nil
}

func (m *MPRISHandler) SetPosition(trackId string, position types.Microseconds) error {
	if m.curTrackPath == trackId {
		m.pm.SeekSeconds(microsecondsToSeconds(position))
	}
	return nil
}

func (m *MPRISHandler) OpenUri(uri string) error {
	return errNotSupported
}

func (m *MPRISHandler) PlaybackStatus() (types.PlaybackStatus, error) {
	switch m.pm.PlaybackStatus().State {
	case player.Playing:
		return types.PlaybackStatusPlaying, nil
	case player.Paused:
		return types.PlaybackStatusPaused, nil
	case player.Stopped:
		return types.PlaybackStatusStopped, nil
	}
	return "", errors.New("unknown playback status")
}

// OrgMprisMediaPlayer2PlayerAdapterLoopStatus implementation

func (m *MPRISHandler) LoopStatus() (types.LoopStatus, error) {
	switch m.pm.GetLoopMode() {
	case LoopAll:
		return types.LoopStatusPlaylist, nil
	case LoopOne:
		return types.LoopStatusTrack, nil
	case LoopNone:
		return types.LoopStatusNone, nil
	}
	return "", errors.New("unknown loop status")
}

func (m *MPRISHandler) SetLoopStatus(status types.LoopStatus) error {
	switch status {
	case types.LoopStatusPlaylist:
		m.pm.SetLoopMode(LoopAll)
	case types.LoopStatusTrack:
		m.pm.SetLoopMode(LoopOne)
	case types.LoopStatusNone:
		m.pm.SetLoopMode(LoopNone)
	default:
		return errors.New("unknown loop status")
	}
	return nil
}

// OrgMprisMediaPlayer2PlayerAdapterShuffle implementation

func (m *MPRISHandler) Shuffle() (bool, error) {
	return m.pm.IsShuffle(), nil
}

func (m *MPRISHandler) SetShuffle(shuffle bool) error {
	m.pm.SetShuffle(shuffle)
	return nil
}

func (m *MPRISHandler) Rate() (float64, error) {
	return 1, nil
}

func (m *MPRISHandler) SetRate(float64) error {
	return errNotSupported
}

func (m *MPRISHandler) Metadata() (types.Metadata, error) {
	trackObjPath := noTrackObjectPath
	if m.curTrackPath != "" {
		trackObjPath = m.curTrackPath
	}
	status := m.pm.PlaybackStatus()

	var meta mediaprovider.MediaItemMetadata
	// metadata that can come only from tracks
	var discNumber, trackNumber, userRating, playCount, year int
	var genres []string

	if np := m.pm.NowPlaying(); np != nil && status.State != player.Stopped {
		meta = np.Metadata()
		if track, ok := np.(*mediaprovider.Track); ok {
			discNumber = track.DiscNumber
			trackNumber = track.TrackNumber
			userRating = track.Rating
			playCount = track.PlayCount
			year = track.Year
			genres = track.Genres
		}
	}
	var artURL string
	if meta.ID != "" && m.ArtURLLookup != nil {
		if u, err := m.ArtURLLookup(meta.CoverArtID); err == nil {
			artURL = u
		}
	}

	title := meta.Name
	artists := meta.Artists
	album := meta.Album
	// if playing a radio station, override title/artist with current Icy metadata if present
	if m.radioStationName == title && m.radioIcyTitle != "" {
		title = m.radioIcyTitle
		artists = []string{m.radioIcyArtist}
		album = m.radioStationName
	}

	mprisMeta := types.Metadata{
		TrackId:     dbus.ObjectPath(trackObjPath),
		Length:      secondsToMicroseconds(status.Duration),
		Title:       title,
		Album:       album,
		Artist:      artists,
		DiscNumber:  discNumber,
		TrackNumber: trackNumber,
		UserRating:  float64(userRating) / 5,
		UseCount:    playCount,
		ArtUrl:      artURL,
		Genre:       genres,
	}
	if year != 0 {
		mprisMeta.ContentCreated = strconv.Itoa(year)
	}
	return mprisMeta, nil
}

func (m *MPRISHandler) Volume() (float64, error) {
	return float64(m.pm.Volume()) / 100, nil
}

func (m *MPRISHandler) SetVolume(v float64) error {
	m.pm.SetVolume(int(v * 100))
	return nil
}

func (m *MPRISHandler) Position() (int64, error) {
	return int64(secondsToMicroseconds(m.pm.PlaybackStatus().TimePos)), nil
}

func (m *MPRISHandler) MinimumRate() (float64, error) {
	return 1, nil
}

func (m *MPRISHandler) MaximumRate() (float64, error) {
	return 1, nil
}

func (m *MPRISHandler) CanGoNext() (bool, error) {
	return true, nil
}

func (m *MPRISHandler) CanGoPrevious() (bool, error) {
	return true, nil
}

func (m *MPRISHandler) CanPlay() (bool, error) {
	return true, nil
}

func (m *MPRISHandler) CanPause() (bool, error) {
	return true, nil
}

func (m *MPRISHandler) CanSeek() (bool, error) {
	return true, nil
}

func (m *MPRISHandler) CanControl() (bool, error) {
	return true, nil
}

func microsecondsToSeconds(m types.Microseconds) float64 {
	return float64(m) / 1_000_000
}

func secondsToMicroseconds(s float64) types.Microseconds {
	return types.Microseconds(s * 1_000_000)
}

func encodeTrackId(id string) string {
	data := []byte(id)
	return base32.StdEncoding.WithPadding('0').EncodeToString(data)
}
