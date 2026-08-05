package jellyfin

import (
	"strings"
	"sync"

	"github.com/deluan/sanitize"
	"github.com/dweymouth/go-jellyfin"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/mediaprovider/helpers"
	"github.com/supersonic-app/supersonic/sharedutil"
)

func (j *JellyfinMediaProvider) SearchAll(searchQuery string, maxResults int) ([]*mediaprovider.SearchResult, error) {
	limit := maxResults / 3
	var wg sync.WaitGroup
	var albums []*jellyfin.Album
	var artists []*jellyfin.Artist
	var songs []*jellyfin.Song
	var genres []jellyfin.NameID
	var playlists []*jellyfin.Playlist

	var opts jellyfin.QueryOpts
	opts.Paging.Limit = limit
	opts.Filter.ParentID = j.currentLibraryID
	wg.Add(1)
	go func() {
		albumResult, _ := j.client.Search(searchQuery, jellyfin.TypeAlbum, opts)
		albums = albumResult.Albums
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		artistResult, _ := j.client.Search(searchQuery, jellyfin.TypeArtist, opts)
		artists = artistResult.Artists
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		songResult, _ := j.client.Search(searchQuery, jellyfin.TypeSong, opts)
		songs = songResult.Songs
		wg.Done()
	}()

	querySanitized := strings.ToLower(sanitize.Accents(searchQuery))
	queryLowerWords := strings.Fields(querySanitized)

	wg.Add(1)
	go func() {
		p, e := j.client.GetPlaylists()
		if e == nil {
			playlists = sharedutil.FilterSlice(p, func(p *jellyfin.Playlist) bool {
				return helpers.AllTermsMatch(strings.ToLower(sanitize.Accents(p.Name)), queryLowerWords)
			})
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		g, e := j.client.GetGenres(jellyfin.Paging{}, "")
		if e == nil {
			genres = sharedutil.FilterSlice(g, func(g jellyfin.NameID) bool {
				return helpers.AllTermsMatch(strings.ToLower(sanitize.Accents(g.Name)), queryLowerWords)
			})
		}
		wg.Done()
	}()

	wg.Wait()

	results := j.mergeResults(albums, artists, songs, playlists, genres)
	helpers.RankSearchResults(results, searchQuery, queryLowerWords)

	return results, nil
}

func (j *JellyfinMediaProvider) mergeResults(
	albums []*jellyfin.Album,
	artists []*jellyfin.Artist,
	songs []*jellyfin.Song,
	matchingPlaylists []*jellyfin.Playlist,
	matchingGenres []jellyfin.NameID,
) []*mediaprovider.SearchResult {
	var results []*mediaprovider.SearchResult

	getArtistNames := func(artist jellyfin.NameID) string {
		return artist.Name
	}

	for _, al := range albums {
		results = append(results, &mediaprovider.SearchResult{
			Type:       mediaprovider.ContentTypeAlbum,
			ID:         al.ID,
			CoverID:    al.ID,
			Name:       al.Name,
			ArtistName: strings.Join(sharedutil.MapSlice(al.Artists, getArtistNames), ","),
			Size:       al.ChildCount,
			Item:       toAlbum(al),
		})
	}

	for _, ar := range artists {
		results = append(results, &mediaprovider.SearchResult{
			Type:    mediaprovider.ContentTypeArtist,
			ID:      ar.ID,
			CoverID: ar.ID,
			Name:    ar.Name,
			Size:    ar.AlbumCount,
			Item:    toArtist(ar),
		})
	}

	for _, tr := range songs {
		results = append(results, &mediaprovider.SearchResult{
			Type:       mediaprovider.ContentTypeTrack,
			ID:         tr.Id,
			CoverID:    tr.Id,
			Name:       tr.Name,
			ArtistName: strings.Join(sharedutil.MapSlice(tr.Artists, getArtistNames), ","),
			Size:       int(tr.RunTimeTicks / 10_000_000),
			Item:       toTrack(tr),
		})
	}

	for _, pl := range matchingPlaylists {
		results = append(results, &mediaprovider.SearchResult{
			Type:    mediaprovider.ContentTypePlaylist,
			ID:      pl.ID,
			CoverID: pl.ID,
			Name:    pl.Name,
			Size:    pl.SongCount,
			Item:    j.toPlaylist(pl),
		})
	}

	for _, g := range matchingGenres {
		results = append(results, &mediaprovider.SearchResult{
			Type: mediaprovider.ContentTypeGenre,
			ID:   g.Name,
			Name: g.Name,
			Size: -1,
		})
	}

	return results
}
