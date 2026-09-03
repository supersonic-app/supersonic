package subsonic

import (
	"log"
	"strconv"
	"strings"

	"github.com/supersonic-app/go-subsonic/subsonic"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/mediaprovider/helpers"
	"github.com/supersonic-app/supersonic/sharedutil"
)

func (s *subsonicMediaProvider) AlbumSortOrders() []string {
	return []string{
		mediaprovider.AlbumSortRecentlyAdded,
		mediaprovider.AlbumSortRecentlyPlayed,
		mediaprovider.AlbumSortFrequentlyPlayed,
		mediaprovider.AlbumSortRandom,
		mediaprovider.AlbumSortTitleAZ,
		mediaprovider.AlbumSortArtistAZ,
		mediaprovider.AlbumSortYearAscending,
		mediaprovider.AlbumSortYearDescending,
	}
}

func filterAlbumMatches(f mediaprovider.AlbumFilter, album *subsonic.AlbumID3, ignoreGenre bool) bool {
	filterOptions := f.Options()
	if album == nil {
		return false
	}
	if filterOptions.ExcludeFavorited && !album.Starred.IsZero() {
		return false
	}
	if filterOptions.ExcludeUnfavorited && album.Starred.IsZero() {
		return false
	}
	if y := album.Year; y < filterOptions.MinYear || (filterOptions.MaxYear > 0 && y > filterOptions.MaxYear) {
		return false
	}
	if ignoreGenre || len(filterOptions.Genres) == 0 {
		return true
	}
	for _, g := range filterOptions.Genres {
		if strings.EqualFold(g, album.Genre) {
			return true
		}
	}
	return false
}

func (s *subsonicMediaProvider) IterateAlbums(sortOrder string, filter mediaprovider.AlbumFilter) mediaprovider.AlbumIterator {
	filterOptions := filter.Options()
	if sortOrder == "" && len(filterOptions.Genres) == 1 {
		genre := filterOptions.Genres[0]
		// The Subsonic API (non-OpenSubsonic) returns only the first genre for multi-genre albums,
		// but servers do internally match against all the genres the album is categorized with.
		// So we must not additionally filter by genre to avoid excluding results where
		// the single genre returned by Subsonic isn't the one we're iterating on.
		modifiedFilter := filter.Clone()
		modifiedOptions := modifiedFilter.Options()
		modifiedOptions.Genres = nil
		modifiedFilter.SetOptions(modifiedOptions)
		fetchFn := func(offset, limit int) ([]*subsonic.AlbumID3, error) {
			params := map[string]string{"genre": genre, "offset": strconv.Itoa(offset), "limit": strconv.Itoa(limit)}
			if s.currentLibraryID != "" {
				params["musicFolderId"] = s.currentLibraryID
			}
			return s.client.GetAlbumList2("byGenre", params)
		}
		return helpers.NewAlbumIterator(makeFetchFn(fetchFn), modifiedFilter)
	}
	if sortOrder == "" && filterOptions.ExcludeUnfavorited {
		modifiedFilter := filter.Clone()
		modifiedOptions := modifiedFilter.Options()
		modifiedOptions.ExcludeUnfavorited = false // we're already filtering by this
		modifiedFilter.SetOptions(modifiedOptions)
		return s.baseIterFromSimpleSortOrder("starred", modifiedFilter)
	}
	if sortOrder == "" {
		sortOrder = mediaprovider.AlbumSortRecentlyAdded // default
	}
	switch sortOrder {
	case mediaprovider.AlbumSortRecentlyAdded:
		return s.baseIterFromSimpleSortOrder("newest", filter)
	case mediaprovider.AlbumSortRecentlyPlayed:
		return s.baseIterFromSimpleSortOrder("recent", filter)
	case mediaprovider.AlbumSortFrequentlyPlayed:
		return s.baseIterFromSimpleSortOrder("frequent", filter)
	case mediaprovider.AlbumSortRandom:
		return s.newRandomIter(filter)
	case mediaprovider.AlbumSortTitleAZ:
		return s.baseIterFromSimpleSortOrder("alphabeticalByName", filter)
	case mediaprovider.AlbumSortArtistAZ:
		return s.baseIterFromSimpleSortOrder("alphabeticalByArtist", filter)
	case mediaprovider.AlbumSortYearAscending:
		fetchFn := func(offset, limit int) ([]*subsonic.AlbumID3, error) {
			params := map[string]string{"fromYear": "0", "toYear": "3000", "offset": strconv.Itoa(offset), "limit": strconv.Itoa(limit)}
			if s.currentLibraryID != "" {
				params["musicFolderId"] = s.currentLibraryID
			}
			return s.client.GetAlbumList2("byYear", params)
		}
		return helpers.NewAlbumIterator(makeFetchFn(fetchFn), filter)
	case mediaprovider.AlbumSortYearDescending:
		fetchFn := func(offset, limit int) ([]*subsonic.AlbumID3, error) {
			params := map[string]string{"fromYear": "3000", "toYear": "0", "offset": strconv.Itoa(offset), "limit": strconv.Itoa(limit)}
			if s.currentLibraryID != "" {
				params["musicFolderId"] = s.currentLibraryID
			}
			return s.client.GetAlbumList2("byYear", params)
		}
		return helpers.NewAlbumIterator(makeFetchFn(fetchFn), filter)
	default:
		log.Printf("Undefined album sort order: %s", sortOrder)
		return nil
	}
}

func (s *subsonicMediaProvider) SearchAlbums(searchQuery string, filter mediaprovider.AlbumFilter) mediaprovider.AlbumIterator {
	return s.newSearchAlbumIter(searchQuery, filter)
}

type searchAlbumIter struct {
	searchIterBase

	filter        mediaprovider.AlbumFilter
	prefetched    []*subsonic.AlbumID3
	prefetchedPos int
	albumIDset    map[string]bool
	done          bool
}

func (s *subsonicMediaProvider) newSearchAlbumIter(query string, filter mediaprovider.AlbumFilter) *searchAlbumIter {
	return &searchAlbumIter{
		searchIterBase: searchIterBase{
			query:         query,
			s:             s.client,
			musicFolderId: s.currentLibraryID,
			albumCount:    50, // default page size for dedicated search
			artistCount:   0,
			songCount:     0,
		},
		filter:     filter,
		albumIDset: make(map[string]bool),
	}
}

func (s *searchAlbumIter) Next() *mediaprovider.Album {
	if s.done {
		return nil
	}

	// prefetch more search results from server
	if s.prefetched == nil {
		results := s.searchIterBase.fetchHybridResults("album")
		if results == nil {
			s.done = true
			s.albumIDset = nil
			return nil
		}

		// add results from albums search
		s.addNewAlbums(results.Album)
		s.albumOffset += len(results.Album)

		// add results from artists search (fallback discovery)
		for _, artist := range results.Artist {
			artist, err := s.s.GetArtist(artist.ID)
			if err != nil {
				log.Printf("error fetching artist: %s", err.Error())
			} else if artist == nil {
				log.Printf("artist not found")
			} else {
				s.addNewAlbums(artist.Album)
			}
		}
		s.artistOffset += len(results.Artist)

		// add results from songs search (fallback discovery)
		for _, song := range results.Song {
			if song.AlbumID == "" {
				continue
			}
			album, err := s.s.GetAlbum(song.AlbumID)
			if err != nil {
				log.Printf("error fetching album: %s", err.Error())
			} else if album == nil {
				log.Printf("album not found")
			} else {
				s.addNewAlbums([]*subsonic.AlbumID3{album})
			}
		}
		s.songOffset += len(results.Song)
	}

	// return from prefetched results
	if len(s.prefetched) > 0 {
		a := s.prefetched[s.prefetchedPos]
		s.prefetchedPos++
		if s.prefetchedPos == len(s.prefetched) {
			s.prefetched = nil
			s.prefetchedPos = 0
		}

		return toAlbum(a)
	}

	return nil
}

func (s *searchAlbumIter) addNewAlbums(al []*subsonic.AlbumID3) {
	for _, album := range al {
		if _, have := s.albumIDset[album.ID]; have {
			continue
		}
		if !filterAlbumMatches(s.filter, album, false) {
			continue
		}
		s.prefetched = append(s.prefetched, album)
		s.albumIDset[album.ID] = true
	}
}

func (s *subsonicMediaProvider) newRandomIter(filter mediaprovider.AlbumFilter) mediaprovider.AlbumIterator {
	return helpers.NewRandomAlbumIter(
		s.fetchFnFromStandardSort("newest"),
		makeFetchFn(func(offset, limit int) ([]*subsonic.AlbumID3, error) {
			args := map[string]string{
				"size":   strconv.Itoa(limit),
				"offset": strconv.Itoa(offset),
			}
			if s.currentLibraryID != "" {
				args["musicFolderId"] = s.currentLibraryID
			}
			return s.client.GetAlbumList2("random", args)
		}),
		filter)
}

func (s *subsonicMediaProvider) baseIterFromSimpleSortOrder(sort string, filter mediaprovider.AlbumFilter) mediaprovider.AlbumIterator {
	return helpers.NewAlbumIterator(s.fetchFnFromStandardSort(sort), filter)
}

func (s *subsonicMediaProvider) fetchFnFromStandardSort(sort string) helpers.AlbumFetchFn {
	return makeFetchFn(func(offset, limit int) ([]*subsonic.AlbumID3, error) {
		params := map[string]string{"size": strconv.Itoa(limit), "offset": strconv.Itoa(offset)}
		if s.currentLibraryID != "" {
			params["musicFolderId"] = s.currentLibraryID
		}
		return s.client.GetAlbumList2(sort, params)
	})
}

func makeFetchFn(subsonicFetchFn func(offset, limit int) ([]*subsonic.AlbumID3, error)) helpers.AlbumFetchFn {
	return func(offset, limit int) ([]*mediaprovider.Album, error) {
		al, err := subsonicFetchFn(offset, limit)
		if err != nil {
			return nil, err
		}
		return sharedutil.MapSlice(al, toAlbum), nil
	}
}
