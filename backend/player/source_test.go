package player

import "testing"

func TestNewSourceReceiptRecordsOnlyContractualProvenance(t *testing.T) {
	tests := []struct {
		policy     DeliveryPolicy
		provenance SourceProvenance
	}{
		{DeliveryRawRequested, ProvenanceUnknown},
		{DeliveryTranscodeRequested, ProvenanceTranscodedConfirmed},
		{DeliveryServerDefault, ProvenanceUnknown},
		{DeliveryExternalStream, ProvenanceUnknown},
		{DeliveryRemoteControlled, ProvenanceUnknown},
	}
	for _, test := range tests {
		t.Run(string(test.policy), func(t *testing.T) {
			receipt := NewSourceReceipt(test.policy)
			if receipt.DeliveryPolicy != test.policy {
				t.Fatalf("delivery policy = %q, want %q", receipt.DeliveryPolicy, test.policy)
			}
			if receipt.Provenance != test.provenance {
				t.Fatalf("provenance = %q, want %q", receipt.Provenance, test.provenance)
			}
			if len(receipt.Evidence) != 1 || receipt.Evidence[0].Value != string(test.policy) {
				t.Fatalf("unexpected policy evidence: %#v", receipt.Evidence)
			}
		})
	}
}

func TestPlaybackSourceCloneDoesNotShareEvidence(t *testing.T) {
	original := PlaybackSource{Receipt: NewSourceReceipt(DeliveryRawRequested)}
	clone := original.Clone()
	clone.Receipt.Evidence[0].Value = "changed"
	if original.Receipt.Evidence[0].Value != string(DeliveryRawRequested) {
		t.Fatal("clone mutated the original receipt evidence")
	}
}
