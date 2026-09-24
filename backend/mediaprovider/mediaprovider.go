package mediaprovider

import (
	"image"
	"io"
	"net/url"
	"strings"

	"github.com/deluan/sanitize"
)

const (
	// set of all supported album sorts across all media providers
	// these strings may be translated
	AlbumSortRecentlyAdded    string = "Recently Added"
	AlbumSortRecentlyPlayed   string = "Recently Played"
	AlbumSortFrequentlyPlayed string = "Frequently Played"
	AlbumSortRandom           string = "Random"
	AlbumSortTitleAZ          string = "Title (A-Z)"
	AlbumSortArtistAZ         string = "Artist (A-Z)"
	AlbumSortYearAscending    string = "Year (ascending)"
	AlbumSortYearDescending   string = "Year (descending)"

	// set of all supported artist sorts across all media providers
	// these strings may be translated
	ArtistSortAlbumCount string = "Album Count"
	ArtistSortNameAZ     string = "Name (A-Z)"
	ArtistSortRandom     string = "Random"
)

type MediaIterator[M any] interface {
	Next() *M
}

type (
	ArtistIterator = MediaIterator[Artist]
	AlbumIterator  = MediaIterator[Album]
	TrackIterator  = MediaIterator[Track]
)

type MediaFilter[M, F any] interface {
	Options() F
	SetOptions(F)
	Clone() MediaFilter[M, F]
	IsNil() bool
	Matches(*M) bool
}

type AlbumFilter = MediaFilter[Album, AlbumFilterOptions]

type AlbumFilterOptions struct {
	MinYear int
	MaxYear int      // 0 == unset/match any
	Genres  []string // len(0) == unset/match any

	ExcludeFavorited   bool // mut. exc. with ExcludeUnfavorited
	ExcludeUnfavorited bool // mut. exc. with ExcludeFavorited
}

// Clone returns a deep copy of the filter options
func (o AlbumFilterOptions) Clone() AlbumFilterOptions {
	genres := make([]string, len(o.Genres))
	copy(genres, o.Genres)
	return AlbumFilterOptions{
		MinYear:            o.MinYear,
		MaxYear:            o.MaxYear,
		Genres:             genres,
		ExcludeFavorited:   o.ExcludeFavorited,
		ExcludeUnfavorited: o.ExcludeUnfavorited,
	}
}

type albumFilter struct {
	options AlbumFilterOptions
}

func NewAlbumFilter(options AlbumFilterOptions) *albumFilter {
	return &albumFilter{options}
}

func (a albumFilter) Options() AlbumFilterOptions {
	return a.options
}

func (a *albumFilter) SetOptions(options AlbumFilterOptions) {
	a.options = options
}

// Clone returns a deep copy of the filter
func (a albumFilter) Clone() AlbumFilter {
	return NewAlbumFilter(a.options.Clone())
}

// Returns true if the filter is the nil filter - i.e. matches everything
func (a albumFilter) IsNil() bool {
	return a.options.MinYear == 0 && a.options.MaxYear == 0 &&
		len(a.options.Genres) == 0 &&
		!a.options.ExcludeFavorited && !a.options.ExcludeUnfavorited
}

func (f albumFilter) Matches(album *Album) bool {
	if album == nil {
		return false
	}
	if f.options.ExcludeFavorited && album.Favorite {
		return false
	}
	if f.options.ExcludeUnfavorited && !album.Favorite {
		return false
	}
	if y := album.YearOrZero(); y < f.options.MinYear || (f.options.MaxYear > 0 && y > f.options.MaxYear) {
		return false
	}
	if len(f.options.Genres) == 0 {
		return true
	}
	return genresMatch(f.options.Genres, album.Genres)
}

type ArtistFilter = MediaFilter[Artist, ArtistFilterOptions]

type ArtistFilterOptions struct {
	SearchQuery string
}

// Clone returns a deep copy of the filter options
func (o ArtistFilterOptions) Clone() ArtistFilterOptions {
	return ArtistFilterOptions{
		SearchQuery: o.SearchQuery,
	}
}

type artistFilter struct {
	options ArtistFilterOptions
}

func NewArtistFilter(options ArtistFilterOptions) *artistFilter {
	return &artistFilter{options}
}

func (a artistFilter) Options() ArtistFilterOptions {
	return a.options
}

func (a *artistFilter) SetOptions(o ArtistFilterOptions) {
	a.options = o
}

// Clone returns a deep copy of the filter
func (a artistFilter) Clone() ArtistFilter {
	return NewArtistFilter(a.options.Clone())
}

// Returns true if the filter is the nil filter - i.e. matches everything
func (a artistFilter) IsNil() bool {
	return a.options.SearchQuery == ""
}

func (f artistFilter) Matches(artist *Artist) bool {
	if artist == nil {
		return false
	}
	if f.options.SearchQuery != "" && !strings.Contains(
		sanitize.Accents(strings.ToLower(artist.Name)),
		sanitize.Accents(strings.ToLower(f.options.SearchQuery)),
	) {
		return false
	}
	return true
}

type RatingFavoriteParameters struct {
	AlbumIDs  []string
	ArtistIDs []string
	TrackIDs  []string
}

type Favorites struct {
	Albums  []*Album
	Artists []*Artist
	Tracks  []*Track
}

type LoginResponse struct {
	Error       error
	IsAuthError bool
}

type TranscodeSettings struct {
	Codec       string
	BitRateKBPS int
}

type Server interface {
	Login(username, password string) LoginResponse
	MediaProvider() MediaProvider
}

type MediaProvider interface {
	// GetLibraries gets the list of top-level music libraries
	// (musicFolders in Subsonic)
	GetLibraries() ([]Library, error)

	// SetLibrary sets the current library that all other
	// MediaProvider API calls will filter to. Use empty string
	// to reset to all libraries.
	SetLibrary(id string) error

	GetTrack(trackID string) (*Track, error)

	GetAlbum(albumID string) (*AlbumWithTracks, error)

	GetAlbumInfo(albumID string) (*AlbumInfo, error)

	GetArtist(artistID string) (*ArtistWithAlbums, error)

	GetArtistTracks(artistID string) ([]*Track, error)

	GetArtistInfo(artistID string) (*ArtistInfo, error)

	GetPlaylist(playlistID string) (*PlaylistWithTracks, error)

	GetCoverArt(coverArtID string, size int) (image.Image, error)

	AlbumSortOrders() []string

	IterateAlbums(sortOrder string, filter AlbumFilter) AlbumIterator

	IterateTracks(searchQuery string) TrackIterator

	SearchAlbums(searchQuery string, filter AlbumFilter) AlbumIterator

	SearchAll(searchQuery string, maxResults int) ([]*SearchResult, error)

	GetRandomTracks(genre string, count int) ([]*Track, error)

	GetSimilarTracks(artistID string, count int) ([]*Track, error)

	GetSongRadio(trackID string, count int) ([]*Track, error)

	ArtistSortOrders() []string

	IterateArtists(sortOrder string, filter ArtistFilter) ArtistIterator

	SearchArtists(searchQuery string, filter ArtistFilter) ArtistIterator

	GetGenres() ([]*Genre, error)

	GetFavorites() (Favorites, error)

	GetStreamURL(trackID string, transcodeSettings *TranscodeSettings, forceRaw bool) (string, error)

	GetTopTracks(artist Artist, count int) ([]*Track, error)

	SetFavorite(params RatingFavoriteParameters, favorite bool) error

	GetPlaylists() ([]*Playlist, error)

	CreatePlaylistWithTracks(name string, trackIDs []string) error

	CanMakePublicPlaylist() bool

	CreatePlaylist(name, description string, public bool) error

	EditPlaylist(id, name, description string, public bool) error

	AddPlaylistTracks(id string, trackIDsToAdd []string) error

	RemovePlaylistTracks(id string, trackIdxsToRemove []int) error

	ReplacePlaylistTracks(id string, trackIDs []string) error

	DeletePlaylist(id string) error

	// True if the `submission` parameter to TrackEndedPlayback will be respected
	// If false, the begin playback scrobble registers a play count immediately
	// when TrackBeganPlayback is invoked.
	ClientDecidesScrobble() bool

	TrackBeganPlayback(trackID string) error

	TrackEndedPlayback(trackID string, positionSecs int, submission bool) error

	DownloadTrack(trackID string) (io.Reader, error)

	RescanLibrary() error
}

type SupportsRating interface {
	SetRating(params RatingFavoriteParameters, rating int) error
}

type SupportsSharing interface {
	CreateShareURL(id string) (*url.URL, error)
	CanShareArtists() bool
}

type CanSavePlayQueue interface {
	SavePlayQueue(trackIDs []string, currentTrackPos int, timeSeconds int) error
	GetPlayQueue() (*SavedPlayQueue, error)
}

type CanReportPlayback interface {
	ReportPlayback(trackID string, positionMs int64, state string) error
}

type LyricsProvider interface {
	GetLyrics(track *Track) (*Lyrics, error)
}

type RadioProvider interface {
	GetRadioStation(id string) (*RadioStation, error)
	GetRadioStations() ([]*RadioStation, error)
}

type JukeboxProvider interface {
	JukeboxStart() error
	JukeboxStop() error
	JukeboxSeek(idx, seconds int) error
	JukeboxClear() error
	JukeboxAdd(trackID string) error
	JukeboxRemove(idx int) error
	JukeboxGetStatus() (*JukeboxStatus, error)

	// Performs a Clear followed by an Add to set the queue
	// to contain a single track
	JukeboxSet(trackID string) error

	// Sets the volume of the jukebox player (0-100)
	JukeboxSetVolume(vol int) error
}

type JukeboxStatus struct {
	Volume          int
	CurrentTrack    int
	Playing         bool
	PositionSeconds float64
}

func genresMatch(filterGenres, albumGenres []string) bool {
	for _, g1 := range filterGenres {
		for _, g2 := range albumGenres {
			if strings.EqualFold(g1, g2) {
				return true
			}
		}
	}
	return false
}
