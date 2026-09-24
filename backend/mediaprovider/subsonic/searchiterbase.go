package subsonic

import (
	"log"
	"strconv"

	"github.com/supersonic-app/go-subsonic/subsonic"
)

type searchIterBase struct {
	musicFolderId string
	query         string
	artistOffset  int
	albumOffset   int
	songOffset    int
	artistCount   int
	albumCount    int
	songCount     int
	s             *subsonic.Client
}

func (s *searchIterBase) fetchResults() *subsonic.SearchResult3 {
	return s.fetchWithCounts(s.artistCount, s.albumCount, s.songCount)
}

func (s *searchIterBase) fetchWithCounts(artistCount, albumCount, songCount int) *subsonic.SearchResult3 {
	searchOpts := map[string]string{
		"artistOffset": strconv.Itoa(s.artistOffset),
		"albumOffset":  strconv.Itoa(s.albumOffset),
		"songOffset":   strconv.Itoa(s.songOffset),
		"artistCount":  strconv.Itoa(artistCount),
		"albumCount":   strconv.Itoa(albumCount),
		"songCount":    strconv.Itoa(songCount),
	}
	if s.musicFolderId != "" {
		searchOpts["musicFolderId"] = s.musicFolderId
	}
	results, err := s.s.Search3(s.query, searchOpts)
	if err != nil {
		log.Println(err)
		results = nil
	}
	if results == nil || len(results.Album)+len(results.Artist)+len(results.Song) == 0 {
		return nil
	}
	return results
}

// fetchHybridResults implements the "Smart Hybrid Search":
// 1. Primary request for the category type (e.g. Albums).
// 2. If results are sparse (< 10), trigger secondary requests for Artists/Songs (limit 3)
//    to preserve discovery features (like discography-via-artist-name).
func (s *searchIterBase) fetchHybridResults(primaryType string) *subsonic.SearchResult3 {
	// 1. Primary request
	results := s.fetchResults()

	// 2. Evaluate if fallback is needed.
	// We only do this for the first page (offsets at 0) to avoid infinite loops.
	if s.artistOffset > 0 || s.albumOffset > 0 || s.songOffset > 0 {
		return results
	}

	var count int
	if results != nil {
		switch primaryType {
		case "album":
			count = len(results.Album)
		case "artist":
			count = len(results.Artist)
		case "song":
			count = len(results.Song)
		}
	}

	if count >= 10 {
		return results
	}

	// Trigger fallback. We request small amounts of the other types.
	fallbackArtistCount := 0
	fallbackAlbumCount := 0
	fallbackSongCount := 0

	if primaryType != "artist" {
		fallbackArtistCount = 3
	}
	if primaryType != "album" {
		fallbackAlbumCount = 3
	}
	if primaryType != "song" {
		fallbackSongCount = 3
	}

	fallbackResults := s.fetchWithCounts(fallbackArtistCount, fallbackAlbumCount, fallbackSongCount)
	if fallbackResults == nil {
		return results
	}

	if results == nil {
		return fallbackResults
	}

	// Merge fallback into primary results
	results.Album = append(results.Album, fallbackResults.Album...)
	results.Artist = append(results.Artist, fallbackResults.Artist...)
	results.Song = append(results.Song, fallbackResults.Song...)

	return results
}
