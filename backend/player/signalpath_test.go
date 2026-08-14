package player

import (
	"testing"
	"time"
)

func TestReduceSignalPathCurrentModes(t *testing.T) {
	tests := []struct {
		name         string
		observation  SignalPathObservation
		wantMode     EffectiveMode
		wantVerified VerificationLevel
		wantIssue    FallbackReasonCode
	}{
		{
			name:         "normal is processed shared playback",
			observation:  SignalPathObservation{Requested: ModeNormal},
			wantMode:     EffectiveSharedProcessed,
			wantVerified: VerificationConfigured,
		},
		{
			name:         "exclusive request without observation is unverified",
			observation:  SignalPathObservation{Requested: ModeExclusiveProcessed},
			wantMode:     EffectiveUnverified,
			wantVerified: VerificationConfigured,
			wantIssue:    FallbackEffectiveModeUnknown,
		},
		{
			name: "observed exclusive processed",
			observation: SignalPathObservation{
				Requested:       ModeExclusiveProcessed,
				RuntimeObserved: true,
				Output:          OutputState{ExclusiveKnown: true, Exclusive: true},
			},
			wantMode:     EffectiveExclusiveProcessed,
			wantVerified: VerificationObserved,
		},
		{
			name: "exclusive rejection is visible fallback",
			observation: SignalPathObservation{
				Requested: ModeExclusiveProcessed,
				Output:    OutputState{ExclusiveKnown: true},
			},
			wantMode:     EffectiveFallback,
			wantVerified: VerificationConfigured,
		},
		{
			name: "remote renderer never claims the local DAC path",
			observation: SignalPathObservation{
				Requested:      ModeNormal,
				RemoteRenderer: true,
			},
			wantMode:     EffectiveRemoteRenderer,
			wantVerified: VerificationConfigured,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := ReduceSignalPath(test.observation)
			if snapshot.Effective != test.wantMode {
				t.Fatalf("effective mode = %q, want %q", snapshot.Effective, test.wantMode)
			}
			if snapshot.Verification != test.wantVerified {
				t.Fatalf("verification = %q, want %q", snapshot.Verification, test.wantVerified)
			}
			if test.wantIssue != "" && !hasIssue(snapshot, test.wantIssue) {
				t.Fatalf("issues = %#v, want %q", snapshot.Issues, test.wantIssue)
			}
			if snapshot.IsBitPerfect() {
				t.Fatal("current playback mode unexpectedly claimed bit-perfect output")
			}
		})
	}
}

func TestReduceSignalPathStrictPCMRequiresEveryObservedInvariant(t *testing.T) {
	base := strictPCMObservation()
	tests := []struct {
		name      string
		mutate    func(*SignalPathObservation)
		wantIssue FallbackReasonCode
	}{
		{"decoder unavailable", func(o *SignalPathObservation) { o.Decoder.Available = false }, FallbackEffectiveModeUnknown},
		{"lossy decoder", func(o *SignalPathObservation) { o.Decoder.Lossless = false }, FallbackSourceLossy},
		{"exclusive unknown", func(o *SignalPathObservation) { o.Output.ExclusiveKnown = false }, FallbackEffectiveModeUnknown},
		{"exclusive denied", func(o *SignalPathObservation) { o.Output.Exclusive = false }, FallbackExclusiveDenied},
		{"device unknown", func(o *SignalPathObservation) { o.Device.IdentityKnown = false }, FallbackDeviceMissing},
		{"wrong device", func(o *SignalPathObservation) { o.Device.EffectiveID = "other" }, FallbackDeviceMissing},
		{"format unknown", func(o *SignalPathObservation) { o.Output.FormatKnown = false }, FallbackEffectiveModeUnknown},
		{"rate mismatch", func(o *SignalPathObservation) { o.Output.Format.SampleRate = 48000 }, FallbackRateMismatch},
		{"bit depth mismatch", func(o *SignalPathObservation) { o.Output.Format.BitDepth = 16 }, FallbackBitDepthMismatch},
		{"channels mismatch", func(o *SignalPathObservation) { o.Output.Format.Channels = 1 }, FallbackChannelMappingRequired},
		{"processing unknown", func(o *SignalPathObservation) { o.Output.ProcessingKnown = false }, FallbackEffectiveModeUnknown},
		{"resampler active", func(o *SignalPathObservation) {
			o.Processing = []ProcessingStage{{Kind: ProcessingResampler, Active: true, ChangesSamples: true}}
		}, FallbackResamplerActive},
		{"software gain active", func(o *SignalPathObservation) {
			o.Processing = []ProcessingStage{{Kind: ProcessingReplayGain, Active: true, ChangesSamples: true}}
		}, FallbackSoftwareGainActive},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := base
			test.mutate(&observation)
			snapshot := ReduceSignalPath(observation)
			if snapshot.Effective != EffectiveStrictUnverified {
				t.Fatalf("effective = %q, want strict_unverified", snapshot.Effective)
			}
			if !hasIssue(snapshot, test.wantIssue) {
				t.Fatalf("issues = %#v, want %q", snapshot.Issues, test.wantIssue)
			}
			if snapshot.IsBitPerfect() {
				t.Fatal("failed invariant still produced a bit-perfect claim")
			}
		})
	}
}

func TestReduceSignalPathPromotesOnlyObservedStrictPCM(t *testing.T) {
	configured := strictPCMObservation()
	configured.RuntimeObserved = false
	configured.Negotiated = true
	if snapshot := ReduceSignalPath(configured); snapshot.Effective != EffectiveStrictUnverified {
		t.Fatalf("negotiated-only mode = %q, want strict_unverified", snapshot.Effective)
	}

	observed := strictPCMObservation()
	snapshot := ReduceSignalPath(observed)
	if snapshot.Effective != EffectiveStrictPCM || !snapshot.IsBitPerfect() {
		t.Fatalf("observed strict snapshot was not promoted: %#v", snapshot)
	}
	if snapshot.Transparency != TransparencyOriginal {
		t.Fatalf("transparency = %q, want original_source", snapshot.Transparency)
	}

	observed.Receipt.Provenance = ProvenanceDeliveredOnly
	snapshot = ReduceSignalPath(observed)
	if !snapshot.IsBitPerfect() || snapshot.Transparency != TransparencyDelivered {
		t.Fatalf("delivered-stream transparency was not retained: %#v", snapshot)
	}
}

func TestReduceSignalPathDoPRejectsBadMarkersAndFill(t *testing.T) {
	observation := strictPCMObservation()
	observation.Decoder.RawDSD = true
	observation.Receipt.RawDSD = true
	observation.Output.DoP = true
	observation.Output.DoPMarkersKnown = true
	observation.Output.DoPMarkersValid = true
	observation.Output.InjectedFillFrames = 1

	snapshot := ReduceSignalPath(observation)
	if snapshot.Effective != EffectiveStrictUnverified || !hasIssue(snapshot, FallbackUnderrunFillInjected) {
		t.Fatalf("DoP fill did not invalidate strict state: %#v", snapshot)
	}

	observation.Output.InjectedFillFrames = 0
	snapshot = ReduceSignalPath(observation)
	if snapshot.Effective != EffectiveStrictDoP || !snapshot.IsBitPerfect() {
		t.Fatalf("valid observed DoP was not promoted: %#v", snapshot)
	}
}

func TestSignalPathStatePublishesIndependentSnapshots(t *testing.T) {
	state := NewSignalPathState(PlaybackSnapshot{
		Processing: []ProcessingStage{{Kind: ProcessingAnalyzer}},
	})
	callbackRan := make(chan PlaybackSnapshot, 1)
	state.OnChange(func(snapshot PlaybackSnapshot) {
		snapshot.Processing[0].Kind = ProcessingEqualizer
		callbackRan <- snapshot
	})

	state.Update(func(snapshot *PlaybackSnapshot) {
		snapshot.Generation = 2
	})

	select {
	case <-callbackRan:
	case <-time.After(time.Second):
		t.Fatal("signal-path callback did not run")
	}
	current := state.Snapshot()
	if current.Generation != 2 {
		t.Fatalf("generation = %d, want 2", current.Generation)
	}
	if current.Processing[0].Kind != ProcessingAnalyzer {
		t.Fatal("callback mutated the stored snapshot")
	}
}

func strictPCMObservation() SignalPathObservation {
	format := AudioFormat{SampleFormat: "s32", SampleRate: 96000, BitDepth: 24, Channels: 2}
	return SignalPathObservation{
		Requested: ModeStrict,
		Receipt: SourceReceipt{
			Provenance:        ProvenanceOriginalConfirmed,
			DeliveredLossless: true,
			LosslessKnown:     true,
		},
		Decoder: DecoderState{Available: true, Lossless: true, Format: format},
		Output: OutputState{
			Exclusive:       true,
			ExclusiveKnown:  true,
			Format:          format,
			FormatKnown:     true,
			ProcessingKnown: true,
		},
		Device: DeviceState{
			RequestedID:   "dac",
			EffectiveID:   "dac",
			IdentityKnown: true,
		},
		RuntimeObserved: true,
	}
}

func hasIssue(snapshot PlaybackSnapshot, code FallbackReasonCode) bool {
	for _, issue := range snapshot.Issues {
		if issue.Code == code {
			return true
		}
	}
	return snapshot.Fallback != nil && snapshot.Fallback.Code == code
}
