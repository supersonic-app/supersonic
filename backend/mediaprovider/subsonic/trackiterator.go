package subsonic

import (
	"log"

	"github.com/supersonic-app/go-subsonic/subsonic"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

func (s *subsonicMediaProvider) IterateTracks(searchQuery string) mediaprovider.TrackIterator {
	if searchQuery == "" {
		return &allTracksIterator{
			s: s,
			albumIter: s.IterateAlbums(
				mediaprovider.AlbumSortRecentlyAdded,
				mediaprovider.NewAlbumFilter(mediaprovider.AlbumFilterOptions{}),
			),
		}
	}
	return &searchTracksIterator{
		searchIterBase: searchIterBase{
			s:             s.client,
			query:         searchQuery,
			musicFolderId: s.currentLibraryID,
			songCount:     50, // default page size for dedicated search
			artistCount:   0,
			albumCount:    0,
		},
		trackIDset: make(map[string]bool),
	}
}

type allTracksIterator struct {
	s           *subsonicMediaProvider
	albumIter   mediaprovider.AlbumIterator
	curAlbum    *mediaprovider.AlbumWithTracks
	curTrackIdx int
	done        bool
}

func (a *allTracksIterator) Next() *mediaprovider.Track {
	if a.done {
		return nil
	}

	// fetch next album
	if a.curAlbum == nil || a.curTrackIdx >= len(a.curAlbum.Tracks) {
		haveNextAlbum := false
		for !haveNextAlbum && !a.done {
			al := a.albumIter.Next()
			if al == nil {
				a.done = true
				return nil
			}
			alWithTracks, err := a.s.GetAlbum(al.ID)
			if err != nil {
				log.Printf("error fetching album: %s", err.Error())
			} else if alWithTracks == nil {
				log.Printf("album not found")
				continue // try next album
			}
			haveNextAlbum = true
			if len(alWithTracks.Tracks) == 0 {
				// in the unlikely case of an album with zero tracks,
				// just call recursively to move to next album
				return a.Next()
			}
			a.curAlbum = alWithTracks
			a.curTrackIdx = 0
		}
	}

	tr := a.curAlbum.Tracks[a.curTrackIdx]
	a.curTrackIdx += 1
	return tr
}

type searchTracksIterator struct {
	searchIterBase

	prefetched    []*subsonic.Child
	prefetchedPos int
	trackIDset    map[string]bool
	done          bool
}

func (s *searchTracksIterator) Next() *mediaprovider.Track {
	if s.done {
		return nil
	}

	// prefetch more search results from server
	if len(s.prefetched) == 0 {
		results := s.searchIterBase.fetchHybridResults("song")

		if results != nil {
			// add results from songs search
			s.addNewTracks(results.Song)
			s.songOffset += len(results.Song)

			// add results from artists search (fallback discovery)
			for _, artist := range results.Artist {
			artist, err := s.s.GetArtist(artist.ID)
			if err != nil {
				log.Printf("error fetching artist: %s", err.Error())
			} else if artist == nil {
				log.Printf("artist not found")
			} else {
					s.addNewTracksFromAlbums(artist.Album)
				}
			}
			s.artistOffset += len(results.Artist)

			// add results from albums search (fallback discovery)
			s.addNewTracksFromAlbums(results.Album)
			s.albumOffset += len(results.Album)
		}
	}

	// return from prefetched results
	if len(s.prefetched) > 0 {
		tr := s.prefetched[s.prefetchedPos]
		s.prefetchedPos++
		if s.prefetchedPos == len(s.prefetched) {
			s.prefetched = s.prefetched[:0]
			s.prefetchedPos = 0
		}
		return toTrack(tr)
	}

	// no more results
	s.done = true
	s.prefetched = nil
	s.trackIDset = nil
	return nil
}

func (s *searchTracksIterator) addNewTracks(tracks []*subsonic.Child) {
	for _, tr := range tracks {
		if _, have := s.trackIDset[tr.ID]; have {
			continue
		}
		s.prefetched = append(s.prefetched, tr)
		s.trackIDset[tr.ID] = true
	}
}

func (s *searchTracksIterator) addNewTracksFromAlbums(albums []*subsonic.AlbumID3) {
	for _, al := range albums {
		if album, err := s.s.GetAlbum(al.ID); err != nil {
			log.Printf("error fetching album: %s", err.Error())
		} else if album == nil {
			log.Printf("album not found")
		} else {
			s.addNewTracks(album.Song)
		}
	}
}
