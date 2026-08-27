package backend

import (
	"log"

	"github.com/dweymouth/supersonic/backend/mediaprovider"
)

// ArtistInfoManager provides artist info by first querying the media server,
// then falling back to external sources (Deezer/Wikipedia) to fill in
// missing biography or image data. This follows the same pattern as
// LyricsManager with LrcLib fallback.
type ArtistInfoManager struct {
	sm      *ServerManager
	fetcher *ArtistInfoFetcher
}

// NewArtistInfoManager creates a new ArtistInfoManager.
// If fetcher is nil, no external fallback will be used.
func NewArtistInfoManager(sm *ServerManager, fetcher *ArtistInfoFetcher) *ArtistInfoManager {
	return &ArtistInfoManager{
		sm:      sm,
		fetcher: fetcher,
	}
}

// GetArtistInfo fetches artist info, trying the server first and falling back
// to Deezer/Wikipedia for missing biography or image data.
// artistName is required for the external fallback to work.
func (m *ArtistInfoManager) GetArtistInfo(artistID, artistName string) (*mediaprovider.ArtistInfo, error) {
	var info *mediaprovider.ArtistInfo
	var err error

	// First, try the server's native artist info
	if m.sm.Server != nil {
		info, err = m.sm.Server.GetArtistInfo(artistID)
		if err != nil {
			log.Printf("Error fetching artist info from server: %v", err)
		}
	}
	if info == nil {
		info = &mediaprovider.ArtistInfo{}
	}

	// If external fetcher is available and we're missing data, try fallback
	if m.fetcher != nil && artistName != "" {
		if info.Biography == "" || info.ImageURL == "" {
			fallback, fetchErr := m.fetcher.FetchArtistInfo(artistName)
			if fetchErr != nil {
				log.Printf("Error fetching external artist info: %v", fetchErr)
			}
			if fallback != nil {
				if info.Biography == "" {
					info.Biography = fallback.Biography
				}
				if info.ImageURL == "" {
					info.ImageURL = fallback.ImageURL
				}
				if info.LastFMUrl == "" {
					info.LastFMUrl = fallback.LastFMUrl
				}
			}
		}
	}

	return info, nil
}

// ClearCache clears the external artist info cache.
func (m *ArtistInfoManager) ClearCache() {
	if m.fetcher != nil {
		m.fetcher.ClearCache()
	}
}
