package helpers

import (
	"log"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
)

type baseIter[M, F any] struct {
	filter        mediaprovider.MediaFilter[M, F]
	prefetchCB    func(*M)
	serverPos     int
	fetcher       func(offset, limit int) ([]*M, error)
	prefetched    []*M
	prefetchedPos int
	done          bool
}

type AlbumFetchFn func(offset, limit int) ([]*mediaprovider.Album, error)

func NewAlbumIterator(fetchFn AlbumFetchFn, filter mediaprovider.AlbumFilter, cb func(string)) mediaprovider.AlbumIterator {
	return &baseIter[mediaprovider.Album, mediaprovider.AlbumFilterOptions]{
		prefetchCB: func(a *mediaprovider.Album) { cb(a.CoverArtID) },
		filter:     filter,
		fetcher:    fetchFn,
	}
}

type ArtistFetchFn func(offset, limit int) ([]*mediaprovider.Artist, error)

func NewArtistIterator(fetchFn ArtistFetchFn, filter mediaprovider.ArtistFilter, cb func(string)) mediaprovider.ArtistIterator {
	return &baseIter[mediaprovider.Artist, mediaprovider.ArtistFilterOptions]{
		prefetchCB: func(a *mediaprovider.Artist) { cb(a.CoverArtID) },
		fetcher:    fetchFn,
		filter:     filter,
	}
}

type TrackFetchFn func(offset, limit int) ([]*mediaprovider.Track, error)

func NewTrackIterator(fetchFn TrackFetchFn, cb func(string)) mediaprovider.TrackIterator {
	return &baseIter[mediaprovider.Track, nilFilterOptions]{
		prefetchCB: func(a *mediaprovider.Track) { cb(a.CoverArtID) },
		filter:     nilFilter[mediaprovider.Track]{},
		fetcher:    fetchFn,
	}
}

func (r *baseIter[M, F]) Next() *M {
	if r.done {
		return nil
	}
	if r.prefetched != nil && r.prefetchedPos < len(r.prefetched) {
		a := r.prefetched[r.prefetchedPos]
		r.prefetchedPos++
		return a
	}
	r.prefetched = nil
	for { // keep fetching until we are done or have matching results
		items, err := r.fetcher(r.serverPos, 20)
		if err != nil {
			log.Printf("error fetching items: %s", err.Error())
			items = nil
		}
		if len(items) == 0 {
			r.done = true
			return nil
		}
		r.serverPos += len(items)
		if !r.filter.IsNil() {
			items = sharedutil.FilterSlice(items, r.filter.Matches)
		}
		r.prefetched = items
		if len(items) > 0 {
			break
		}
	}
	r.prefetchedPos = 1
	if r.prefetchCB != nil {
		for _, item := range r.prefetched {
			go r.prefetchCB(item)
		}
	}
	return r.prefetched[0]
}

type randomAlbumIter struct {
	filter        mediaprovider.AlbumFilter
	prefetchCB    func(coverArtID string)
	albumIDSet    map[string]bool
	prefetched    []*mediaprovider.Album
	prefetchedPos int
	// Random iter works in two phases - phase 1 by requesting random
	// albums from the server. Since the Subsonic API provides no way
	// of paginating a single random sort, we may get albums back twice.
	// We use albumIDSet to keep track of which albums have already been returned.
	// Once we start getting back too many already-returned albums,
	// switch to requesting more albums from a deterministic sort order.
	deterministicFetcher AlbumFetchFn
	randomFetcher        AlbumFetchFn
	phaseTwo             bool
	offset               int
	done                 bool
}

func NewRandomAlbumIter(deterministicFetcher, randomFetcher AlbumFetchFn, filter mediaprovider.AlbumFilter, prefetchCoverCB func(string)) *randomAlbumIter {
	return &randomAlbumIter{
		filter:               filter,
		prefetchCB:           prefetchCoverCB,
		deterministicFetcher: deterministicFetcher,
		randomFetcher:        randomFetcher,
		albumIDSet:           make(map[string]bool),
	}
}

func (r *randomAlbumIter) Next() *mediaprovider.Album {
	if r.done {
		return nil
	}

	// repeat fetch task until we have matching results
	// or we reach the end (handled via short circuit return)
	for len(r.prefetched) == 0 {
		if r.phaseTwo {
			// fetch albums from deterministic order
			albums, err := r.deterministicFetcher(r.offset, 25)
			if err != nil {
				log.Printf("error fetching albums: %s", err.Error())
				albums = nil
			}
			if len(albums) == 0 {
				r.done = true
				r.albumIDSet = nil
				return nil
			}
			r.offset += len(albums)
			for _, album := range albums {
				if _, ok := r.albumIDSet[album.ID]; !ok && r.filter.Matches(album) {
					r.prefetched = append(r.prefetched, album)
					if r.prefetchCB != nil {
						go r.prefetchCB(album.CoverArtID)
					}
					r.albumIDSet[album.ID] = true
				}
			}
		} else {
			albums, err := r.randomFetcher(r.offset, 25)
			if err != nil {
				log.Println(err)
				r.done = true
				r.albumIDSet = nil
				return nil
			}
			r.offset += len(albums)
			var hitCount int
			for _, album := range albums {
				if _, ok := r.albumIDSet[album.ID]; !ok {
					// still need to keep track even if album is not matched
					// by the filter because we need to know when to move to phase two
					hitCount++
					r.albumIDSet[album.ID] = true
					if r.filter.Matches(album) {
						r.prefetched = append(r.prefetched, album)
						if r.prefetchCB != nil {
							go r.prefetchCB(album.CoverArtID)
						}
					}
				}
			}
			if successRatio := float64(hitCount) / float64(25); successRatio < 0.3 {
				r.phaseTwo = true
				r.offset = 0
			}
		}
	}

	// return from prefetched results
	if len(r.prefetched) > 0 {
		a := r.prefetched[r.prefetchedPos]
		r.prefetchedPos++
		if r.prefetchedPos == len(r.prefetched) {
			r.prefetched = nil
			r.prefetchedPos = 0
		}

		return a
	}

	return nil
}

type nilFilterOptions struct{}

type nilFilter[M any] struct{}

func (n nilFilter[M]) IsNil() bool { return true }

func (n nilFilter[M]) Matches(*M) bool { return true }

func (n nilFilter[M]) Clone() mediaprovider.MediaFilter[M, nilFilterOptions] { return n }

func (n nilFilter[M]) Options() nilFilterOptions { return nilFilterOptions{} }

func (n nilFilter[M]) SetOptions(options nilFilterOptions) {}
