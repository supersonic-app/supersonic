package mpv

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/supersonic-app/supersonic/backend/player"
)

func TestExclusiveRequestRemainsUnverifiedWithoutRuntimeEvidence(t *testing.T) {
	p := New()
	p.SetAudioExclusive(true)

	snapshot := p.SignalPathSnapshot()
	if snapshot.Requested != player.ModeExclusiveProcessed {
		t.Fatalf("requested mode = %q, want exclusive_processed", snapshot.Requested)
	}
	if snapshot.Effective != player.EffectiveUnverified {
		t.Fatalf("effective mode = %q, want unverified", snapshot.Effective)
	}
	if snapshot.IsBitPerfect() {
		t.Fatal("an exclusive request without observations claimed bit-perfect output")
	}
}

func TestConfiguredProcessingStagesArePublished(t *testing.T) {
	p := New()
	if err := p.SetVolume(80); err != nil {
		t.Fatal(err)
	}
	if err := p.SetReplayGainOptions(player.ReplayGainOptions{Mode: player.ReplayGainAlbum}); err != nil {
		t.Fatal(err)
	}
	p.SetPauseFade(true)

	snapshot := p.SignalPathSnapshot()
	for _, kind := range []player.ProcessingKind{
		player.ProcessingSoftwareVolume,
		player.ProcessingReplayGain,
		player.ProcessingFade,
	} {
		if !activeProcessing(snapshot, kind) {
			t.Fatalf("processing stage %q was not published: %#v", kind, snapshot.Processing)
		}
	}
}

func TestSignalPathSnapshotNeverContainsSourceURL(t *testing.T) {
	p := New()
	p.setCurrentSource(player.PlaybackSource{
		URL: "https://user:secret@example.test/stream?token=private",
		Descriptor: player.SourceDescriptor{
			ServerID: "server",
			TrackID:  "track",
		},
		Receipt: player.NewSourceReceipt(player.DeliveryRawRequested),
	})

	encoded, err := json.Marshal(p.SignalPathSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "private") {
		t.Fatalf("snapshot leaked source credentials: %s", encoded)
	}
}

func TestPreparedSourceIsPromotedAtTrackBoundary(t *testing.T) {
	p := New()
	p.setCurrentSource(player.PlaybackSource{Descriptor: player.SourceDescriptor{TrackID: "current"}})
	p.sourceMu.Lock()
	p.nextSource = player.PlaybackSource{Descriptor: player.SourceDescriptor{TrackID: "next"}}
	p.haveNextSource = true
	p.sourceMu.Unlock()

	p.promoteNextSource()
	snapshot := p.SignalPathSnapshot()
	if snapshot.Source.TrackID != "next" {
		t.Fatalf("track ID = %q, want next", snapshot.Source.TrackID)
	}
	if snapshot.Generation != 2 {
		t.Fatalf("generation = %d, want 2", snapshot.Generation)
	}
}

func TestLosslessCodecClassification(t *testing.T) {
	tests := map[string]bool{
		"flac":      true,
		"ALAC":      true,
		"pcm_s24le": true,
		"wavpack":   true,
		"mp3":       false,
		"aac":       false,
		"dsd_lsbf":  false,
		"":          false,
	}
	for codec, want := range tests {
		if got := isLosslessCodec(codec); got != want {
			t.Errorf("isLosslessCodec(%q) = %t, want %t", codec, got, want)
		}
	}
}

func activeProcessing(snapshot player.PlaybackSnapshot, kind player.ProcessingKind) bool {
	for _, stage := range snapshot.Processing {
		if stage.Kind == kind {
			return stage.Active
		}
	}
	return false
}
