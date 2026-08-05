package jellyfin

import (
	"slices"

	"github.com/dweymouth/go-jellyfin"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/mediaprovider/helpers"
	"github.com/supersonic-app/supersonic/sharedutil"
)

func (j *JellyfinMediaProvider) ArtistSortOrders() []string {
	return []string{
		mediaprovider.ArtistSortAlbumCount,
		mediaprovider.ArtistSortNameAZ,
		mediaprovider.ArtistSortRandom,
	}
}

func (j *JellyfinMediaProvider) IterateArtists(sortOrder string, filter mediaprovider.ArtistFilter) mediaprovider.ArtistIterator {
	var jfSort jellyfin.Sort
	var disablePagination bool
	var sortFn func([]*jellyfin.Artist) []*jellyfin.Artist

	if sortOrder == "" {
		sortOrder = mediaprovider.ArtistSortNameAZ // default
	}
	switch sortOrder {
	case mediaprovider.ArtistSortAlbumCount:
		// Pagination needs to be disabled, to retrieve all results in a single request, and correctly sort them.
		disablePagination = true
		sortFn = func(artists []*jellyfin.Artist) []*jellyfin.Artist {
			slices.SortStableFunc(artists, func(a, b *jellyfin.Artist) int {
				return b.AlbumCount - a.AlbumCount
			})
			return artists
		}
	case mediaprovider.ArtistSortNameAZ:
		jfSort.Field = jellyfin.SortByName
		jfSort.Mode = jellyfin.SortAsc
	case mediaprovider.ArtistSortRandom:
		jfSort.Field = jellyfin.SortByRandom
	}

	fetcher := makeArtistFetchFn(
		func(offs, limit int) ([]*jellyfin.Artist, error) {
			if disablePagination && offs > 0 {
				return nil, nil
			}
			var paging jellyfin.Paging
			if !disablePagination {
				paging = jellyfin.Paging{StartIndex: offs, Limit: limit}
			}
			return j.client.GetAlbumArtists(jellyfin.QueryOpts{
				Sort:   jfSort,
				Paging: paging,
				Filter: jellyfin.Filter{ParentID: j.currentLibraryID},
			})
		},
		sortFn,
	)

	return helpers.NewArtistIterator(fetcher, filter, j.prefetchCoverCB)
}

func (j *JellyfinMediaProvider) SearchArtists(searchQuery string, filter mediaprovider.ArtistFilter) mediaprovider.ArtistIterator {
	// TODO: Jellyfin API is not returning search results for artists.
	//       Uncomment the following code once the issue is resolved.
	//       Related issue: https://github.com/jellyfin/jellyfin/issues/8222
	// fetcher := makeArtistFetchFn(
	// 	func(offs, limit int) ([]*jellyfin.Artist, error) {
	// 		log.Printf("Searching for artists: %s", searchQuery)
	// 		sr, err := j.client.Search(searchQuery, jellyfin.TypeArtist, jellyfin.Paging{StartIndex: offs, Limit: limit})
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		log.Printf("Found %d artists", len(sr.Artists))
	// 		for _, a := range sr.Artists {
	// 			log.Printf("Artist: %s", a.Name)
	// 		}
	// 		return sr.Artists, nil
	// 	},
	// 	nil,
	// )
	// return helpers.NewArtistIterator(fetcher, filter, j.prefetchCoverCB)

	modifiedFilter := filter.Clone()
	modifiedOptions := modifiedFilter.Options()
	modifiedOptions.SearchQuery = searchQuery
	modifiedFilter.SetOptions(modifiedOptions)

	fetcher := makeArtistFetchFn(
		func(offs, limit int) ([]*jellyfin.Artist, error) {
			return j.client.GetAlbumArtists(jellyfin.QueryOpts{
				Sort: jellyfin.Sort{
					Field: jellyfin.SortByName,
					Mode:  jellyfin.SortAsc,
				},
				Paging: jellyfin.Paging{StartIndex: offs, Limit: limit},
			})
		},
		nil,
	)
	return helpers.NewArtistIterator(fetcher, modifiedFilter, j.prefetchCoverCB)
}

func makeArtistFetchFn(
	fetchFn func(offset, limit int) ([]*jellyfin.Artist, error),
	sortFn func([]*jellyfin.Artist) []*jellyfin.Artist,
) helpers.ArtistFetchFn {
	return func(offset, limit int) ([]*mediaprovider.Artist, error) {
		ar, err := fetchFn(offset, limit)
		if err != nil {
			return nil, err
		}
		if sortFn != nil {
			ar = sortFn(ar)
		}
		return sharedutil.MapSlice(ar, toArtist), nil
	}
}
