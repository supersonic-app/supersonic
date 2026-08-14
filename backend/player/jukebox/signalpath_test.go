package jukebox

import (
	"testing"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/player"
)

func TestJukeboxSnapshotIsAlwaysRemoteRenderer(t *testing.T) {
	j := &JukeboxPlayer{}
	j.publishSignalPath(&mediaprovider.Track{
		ID:          "track",
		ContentType: "audio/flac",
		Extension:   "flac",
		SampleRate:  96000,
		BitDepth:    24,
		Channels:    2,
	})

	snapshot := j.SignalPathSnapshot()
	if snapshot.Effective != player.EffectiveRemoteRenderer {
		t.Fatalf("effective mode = %q, want remote_renderer", snapshot.Effective)
	}
	if snapshot.Source.TrackID != "track" || snapshot.Receipt.DeliveryPolicy != player.DeliveryRemoteControlled {
		t.Fatalf("source evidence was not retained: %#v", snapshot)
	}
	if snapshot.IsBitPerfect() {
		t.Fatal("Jukebox snapshot claimed the remote DAC path is bit-perfect")
	}
}
