package subsonic

import (
	"strconv"
	"strings"
	"sync"

	"github.com/deluan/sanitize"
	"github.com/supersonic-app/go-subsonic/subsonic"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/mediaprovider/helpers"
	"github.com/supersonic-app/supersonic/sharedutil"
)

func (s *subsonicMediaProvider) SearchAll(searchQuery string, maxResults int) ([]*mediaprovider.SearchResult, error) {
	var wg sync.WaitGroup
	var err error // only set by Search3
	var result *subsonic.SearchResult3
	var playlists []*subsonic.Playlist
	var genres []*subsonic.Genre
	var radios []*mediaprovider.RadioStation

	wg.Add(1)
	go func() {
		count := strconv.Itoa(maxResults / 3)
		params := map[string]string{
			"artistCount": count,
			"albumCount":  count,
			"songCount":   count,
		}
		if s.currentLibraryID != "" {
			params["musicFolderId"] = s.currentLibraryID
		}
		res, e := s.client.Search3(searchQuery, params)
		if e != nil {
			err = e
		} else {
			result = res
		}
		wg.Done()
	}()

	querySanitized := strings.ToLower(sanitize.Accents(searchQuery))
	queryLowerWords := strings.Fields(querySanitized)

	wg.Add(1)
	go func() {
		p, e := s.client.GetPlaylists(nil)
		if e == nil {
			playlists = sharedutil.FilterSlice(p, func(p *subsonic.Playlist) bool {
				return helpers.AllTermsMatch(strings.ToLower(sanitize.Accents(p.Name)), queryLowerWords)
			})
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		g, e := s.client.GetGenres()
		if e == nil {
			genres = sharedutil.FilterSlice(g, func(g *subsonic.Genre) bool {
				return helpers.AllTermsMatch(strings.ToLower(sanitize.Accents(g.Name)), queryLowerWords)
			})
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		r, e := s.GetRadioStations()
		if e == nil {
			radios = sharedutil.FilterSlice(r, func(r *mediaprovider.RadioStation) bool {
				return helpers.AllTermsMatch(strings.ToLower(sanitize.Accents(r.StationName)), queryLowerWords)
			})
		}
		wg.Done()
	}()

	wg.Wait()
	if err != nil {
		return nil, err
	}

	results := mergeResults(result, playlists, genres, radios)
	helpers.RankSearchResults(results, querySanitized, queryLowerWords)
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}

func mergeResults(
	searchResult *subsonic.SearchResult3,
	matchingPlaylists []*subsonic.Playlist,
	matchingGenres []*subsonic.Genre,
	matchingRadios []*mediaprovider.RadioStation,
) []*mediaprovider.SearchResult {
	var results []*mediaprovider.SearchResult

	for _, al := range searchResult.Album {
		results = append(results, &mediaprovider.SearchResult{
			Type:       mediaprovider.ContentTypeAlbum,
			ID:         al.ID,
			CoverID:    al.CoverArt,
			Name:       al.Name,
			ArtistName: getNameString(al.Artist, al.Artists),
			Size:       al.SongCount,
			Item:       toAlbum(al),
		})
	}

	for _, ar := range searchResult.Artist {
		results = append(results, &mediaprovider.SearchResult{
			Type:    mediaprovider.ContentTypeArtist,
			ID:      ar.ID,
			CoverID: ar.CoverArt,
			Name:    ar.Name,
			Size:    ar.AlbumCount,
			Item:    toArtistFromID3(ar),
		})
	}

	for _, tr := range searchResult.Song {
		results = append(results, &mediaprovider.SearchResult{
			Type:       mediaprovider.ContentTypeTrack,
			ID:         tr.ID,
			CoverID:    tr.CoverArt,
			Name:       tr.Title,
			ArtistName: getNameString(tr.Artist, tr.Artists),
			Size:       tr.Duration,
			Item:       toTrack(tr),
		})
	}

	for _, pl := range matchingPlaylists {
		results = append(results, &mediaprovider.SearchResult{
			Type:    mediaprovider.ContentTypePlaylist,
			ID:      pl.ID,
			CoverID: pl.CoverArt,
			Name:    pl.Name,
			Size:    pl.SongCount,
			Item:    toPlaylist(pl),
		})
	}

	for _, g := range matchingGenres {
		results = append(results, &mediaprovider.SearchResult{
			Type: mediaprovider.ContentTypeGenre,
			ID:   g.Name,
			Name: g.Name,
			Size: g.AlbumCount,
		})
	}

	for _, r := range matchingRadios {
		results = append(results, &mediaprovider.SearchResult{
			Type: mediaprovider.ContentTypeRadioStation,
			ID:   r.ID,
			Name: r.StationName,
			Item: r,
		})
	}

	return results
}

// select Subsonic single-valued name or join OpenSubsonic multi-valued names
func getNameString(singleName string, idNames []subsonic.IDName) string {
	if len(idNames) == 0 {
		return singleName
	}
	names := sharedutil.MapSlice(idNames, func(a subsonic.IDName) string {
		return a.Name
	})
	return strings.Join(names, ", ")
}
