package helpers

import (
	"fmt"
	"sort"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
)

func GetSimilarSongsFallback(mp mediaprovider.MediaProvider, track *mediaprovider.Track, count int) []*mediaprovider.Track {
	var tracks []*mediaprovider.Track
	if len(track.ArtistIDs) > 0 {
		tracks, _ = mp.GetSimilarTracks(track.ArtistIDs[0], count)
	}
	if len(tracks) == 0 {
		genre := ""
		if len(track.Genres) > 0 {
			genre = track.Genres[0]
		}
		tracks, _ = mp.GetRandomTracks(genre, count)
	}

	// make sure to exclude the song itself from the similar list
	return sharedutil.FilterSlice(tracks, func(t *mediaprovider.Track) bool {
		return t.ID != track.ID
	})
}

func GetArtistTracks(mp mediaprovider.MediaProvider, artistID string) ([]*mediaprovider.Track, error) {
	artist, err := mp.GetArtist(artistID)
	if err != nil {
		return nil, fmt.Errorf("error getting artist tracks: %v", err.Error())
	}
	var allTracks []*mediaprovider.Track
	for _, album := range artist.Albums {
		album, err := mp.GetAlbum(album.ID)
		if err != nil {
			return nil, fmt.Errorf("error loading album tracks: %v", err.Error())
		}
		allTracks = append(allTracks, album.Tracks...)
	}
	return allTracks, nil
}

func GetTopTracksFallback(mp mediaprovider.MediaProvider, artistID string, count int) ([]*mediaprovider.Track, error) {
	tracks, err := GetArtistTracks(mp, artistID)
	if err != nil {
		return nil, err
	}
	sort.Slice(tracks, func(i, j int) bool {
		return tracks[i].PlayCount > tracks[j].PlayCount
	})
	if len(tracks) > count {
		return tracks[:count], nil
	}
	return tracks, nil
}
