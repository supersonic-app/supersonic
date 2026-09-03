package jellyfin

import (
	"image"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/dweymouth/go-jellyfin"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/mediaprovider/helpers"
	"github.com/supersonic-app/supersonic/sharedutil"
)

const (
	cacheValidDurationSeconds  = 60
	runTimeTicksPerMicrosecond = 10
)

type JellyfinServer struct {
	jellyfin.Client
}

func (j *JellyfinServer) Login(user, pass string) mediaprovider.LoginResponse {
	if _, err := j.Ping(); err != nil {
		return mediaprovider.LoginResponse{Error: err}
	}
	err := j.Client.Login(user, pass)
	return mediaprovider.LoginResponse{
		Error:       err,
		IsAuthError: err != nil,
	}
}

func (j *JellyfinServer) MediaProvider() mediaprovider.MediaProvider {
	return newJellyfinMediaProvider(&j.Client)
}

var _ mediaprovider.MediaProvider = (*JellyfinMediaProvider)(nil)

type JellyfinMediaProvider struct {
	client          *jellyfin.Client
	prefetchCoverCB func(coverArtID string)

	currentLibraryID string

	genresCached   []*mediaprovider.Genre
	genresCachedAt int64 // unix
}

func newJellyfinMediaProvider(cli *jellyfin.Client) mediaprovider.MediaProvider {
	return &JellyfinMediaProvider{
		client:       cli,
		genresCached: make([]*mediaprovider.Genre, 0),
	}
}

func (j *JellyfinMediaProvider) SetPrefetchCoverCallback(cb func(coverArtID string)) {
	j.prefetchCoverCB = cb
}

func (j *JellyfinMediaProvider) GetLibraries() ([]mediaprovider.Library, error) {
	v, err := j.client.GetUserViews()
	if err != nil {
		return nil, err
	}
	return sharedutil.FilterMapSlice(v, func(v *jellyfin.BaseItem) (mediaprovider.Library, bool) {
		return mediaprovider.Library{Name: v.Name, ID: v.ID}, v.CollectionType == string(jellyfin.CollectionTypeMusic)
	}), nil
}

func (j *JellyfinMediaProvider) SetLibrary(id string) error {
	j.currentLibraryID = id
	j.genresCached = nil
	return nil
}

func (j *JellyfinMediaProvider) CreatePlaylistWithTracks(name string, trackIDs []string) error {
	return j.client.CreatePlaylist(name, "", false, trackIDs)
}

func (j *JellyfinMediaProvider) DeletePlaylist(id string) error {
	return j.client.DeletePlaylist(id)
}

func (j *JellyfinMediaProvider) CanMakePublicPlaylist() bool {
	return false
}

func (j *JellyfinMediaProvider) EditPlaylist(id, name, description string, public bool) error {
	return j.client.UpdatePlaylistMetadata(id, name, description, false)
}

func (j *JellyfinMediaProvider) CreatePlaylist(name, description string, public bool) error {
	return j.client.CreatePlaylist(name, description, public, nil)
}

func (j *JellyfinMediaProvider) AddPlaylistTracks(id string, trackIDsToAdd []string) error {
	return j.client.AddSongsToPlaylist(id, trackIDsToAdd)
}

func (j *JellyfinMediaProvider) RemovePlaylistTracks(playlistID string, removeIdxs []int) error {
	return j.client.RemoveSongsFromPlaylist(playlistID, removeIdxs)
}

func (j *JellyfinMediaProvider) ReplacePlaylistTracks(playlistID string, trackIDs []string) error {
	pl, err := j.client.GetPlaylist(playlistID)
	if err != nil {
		return err
	}
	allIndexes := make([]int, pl.SongCount)
	for i := range allIndexes {
		allIndexes[i] = i
	}
	if err = j.client.RemoveSongsFromPlaylist(playlistID, allIndexes); err != nil {
		return err
	}
	return j.client.AddSongsToPlaylist(playlistID, trackIDs)
}

func (j *JellyfinMediaProvider) GetAlbum(albumID string) (*mediaprovider.AlbumWithTracks, error) {
	al, err := j.client.GetAlbum(albumID)
	if err != nil {
		return nil, err
	}
	var opts jellyfin.QueryOpts
	opts.Filter.ParentID = albumID
	tr, err := j.client.GetSongs(opts)
	if err != nil {
		return nil, err
	}

	album := &mediaprovider.AlbumWithTracks{}
	fillAlbum(al, &album.Album)
	album.Tracks = sharedutil.MapSlice(tr, toTrack)
	return album, nil
}

func (j *JellyfinMediaProvider) GetAlbumInfo(albumID string) (*mediaprovider.AlbumInfo, error) {
	al, err := j.client.GetAlbum(albumID)
	if err != nil {
		return nil, err
	}
	return &mediaprovider.AlbumInfo{
		Notes: al.Overview,
	}, nil
}

func (j *JellyfinMediaProvider) GetArtist(artistID string) (*mediaprovider.ArtistWithAlbums, error) {
	ar, err := j.client.GetArtist(artistID)
	if err != nil {
		return nil, err
	}
	var opts jellyfin.QueryOpts
	opts.Filter.ArtistID = artistID
	al, err := j.client.GetAlbums(opts)
	if err != nil {
		return nil, err
	}

	artist := &mediaprovider.ArtistWithAlbums{
		Albums: sharedutil.MapSlice(al, toAlbum),
	}
	fillArtist(ar, &artist.Artist)
	return artist, nil
}

func (j *JellyfinMediaProvider) GetArtistTracks(artistID string) ([]*mediaprovider.Track, error) {
	return helpers.GetArtistTracks(j, artistID)
}

func (j *JellyfinMediaProvider) GetArtistInfo(artistID string) (*mediaprovider.ArtistInfo, error) {
	ar, err := j.client.GetArtist(artistID)
	if err != nil {
		return nil, err
	}
	similar, err := j.client.GetSimilarArtists(artistID)
	if err != nil {
		return nil, err
	}
	return &mediaprovider.ArtistInfo{
		SimilarArtists: sharedutil.MapSlice(similar, toArtist),
		Biography:      ar.Overview,
	}, nil
}

func (j *JellyfinMediaProvider) GetTrack(trackID string) (*mediaprovider.Track, error) {
	tr, err := j.client.GetSong(trackID)
	if err != nil {
		return nil, err
	}
	return toTrack(tr), nil
}

func (j *JellyfinMediaProvider) GetTopTracks(artist mediaprovider.Artist, limit int) ([]*mediaprovider.Track, error) {
	var opts jellyfin.QueryOpts
	opts.Paging.Limit = limit
	opts.Filter.ArtistID = artist.ID
	opts.Sort.Field = jellyfin.SortByCommunityRating
	opts.Sort.Mode = jellyfin.SortDesc
	if j.currentLibraryID != "" {
		opts.Filter.ParentID = j.currentLibraryID
	}
	tr, err := j.client.GetSongs(opts)
	if err != nil {
		return nil, err
	}
	if len(tr) == 0 {
		return helpers.GetTopTracksFallback(j, artist.ID, limit)
	}
	return sharedutil.MapSlice(tr, toTrack), nil
}

func (j *JellyfinMediaProvider) GetRandomTracks(genreName string, limit int) ([]*mediaprovider.Track, error) {
	var opts jellyfin.QueryOpts
	opts.Paging.Limit = limit
	opts.Filter.Genres = []string{genreName}
	opts.Sort.Field = jellyfin.SortByRandom
	if j.currentLibraryID != "" {
		opts.Filter.ParentID = j.currentLibraryID
	}
	tr, err := j.client.GetSongs(opts)
	if err != nil {
		return nil, err
	}
	return sharedutil.MapSlice(tr, toTrack), nil
}

func (j *JellyfinMediaProvider) GetSimilarTracks(artistID string, limit int) ([]*mediaprovider.Track, error) {
	tr, err := j.client.GetInstantMix(artistID, jellyfin.TypeArtist, limit)
	if err != nil {
		return nil, err
	}
	return sharedutil.MapSlice(tr, toTrack), nil
}

func (j *JellyfinMediaProvider) GetCoverArt(id string, size int) (image.Image, error) {
	return j.client.GetItemImage(id, "Primary", size, 92)
}

func (j *JellyfinMediaProvider) GetFavorites() (mediaprovider.Favorites, error) {
	var wg sync.WaitGroup
	var favorites mediaprovider.Favorites

	var opts jellyfin.QueryOpts
	opts.Filter.Favorite = true
	opts.Filter.ParentID = j.currentLibraryID
	wg.Add(1)
	go func() {
		al, err := j.client.GetAlbums(opts)
		if err == nil && len(al) > 0 {
			favorites.Albums = sharedutil.MapSlice(al, toAlbum)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		ar, err := j.client.GetAlbumArtists(opts)
		if err == nil && len(ar) > 0 {
			favorites.Artists = sharedutil.MapSlice(ar, toArtist)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		tr, err := j.client.GetSongs(opts)
		if err == nil && len(tr) > 0 {
			favorites.Tracks = sharedutil.MapSlice(tr, toTrack)
		}
		wg.Done()
	}()

	wg.Wait()
	return favorites, nil
}

func (j *JellyfinMediaProvider) GetGenres() ([]*mediaprovider.Genre, error) {
	if j.genresCached != nil && time.Now().Unix()-j.genresCachedAt < cacheValidDurationSeconds {
		return j.genresCached, nil
	}

	g, err := j.client.GetGenres(jellyfin.Paging{}, j.currentLibraryID)
	if err != nil {
		return nil, err
	}
	j.genresCached = sharedutil.MapSlice(g, func(g jellyfin.NameID) *mediaprovider.Genre {
		return &mediaprovider.Genre{
			Name:       g.Name,
			AlbumCount: -1, // unsupported by Jellyfin
			TrackCount: -1, // unsupported by Jellyfin
		}
	})
	j.genresCachedAt = time.Now().Unix()
	return j.genresCached, nil
}

func (j *JellyfinMediaProvider) GetPlaylists() ([]*mediaprovider.Playlist, error) {
	pl, err := j.client.GetPlaylists()
	if err != nil {
		return nil, err
	}
	return sharedutil.MapSlice(pl, j.toPlaylist), nil
}

func (j *JellyfinMediaProvider) GetPlaylist(playlistID string) (*mediaprovider.PlaylistWithTracks, error) {
	tr, err := j.client.GetPlaylistSongs(playlistID)
	if err != nil {
		return nil, err
	}
	pl, err := j.client.GetPlaylist(playlistID)
	if err != nil {
		return nil, err
	}

	playlist := &mediaprovider.PlaylistWithTracks{
		Tracks: sharedutil.MapSlice(tr, toTrack),
	}
	j.fillPlaylist(pl, &playlist.Playlist)
	return playlist, nil
}

func (j *JellyfinMediaProvider) SetFavorite(params mediaprovider.RatingFavoriteParameters, favorite bool) error {
	var allIDs []string
	allIDs = append(allIDs, params.AlbumIDs...)
	allIDs = append(allIDs, params.ArtistIDs...)
	allIDs = append(allIDs, params.TrackIDs...)

	// Jellyfin doesn't allow bulk setting favorites.
	// To not overwhelm the server with requests, set favorite for
	// only 5 items at a time concurrently
	batchSize := 5
	var err error
	batchSetFavorite := func(offs int, wg *sync.WaitGroup) {
		for i := 0; i < batchSize && offs+i < len(allIDs); i++ {
			wg.Add(1)
			go func(idx int) {
				newErr := j.client.SetFavorite(allIDs[idx], favorite)
				if err == nil && newErr != nil {
					err = newErr
				}
				wg.Done()
			}(offs + i)
		}
	}

	numBatches := int(math.Ceil(float64(len(allIDs)) / float64(batchSize)))
	for i := range numBatches {
		var wg sync.WaitGroup
		batchSetFavorite(i*batchSize, &wg)
		wg.Wait()
	}

	return err
}

func (j *JellyfinMediaProvider) GetStreamURL(trackID string, transcode *mediaprovider.TranscodeSettings, forceRaw bool) (string, error) {
	var jfTranscode *jellyfin.TranscodeOptions
	if transcode != nil {
		jfTranscode = &jellyfin.TranscodeOptions{
			// we only support opus and mp3 transcoding codecs right now,
			// both are accepted as a container format too
			Container:    transcode.Codec,
			AudioCodec:   transcode.Codec,
			AudioBitRate: uint32(transcode.BitRateKBPS * 1000),
		}
	}
	return j.client.GetStreamURL(trackID, jfTranscode)
}

// GetHLSStreamURL returns the HLS M3U8 manifest URL for a track (OpenSubsonic extension)
// Jellyfin doesn't support this endpoint natively, so we return empty string
func (j *JellyfinMediaProvider) GetHLSStreamURL(trackID string) (string, error) {
	return "", nil
}

func (j *JellyfinMediaProvider) DownloadTrack(trackID string) (io.Reader, error) {
	url, err := j.client.GetStreamURL(trackID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (j *JellyfinMediaProvider) ClientDecidesScrobble() bool { return false }

func (j *JellyfinMediaProvider) TrackBeganPlayback(trackID string) error {
	return j.client.UpdatePlayStatus(trackID, jellyfin.Start, 0)
}

func (j *JellyfinMediaProvider) TrackEndedPlayback(trackID string, position int, submission bool) error {
	return j.client.UpdatePlayStatus(trackID, jellyfin.Stop, int64(position)*runTimeTicksPerMicrosecond*1_000_000)
}

func (j *JellyfinMediaProvider) RescanLibrary() error {
	return j.client.RefreshLibrary()
}

var _ mediaprovider.LyricsProvider = (*JellyfinMediaProvider)(nil)

func (j *JellyfinMediaProvider) GetLyrics(tr *mediaprovider.Track) (*mediaprovider.Lyrics, error) {
	l, err := j.client.GetLyrics(tr.ID)
	if err != nil {
		return nil, err
	}
	return &mediaprovider.Lyrics{
		Title:  l.Metadata.Title,
		Artist: l.Metadata.Artist,
		Synced: l.Metadata.IsSynced || (len(l.Lyrics) > 0 && l.Lyrics[len(l.Lyrics)-1].Start > 0),
		Lines:  sharedutil.MapSlice(l.Lyrics, toLyricLine),
	}, nil
}

func toLyricLine(ll jellyfin.LyricLine) mediaprovider.LyricLine {
	return mediaprovider.LyricLine{
		Text:  ll.Text,
		Start: (time.Duration(ll.Start/runTimeTicksPerMicrosecond) * time.Microsecond).Seconds(),
	}
}

func toTrack(ch *jellyfin.Song) *mediaprovider.Track {
	if ch == nil {
		return nil
	}
	var artistNames, artistIDs []string
	for _, a := range ch.Artists {
		artistIDs = append(artistIDs, a.ID)
		artistNames = append(artistNames, a.Name)
	}

	coverArtID := ch.AlbumID
	if ch.ImageTags.Primary != "" {
		coverArtID = ch.Id
	}

	lastPlayed, _ := time.Parse(time.RFC3339Nano, ch.UserData.LastPlayedDate)
	dateAdded, _ := time.Parse(time.RFC3339Nano, ch.DateCreated)

	t := &mediaprovider.Track{
		ID:          ch.Id,
		CoverArtID:  coverArtID,
		ParentID:    ch.AlbumID,
		Title:       ch.Name,
		Duration:    time.Duration(ch.RunTimeTicks/runTimeTicksPerMicrosecond) * time.Microsecond,
		TrackNumber: ch.IndexNumber,
		DiscNumber:  ch.DiscNumber,
		// Genre:       ch.Genres,
		ArtistIDs:   artistIDs,
		ArtistNames: artistNames,
		Album:       ch.Album,
		AlbumID:     ch.AlbumID,
		Year:        ch.ProductionYear,
		Rating:      ch.UserData.Rating,
		Favorite:    ch.UserData.IsFavorite,
		PlayCount:   ch.UserData.PlayCount,
		LastPlayed:  lastPlayed,
		DateAdded:   dateAdded,
	}
	if len(ch.MediaSources) > 0 {
		t.FilePath = ch.MediaSources[0].Path
		t.Size = int64(ch.MediaSources[0].Size)
		t.BitRate = ch.MediaSources[0].Bitrate / 1000
		if strs := ch.MediaSources[0].MediaStreams; len(strs) > 0 {
			t.SampleRate = strs[0].SampleRate
			t.BitDepth = strs[0].BitDepth
			t.Channels = strs[0].Channels
		}
	}
	if len(ch.MediaStreams) > 0 {
		t.SampleRate = max(t.SampleRate, ch.MediaStreams[0].SampleRate)
		t.BitDepth = max(t.BitDepth, ch.MediaStreams[0].BitDepth)
		t.Channels = max(t.Channels, ch.MediaStreams[0].Channels)
	}
	return t
}

func toArtist(a *jellyfin.Artist) *mediaprovider.Artist {
	art := &mediaprovider.Artist{}
	fillArtist(a, art)
	return art
}

func fillArtist(a *jellyfin.Artist, artist *mediaprovider.Artist) {
	artist.AlbumCount = a.AlbumCount
	artist.Favorite = a.UserData.IsFavorite
	artist.ID = a.ID
	artist.Name = a.Name
	artist.CoverArtID = a.ID
}

func toAlbum(a *jellyfin.Album) *mediaprovider.Album {
	album := &mediaprovider.Album{}
	fillAlbum(a, album)
	return album
}

func fillAlbum(a *jellyfin.Album, album *mediaprovider.Album) {
	var artistNames, artistIDs []string
	for _, a := range a.Artists {
		artistIDs = append(artistIDs, a.ID)
		artistNames = append(artistNames, a.Name)
	}

	album.ID = a.ID
	album.CoverArtID = a.ID
	album.Name = a.Name
	album.Duration = time.Duration(a.RunTimeTicks/runTimeTicksPerMicrosecond) * time.Microsecond
	album.ArtistIDs = artistIDs
	album.ArtistNames = artistNames
	album.Date.Year = &a.Year
	album.TrackCount = a.ChildCount
	album.Genres = a.Genres
	album.Favorite = a.UserData.IsFavorite
	album.ReleaseTypes = mediaprovider.ReleaseTypeAlbum
}

func (j *JellyfinMediaProvider) toPlaylist(p *jellyfin.Playlist) *mediaprovider.Playlist {
	pl := &mediaprovider.Playlist{}
	j.fillPlaylist(p, pl)
	return pl
}

func (j *JellyfinMediaProvider) fillPlaylist(p *jellyfin.Playlist, pl *mediaprovider.Playlist) {
	pl.Name = p.Name
	pl.ID = p.ID
	pl.CoverArtID = p.ID
	pl.Description = p.Overview
	pl.TrackCount = p.SongCount
	pl.Duration = time.Duration(p.RunTimeTicks/runTimeTicksPerMicrosecond) * time.Microsecond
	// Jellyfin does not have public playlists
	pl.Owner = j.client.LoggedInUser()
	pl.Public = false
}

func (j *JellyfinMediaProvider) GetSongRadio(trackID string, count int) ([]*mediaprovider.Track, error) {
	tr, err := j.client.GetInstantMix(trackID, jellyfin.TypeSong, count)
	if err != nil {
		return nil, err
	}
	if len(tr) == 0 {
		track, err := j.GetTrack(trackID)
		if err != nil {
			return nil, err
		}
		return helpers.GetSimilarSongsFallback(j, track, count), nil
	}
	return sharedutil.MapSlice(tr, toTrack), nil
}
