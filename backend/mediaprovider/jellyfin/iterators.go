package jellyfin

import (
	"time"

	"github.com/dweymouth/go-jellyfin"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/mediaprovider/helpers"
	"github.com/supersonic-app/supersonic/sharedutil"
)

func (j *JellyfinMediaProvider) AlbumSortOrders() []string {
	return []string{
		mediaprovider.AlbumSortRecentlyAdded,
		mediaprovider.AlbumSortRandom,
		mediaprovider.AlbumSortTitleAZ,
		mediaprovider.AlbumSortArtistAZ,
		mediaprovider.AlbumSortYearAscending,
		mediaprovider.AlbumSortYearDescending,
	}
}

func (j *JellyfinMediaProvider) IterateAlbums(sortOrder string, filter mediaprovider.AlbumFilter) mediaprovider.AlbumIterator {
	var jfSort jellyfin.Sort
	switch sortOrder {
	case mediaprovider.AlbumSortRecentlyAdded:
		jfSort.Field = jellyfin.SortByDateCreated
		jfSort.Mode = jellyfin.SortDesc
	case mediaprovider.AlbumSortRandom:
		jfSort.Field = jellyfin.SortByRandom
	case mediaprovider.AlbumSortArtistAZ:
		jfSort.Field = jellyfin.SortByArtist
		jfSort.Mode = jellyfin.SortAsc
	case mediaprovider.AlbumSortTitleAZ:
		jfSort.Field = jellyfin.SortByName
		jfSort.Mode = jellyfin.SortAsc
	case mediaprovider.AlbumSortYearAscending:
		jfSort.Field = jellyfin.SortByYear
		jfSort.Mode = jellyfin.SortAsc
	case mediaprovider.AlbumSortYearDescending:
		jfSort.Field = jellyfin.SortByYear
		jfSort.Mode = jellyfin.SortDesc
	}
	jfFilt, modifiedFilter := jfFilterFromFilter(filter)
	if j.currentLibraryID != "" {
		jfFilt.ParentID = j.currentLibraryID
	}

	fetcher := func(offs, limit int) ([]*mediaprovider.Album, error) {
		al, err := j.client.GetAlbums(jellyfin.QueryOpts{
			Sort:   jfSort,
			Filter: jfFilt,
			Paging: jellyfin.Paging{StartIndex: offs, Limit: limit},
		})
		if err != nil {
			return nil, err
		}
		return sharedutil.MapSlice(al, toAlbum), nil
	}

	if sortOrder == mediaprovider.AlbumSortRandom {
		determFetcher := func(offs, limit int) ([]*mediaprovider.Album, error) {
			al, err := j.client.GetAlbums(jellyfin.QueryOpts{
				Sort:   jellyfin.Sort{Field: "SortName", Mode: jellyfin.SortAsc},
				Filter: jfFilt,
				Paging: jellyfin.Paging{StartIndex: offs, Limit: limit},
			})
			if err != nil {
				return nil, err
			}
			return sharedutil.MapSlice(al, toAlbum), nil
		}
		return helpers.NewRandomAlbumIter(determFetcher, fetcher, modifiedFilter, j.prefetchCoverCB)
	}
	return helpers.NewAlbumIterator(fetcher, modifiedFilter, j.prefetchCoverCB)
}

func (j *JellyfinMediaProvider) SearchAlbums(searchQuery string, filter mediaprovider.AlbumFilter) mediaprovider.AlbumIterator {
	fetcher := func(offs, limit int) ([]*mediaprovider.Album, error) {
		var opts jellyfin.QueryOpts
		opts.Paging = jellyfin.Paging{StartIndex: offs, Limit: limit}
		opts.Filter.ParentID = j.currentLibraryID
		sr, err := j.client.Search(searchQuery, jellyfin.TypeAlbum, opts)
		if err != nil {
			return nil, err
		}
		return sharedutil.MapSlice(sr.Albums, toAlbum), nil
	}
	return helpers.NewAlbumIterator(fetcher, filter, j.prefetchCoverCB)
}

func (j *JellyfinMediaProvider) IterateTracks(searchQuery string) mediaprovider.TrackIterator {
	var fetcher helpers.TrackFetchFn
	if searchQuery == "" {
		fetcher = func(offs, limit int) ([]*mediaprovider.Track, error) {
			var opts jellyfin.QueryOpts
			opts.Paging = jellyfin.Paging{StartIndex: offs, Limit: limit}
			if j.currentLibraryID != "" {
				opts.Filter.ParentID = j.currentLibraryID
			}
			s, err := j.client.GetSongs(opts)
			if err != nil {
				return nil, err
			}
			return sharedutil.MapSlice(s, toTrack), nil
		}
	} else {
		fetcher = func(offs, limit int) ([]*mediaprovider.Track, error) {
			var opts jellyfin.QueryOpts
			opts.Paging = jellyfin.Paging{StartIndex: offs, Limit: limit}
			opts.Filter.ParentID = j.currentLibraryID
			sr, err := j.client.Search(searchQuery, jellyfin.TypeSong, opts)
			if err != nil {
				return nil, err
			}
			return sharedutil.MapSlice(sr.Songs, toTrack), nil
		}
	}
	return helpers.NewTrackIterator(fetcher, j.prefetchCoverCB)
}

// Creates the Jellyfin filter to implement the given mediaprovider filter,
// and returns a modified mediaprovider filter, with now-unneeded fields zeroed out.
func jfFilterFromFilter(filter mediaprovider.AlbumFilter) (jellyfin.Filter, mediaprovider.AlbumFilter) {
	var jfFilt jellyfin.Filter

	// Clone the original filter to not modify its options.
	// Set filters must be maintained in the original filter, as they are used for the UI.
	// Modified filter options are used to ignore further filtering that was already handled by the
	// Jellyfin API.
	modifiedFilter := filter.Clone()
	filterOptions := modifiedFilter.Options()

	if filterOptions.ExcludeUnfavorited {
		jfFilt.Favorite = true
		filterOptions.ExcludeUnfavorited = false
	}
	if filterOptions.MinYear > 0 && filterOptions.MaxYear > 0 {
		jfFilt.YearRange = [2]int{filterOptions.MinYear, filterOptions.MaxYear}
		filterOptions.MinYear, filterOptions.MaxYear = 0, 0
	} else if filterOptions.MinYear > 0 {
		jfFilt.YearRange = [2]int{filterOptions.MinYear, time.Now().Year()}
		filterOptions.MinYear, filterOptions.MaxYear = 0, 0
	} else if filterOptions.MaxYear > 0 {
		jfFilt.YearRange = [2]int{1900, filterOptions.MaxYear}
		filterOptions.MinYear, filterOptions.MaxYear = 0, 0
	}
	jfFilt.Genres = filterOptions.Genres
	filterOptions.Genres = nil

	modifiedFilter.SetOptions(filterOptions)
	return jfFilt, modifiedFilter
}
