package dlna

import (
	"testing"
	"time"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

func TestBuildMediaItemCopiesAudioMetadata(t *testing.T) {
	player := &DLNAPlayer{}
	meta := mediaprovider.MediaItemMetadata{
		MIMEType:     "audio/mpeg",
		Name:         "Track",
		Artists:      []string{"Artist One", "Artist Two"},
		Album:        "Album",
		TrackNumber:  3,
		Duration:     2*time.Minute + 3*time.Second,
		Size:         1234,
		BitRate:      192,
		SampleRate:   44100,
		BitDepth:     16,
		ChannelCount: 2,
	}

	item := player.buildMediaItem("http://192.0.2.10:40000/track", meta)
	if item.URL != "http://192.0.2.10:40000/track" || item.Title != "Track" {
		t.Fatalf("unexpected item identity: %#v", item)
	}
	if item.Artist != "Artist One, Artist Two" || item.Album != "Album" || item.TrackNumber != 3 {
		t.Fatalf("unexpected item metadata: %#v", item)
	}
	if item.Duration != meta.Duration || item.Size != meta.Size || item.Bitrate != 192*125 {
		t.Fatalf("unexpected item stream metadata: %#v", item)
	}
	if item.SampleFrequency != 44100 || item.BitsPerSample != 16 || item.NrAudioChannels != 2 {
		t.Fatalf("unexpected audio metadata: %#v", item)
	}
}
