package subsonic

import (
	"log"
	"math/rand"
	"slices"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/supersonic-app/go-subsonic/subsonic"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/mediaprovider/helpers"
	"github.com/supersonic-app/supersonic/sharedutil"
)

func (s *subsonicMediaProvider) ArtistSortOrders() []string {
	return []string{
		mediaprovider.ArtistSortAlbumCount,
		mediaprovider.ArtistSortNameAZ,
		mediaprovider.ArtistSortRandom,
	}
}

func filterArtistMatches(_ mediaprovider.ArtistFilter, artist *subsonic.ArtistID3) bool {
	return artist != nil
}

func (s *subsonicMediaProvider) IterateArtists(sortOrder string, filter mediaprovider.ArtistFilter) mediaprovider.ArtistIterator {
	if sortOrder == "" {
		sortOrder = mediaprovider.ArtistSortNameAZ // default
	}
	switch sortOrder {
	case mediaprovider.ArtistSortAlbumCount:
		return s.baseArtistIterFromSimpleSortOrder(
			func(artists []*subsonic.ArtistID3) []*subsonic.ArtistID3 {
				slices.SortStableFunc(artists, func(a, b *subsonic.ArtistID3) int {
					return b.AlbumCount - a.AlbumCount
				})
				return artists
			},
			filter,
		)
	case mediaprovider.ArtistSortNameAZ:
		return s.baseArtistIterFromSimpleSortOrder(
			func(artists []*subsonic.ArtistID3) []*subsonic.ArtistID3 {
				c := collate.New(language.English, collate.Loose)
				slices.SortFunc(artists, func(a, b *subsonic.ArtistID3) int {
					sortStr := func(a *subsonic.ArtistID3) string {
						if a.SortName != "" {
							return a.SortName
						}
						return a.Name
					}
					return c.CompareString(sortStr(a), sortStr(b))
				})
				return artists
			},
			filter,
		)
	case mediaprovider.ArtistSortRandom:
		return s.baseArtistIterFromSimpleSortOrder(
			func(artists []*subsonic.ArtistID3) []*subsonic.ArtistID3 {
				newArtists := make([]*subsonic.ArtistID3, len(artists))
				copy(newArtists, artists)
				rand.Shuffle(len(newArtists), func(i, j int) { newArtists[i], newArtists[j] = newArtists[j], newArtists[i] })
				return newArtists
			},
			filter,
		)
	default:
		log.Printf("Undefined artist sort order: %s", sortOrder)
		return nil
	}
}

func (s *subsonicMediaProvider) SearchArtists(searchQuery string, filter mediaprovider.ArtistFilter) mediaprovider.ArtistIterator {
	return s.newSearchArtistIter(searchQuery, filter)
}

type searchArtistIter struct {
	searchIterBase

	filter        mediaprovider.ArtistFilter
	prefetched    []*subsonic.ArtistID3
	prefetchedPos int
	artistIDset   map[string]bool
	done          bool
}

func (s *subsonicMediaProvider) newSearchArtistIter(query string, filter mediaprovider.ArtistFilter) *searchArtistIter {
	return &searchArtistIter{
		searchIterBase: searchIterBase{
			query:         query,
			s:             s.client,
			musicFolderId: s.currentLibraryID,
			artistCount:   50, // default page size for dedicated search
			albumCount:    0,
			songCount:     0,
		},
		filter:      filter,
		artistIDset: make(map[string]bool),
	}
}

func (s *searchArtistIter) Next() *mediaprovider.Artist {
	if s.done {
		return nil
	}

	// prefetch more search results from server
	if s.prefetched == nil {
		results := s.searchIterBase.fetchHybridResults("artist")
		if results == nil {
			s.done = true
			s.artistIDset = nil
			return nil
		}

		// add results from artists search
		s.addNewArtists(results.Artist)
		s.artistOffset += len(results.Artist)

		// skip aggressive discovery for other types in artist view for now
		// as there's no clear way to map albums/songs to unique artists reliably
		// without getting duplicates or irrelevant hits.
		s.albumOffset += len(results.Album)
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

		return toArtistFromID3(a)
	}

	return nil
}

func (s *searchArtistIter) addNewArtists(artists []*subsonic.ArtistID3) {
	for _, artist := range artists {
		if _, have := s.artistIDset[artist.ID]; have {
			continue
		}
		if !filterArtistMatches(s.filter, artist) {
			continue
		}
		s.prefetched = append(s.prefetched, artist)
		s.artistIDset[artist.ID] = true
	}
}

func (s *subsonicMediaProvider) baseArtistIterFromSimpleSortOrder(sortFn func([]*subsonic.ArtistID3) []*subsonic.ArtistID3, filter mediaprovider.ArtistFilter) mediaprovider.ArtistIterator {
	return helpers.NewArtistIterator(s.artistFetchFnFromStandardSort(sortFn), filter)
}

func (s *subsonicMediaProvider) artistFetchFnFromStandardSort(sortFn func([]*subsonic.ArtistID3) []*subsonic.ArtistID3) helpers.ArtistFetchFn {
	return makeArtistFetchFn(func(offset, limit int) ([]*subsonic.ArtistID3, error) {
		// When the iterator asks for a second page of results, return nil, as Subsonic does not support pagination for artists.
		if offset > 0 {
			return nil, nil
		}

		var params map[string]string
		if s.currentLibraryID != "" {
			params = map[string]string{"musicFolderId": s.currentLibraryID}
		}
		idxs, err := s.client.GetArtists(params)
		if err != nil {
			return nil, err
		}
		var artists []*subsonic.ArtistID3
		for _, idx := range idxs.Index {
			artists = append(artists, idx.Artist...)
		}
		artists = sortFn(artists)
		return artists, nil
	})
}

func makeArtistFetchFn(subsonicFetchFn func(offset, limit int) ([]*subsonic.ArtistID3, error)) helpers.ArtistFetchFn {
	return func(offset, limit int) ([]*mediaprovider.Artist, error) {
		ar, err := subsonicFetchFn(offset, limit)
		if err != nil {
			return nil, err
		}
		return sharedutil.MapSlice(ar, toArtistFromID3), nil
	}
}
