package backend

import (
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
)

type ServerType string

const (
	ServerTypeSubsonic ServerType = "Subsonic"
	ServerTypeJellyfin ServerType = "Jellyfin"
)

type ServerConnection struct {
	ServerType    ServerType
	Hostname      string
	AltHostname   string
	Username      string
	LegacyAuth    bool
	SkipSSLVerify bool
}

type ServerConfig struct {
	ServerConnection
	ID              uuid.UUID
	Nickname        string
	Default         bool
	SelectedLibrary string
}

type AppConfig struct {
	WindowWidth                 int
	WindowHeight                int
	LastCheckedVersion          string
	LastLaunchedVersion         string
	EnableSystemTray            bool
	CloseToSystemTray           bool
	StartupPage                 string
	SettingsTab                 string
	AllowMultiInstance          bool
	MaxImageCacheSizeMB         int
	SavePlayQueue               bool
	SaveQueueToServer           bool
	DefaultPlaylistID           string
	AddToPlaylistSkipDuplicates bool
	ShowTrackChangeNotification bool
	EnableLrcLib                bool
	CustomLrcLibUrl             string
	EnablePasswordStorage       bool
	SkipSSLVerify               bool // Deprecated: use per-server SkipSSLVerify. Drop in future version.
	EnqueueBatchSize            int
	Language                    string
	DisableDPIDetection         bool
	EnableAutoUpdateChecker     bool
	RequestTimeoutSeconds       int
	EnableOSMediaPlayerAPIs     bool
	ShowSidebar                 bool
	SidebarWidthFraction        float64
	SidebarTab                  string
	HidePlayedQueueTracks       bool

	PreventScreensaverOnNowPlayingPage bool

	FontNormalTTF string
	FontBoldTTF   string
	UIScaleSize   string
}

type AlbumPageConfig struct {
	TracklistColumns []string
	CompactHeader    bool
}

// shared between Albums and Genre pages
type AlbumsPageConfig struct {
	SortOrder   string // only relevant for Albums page
	ShowYears   bool
	ShuffleMode string // only relevant for Genre page
	PlayInOrder bool   // only relevant for Albums page
}

type ArtistPageConfig struct {
	InitialView      string
	DiscographySort  string
	TracklistColumns []string
	CompactHeader    bool
}

type ArtistsPageConfig struct {
	SortOrder string
}

type FavoritesPageConfig struct {
	InitialView      string
	TracklistColumns []string
	ShowAlbumYears   bool
}

type GridViewConfig struct {
	CardSize float32
}

type PlaylistPageConfig struct {
	TracklistColumns []string
	CompactHeader    bool
}

type PlaylistsPageConfig struct {
	InitialView string
}

type TracksPageConfig struct {
	TracklistColumns []string
}

type NowPlayingPageConfig struct {
	InitialView        string
	UseBackgroundImage bool
}

type PlaybackConfig struct {
	Autoplay                 bool
	Shuffle                  bool
	RepeatMode               string
	SkipOneStarWhenShuffling bool
	SkipKeywordWhenShuffling string
	UseWaveformSeekbar       bool
}

type LocalPlaybackConfig struct {
	AudioDeviceName       string
	AudioExclusive        bool
	InMemoryCacheSizeMB   int
	Volume                int
	EqualizerEnabled      bool
	EqualizerType         string    // "ISO10Band" or "ISO15Band"
	EqualizerPreamp       float64
	GraphicEqualizerBands []float64
	ActiveEQPresetName    string // Name of currently selected EQ preset
	AutoEQProfilePath     string // Path to applied AutoEQ profile (e.g., "oratory1990/over-ear/Sennheiser HD 650")
	AutoEQProfileName     string // Display name of applied profile (e.g., "Sennheiser HD 650")
	PauseFade             bool
}

type ScrobbleConfig struct {
	Enabled              bool
	ThresholdTimeSeconds int
	ThresholdPercent     int
}

type ReplayGainConfig struct {
	Mode            string
	PreampGainDB    float64
	PreventClipping bool
}

type ThemeConfig struct {
	ThemeFile              string
	Appearance             string
	UseRoundedImageCorners bool
}

type TranscodingConfig struct {
	ForceRawFile     bool
	RequestTranscode bool
	Codec            string
	MaxBitRateKBPS   int
}

type PeakMeterConfig struct {
	WindowHeight int
	WindowWidth  int
}

type Config struct {
	Application      AppConfig
	Servers          []*ServerConfig
	AlbumPage        AlbumPageConfig
	AlbumsPage       AlbumsPageConfig
	ArtistPage       ArtistPageConfig
	ArtistsPage      ArtistsPageConfig
	FavoritesPage    FavoritesPageConfig
	GridView         GridViewConfig
	PlaylistPage     PlaylistPageConfig
	PlaylistsPage    PlaylistsPageConfig
	TracksPage       TracksPageConfig
	NowPlayingConfig NowPlayingPageConfig
	Playback         PlaybackConfig
	LocalPlayback    LocalPlaybackConfig
	Scrobbling       ScrobbleConfig
	ReplayGain       ReplayGainConfig
	Transcoding      TranscodingConfig
	Theme            ThemeConfig
	PeakMeter        PeakMeterConfig
}

var SupportedStartupPages = []string{"Albums", "Favorites", "Playlists", "Artists", "All Tracks"}

func DefaultConfig(appVersionTag string) *Config {
	return &Config{
		Application: AppConfig{
			WindowWidth:                        1000,
			WindowHeight:                       800,
			LastCheckedVersion:                 appVersionTag,
			LastLaunchedVersion:                "",
			EnableSystemTray:                   true,
			CloseToSystemTray:                  false,
			StartupPage:                        "Albums",
			SettingsTab:                        "General",
			AllowMultiInstance:                 false,
			MaxImageCacheSizeMB:                50,
			UIScaleSize:                        "Normal",
			SavePlayQueue:                      true,
			SaveQueueToServer:                  false,
			ShowTrackChangeNotification:        false,
			EnableLrcLib:                       true,
			EnablePasswordStorage:              true,
			EnqueueBatchSize:                   100,
			Language:                           "auto",
			EnableAutoUpdateChecker:            true,
			RequestTimeoutSeconds:              15,
			EnableOSMediaPlayerAPIs:            true,
			SidebarWidthFraction:               0.8,
			PreventScreensaverOnNowPlayingPage: false,
		},
		AlbumPage: AlbumPageConfig{
			TracklistColumns: []string{"Artist", "Time", "Plays", "Favorite", "Rating"},
		},
		AlbumsPage: AlbumsPageConfig{
			SortOrder:   string("Recently Added"),
			ShowYears:   false,
			ShuffleMode: "Tracks",
		},
		ArtistPage: ArtistPageConfig{
			InitialView:      "Discography",
			TracklistColumns: []string{"Album", "Time", "Plays", "Favorite", "Rating"},
		},
		ArtistsPage: ArtistsPageConfig{
			SortOrder: string("Name (A-Z)"),
		},
		FavoritesPage: FavoritesPageConfig{
			TracklistColumns: []string{"Album", "Time", "Plays"},
			InitialView:      "Albums",
			ShowAlbumYears:   false,
		},
		GridView: GridViewConfig{
			CardSize: 200,
		},
		PlaylistPage: PlaylistPageConfig{
			TracklistColumns: []string{"Album", "Time", "Plays"},
		},
		PlaylistsPage: PlaylistsPageConfig{
			InitialView: "List",
		},
		NowPlayingConfig: NowPlayingPageConfig{
			InitialView:        "Play Queue",
			UseBackgroundImage: true,
		},
		TracksPage: TracksPageConfig{
			TracklistColumns: []string{"Album", "Time", "Plays"},
		},
		Playback: PlaybackConfig{
			Autoplay:           false,
			Shuffle:            false,
			RepeatMode:         "None",
			UseWaveformSeekbar: false,
		},
		LocalPlayback: LocalPlaybackConfig{
			// "auto" is the name to pass to MPV for autoselecting the output device
			AudioDeviceName:       "auto",
			AudioExclusive:        false,
			InMemoryCacheSizeMB:   30,
			Volume:                100,
			EqualizerEnabled:      false,
			EqualizerType:         "ISO15Band",
			EqualizerPreamp:       0,
			GraphicEqualizerBands: make([]float64, 15),
			PauseFade:             true,
		},
		Scrobbling: ScrobbleConfig{
			Enabled:              true,
			ThresholdTimeSeconds: 240,
			ThresholdPercent:     50,
		},
		ReplayGain: ReplayGainConfig{
			Mode:            ReplayGainNone,
			PreampGainDB:    0.0,
			PreventClipping: true,
		},
		Transcoding: TranscodingConfig{
			ForceRawFile:     false,
			RequestTranscode: false,
			Codec:            "opus",
			MaxBitRateKBPS:   160,
		},
		Theme: ThemeConfig{
			Appearance:             "Dark",
			UseRoundedImageCorners: true,
		},
		PeakMeter: PeakMeterConfig{
			WindowWidth:  375,
			WindowHeight: 100,
		},
	}
}

func ReadConfigFile(filepath, appVersionTag string) (*Config, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := DefaultConfig(appVersionTag)
	if err := toml.NewDecoder(f).Decode(c); err != nil {
		return nil, err
	}
	c.migrateDeprecatedSettings()

	// Backfill Subsonic to empty ServerType fields
	// for updating configs created before multiple MediaProviders were added
	for _, s := range c.Servers {
		if s.ServerType == "" {
			s.ServerType = ServerTypeSubsonic
		}
	}

	return c, nil
}

var writeLock sync.Mutex

func (c *Config) WriteConfigFile(filepath string) error {
	if !writeLock.TryLock() {
		return nil // another write in progress
	}
	defer writeLock.Unlock()

	// clear deprecated global SkipSSLVerify after migrating to per-server settings
	c.Application.SkipSSLVerify = false
	b, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	os.WriteFile(filepath, b, 0o644)

	return nil
}

func (c *Config) migrateDeprecatedSettings() {
	// Migrate deprecated global SkipSSLVerify to per-server settings
	if c.Application.SkipSSLVerify {
		for _, s := range c.Servers {
			s.SkipSSLVerify = true
		}
	}
}
