package backend

import (
	"encoding/json"
	"errors"
	"log"
	"os"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

type SavedPlayQueue struct {
	Tracks     []*mediaprovider.Track
	TrackIndex int
	TimePos    float64
}

type serializedSavedPlayQueue struct {
	ServerID   string   `json:"serverID"`
	TrackIDs   []string `json:"trackIDs"`
	TrackIndex int      `json:"trackIndex"`
	TimePos    float64  `json:"timePos"`
}

// SavePlayQueue saves the current play queue and playback position to a JSON file.
// If the provided CanSavePlayQueue server is non-nil, it will also save to the server.
func SavePlayQueue(serverID string, queue []mediaprovider.MediaItem, pm *PlaybackManager, filepath string, server mediaprovider.CanSavePlayQueue) error {
	stats := pm.PlaybackStatus()
	trackIdx := pm.NowPlayingIndex()

	trackIDs := make([]string, 0, len(queue))
	for _, item := range queue {
		if _, ok := item.(*mediaprovider.Track); ok {
			// don't save radio stations in play queue
			trackIDs = append(trackIDs, item.Metadata().ID)
		}
	}
	if l := len(trackIDs); trackIdx >= l {
		if l == 0 {
			trackIdx = 0
		} else {
			trackIdx = l - 1
		}
	}

	saved := serializedSavedPlayQueue{
		ServerID:   serverID,
		TrackIDs:   trackIDs,
		TrackIndex: trackIdx,
		TimePos:    stats.TimePos,
	}
	b, _ := json.Marshal(saved)
	err := os.WriteFile(filepath, b, 0o644)

	if server != nil {
		// save to server
		err = server.SavePlayQueue(trackIDs, trackIdx, int(stats.TimePos))
	}
	return err
}

// Loads the saved play queue from the given filepath using the current server.
// If loadFromServer is true and the current server supports saving the play queue,
// the queue will attempt to load from the server and only use the local file as a fallback.
// Returns an error if the queue could not be loaded for any reason, including the
// currently logged in server being different than the server from which the queue was saved.
func LoadPlayQueue(filepath string, sm *ServerManager, loadFromServer bool) (*SavedPlayQueue, error) {
	if pq, ok := sm.Server.(mediaprovider.CanSavePlayQueue); loadFromServer && ok && pq != nil {
		// load queue from server
		queue, err := pq.GetPlayQueue()
		if err == nil {
			return &SavedPlayQueue{
				Tracks:     queue.Tracks,
				TrackIndex: queue.TrackPos,
				TimePos:    float64(queue.TimePos),
			}, nil
		} else {
			log.Printf("error loading queue from server: %v", err.Error())
		}
	}

	// load queue from local file
	b, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	var savedData serializedSavedPlayQueue
	if err := json.Unmarshal(b, &savedData); err != nil {
		return nil, err
	}

	if sm.ServerID.String() != savedData.ServerID {
		return nil, errors.New("saved play queue was from a different server")
	}

	tracks := make([]*mediaprovider.Track, 0, len(savedData.TrackIDs))
	mp := sm.Server
	for i, id := range savedData.TrackIDs {
		if tr, err := mp.GetTrack(id); err != nil || tr == nil {
			// ignore/skip individual track failures
			if i < savedData.TrackIndex {
				savedData.TrackIndex--
			}
		} else {
			tracks = append(tracks, tr)
		}
	}

	savedQueue := &SavedPlayQueue{
		Tracks:     tracks,
		TrackIndex: savedData.TrackIndex,
		TimePos:    savedData.TimePos,
	}
	return savedQueue, nil
}
