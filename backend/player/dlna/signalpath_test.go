package dlna

import (
	"testing"

	"github.com/supersonic-app/supersonic/backend/player"
)

func TestDLNASnapshotIsAlwaysRemoteRenderer(t *testing.T) {
	initial := player.ReduceSignalPath(player.SignalPathObservation{
		Requested:      player.ModeNormal,
		RemoteRenderer: true,
	})
	d := &DLNAPlayer{signalPath: player.NewSignalPathState(initial)}
	d.publishSignalPath(player.PlaybackSource{
		Descriptor: player.SourceDescriptor{TrackID: "track"},
		Receipt:    player.NewSourceReceipt(player.DeliveryRawRequested),
	})

	snapshot := d.SignalPathSnapshot()
	if snapshot.Effective != player.EffectiveRemoteRenderer {
		t.Fatalf("effective mode = %q, want remote_renderer", snapshot.Effective)
	}
	if snapshot.Source.TrackID != "track" || snapshot.Receipt.DeliveryPolicy != player.DeliveryRawRequested {
		t.Fatalf("source evidence was not retained: %#v", snapshot)
	}
	if snapshot.IsBitPerfect() {
		t.Fatal("DLNA snapshot claimed the remote DAC path is bit-perfect")
	}
}
