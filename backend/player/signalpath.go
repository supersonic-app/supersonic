package player

import (
	"slices"
	"sync"
	"time"
)

type AudioFormat struct {
	SampleFormat string
	SampleRate   int
	BitDepth     int
	Channels     int
}

type DecoderState struct {
	Available bool
	Codec     string
	Bitrate   int
	Format    AudioFormat
	Lossless  bool
	RawDSD    bool
}

type ProcessingKind string

const (
	ProcessingResampler      ProcessingKind = "resampler"
	ProcessingSoftwareVolume ProcessingKind = "software_volume"
	ProcessingReplayGain     ProcessingKind = "replay_gain"
	ProcessingEqualizer      ProcessingKind = "equalizer"
	ProcessingChannelMix     ProcessingKind = "channel_mix"
	ProcessingFade           ProcessingKind = "fade"
	ProcessingAnalyzer       ProcessingKind = "analyzer"
)

type ProcessingStage struct {
	Kind           ProcessingKind
	Active         bool
	ChangesSamples bool
	Reason         string
}

type OutputState struct {
	Backend            string
	DeviceID           string
	Format             AudioFormat
	Exclusive          bool
	ExclusiveKnown     bool
	FormatKnown        bool
	ProcessingKnown    bool
	DoP                bool
	DoPMarkersKnown    bool
	DoPMarkersValid    bool
	InjectedFillFrames uint64
}

type DeviceState struct {
	RequestedID   string
	EffectiveID   string
	Name          string
	IdentityKnown bool
}

type TransparencyScope string

const (
	TransparencyNone      TransparencyScope = "none"
	TransparencyDelivered TransparencyScope = "delivered_stream"
	TransparencyOriginal  TransparencyScope = "original_source"
)

type PlaybackSnapshot struct {
	Requested    RequestedMode
	Effective    EffectiveMode
	Verification VerificationLevel
	Transparency TransparencyScope
	Source       SourceDescriptor
	Receipt      SourceReceipt
	Decoder      DecoderState
	Processing   []ProcessingStage
	Output       OutputState
	Device       DeviceState
	Fallback     *FallbackReason
	Issues       []FallbackReason
	Generation   uint64
	UpdatedAt    time.Time
}

func (s PlaybackSnapshot) Clone() PlaybackSnapshot {
	s.Receipt = s.Receipt.Clone()
	s.Processing = slices.Clone(s.Processing)
	s.Issues = slices.Clone(s.Issues)
	if s.Fallback != nil {
		fallback := *s.Fallback
		s.Fallback = &fallback
	}
	return s
}

func (s PlaybackSnapshot) IsBitPerfect() bool {
	return (s.Effective == EffectiveStrictPCM || s.Effective == EffectiveStrictDoP || s.Effective == EffectiveNativeDSD) &&
		s.Verification.AtLeast(VerificationObserved) && len(s.Issues) == 0 && s.Fallback == nil
}

type SignalPathObservation struct {
	Requested        RequestedMode
	Source           SourceDescriptor
	Receipt          SourceReceipt
	Decoder          DecoderState
	Processing       []ProcessingStage
	Output           OutputState
	Device           DeviceState
	RemoteRenderer   bool
	Negotiated       bool
	RuntimeObserved  bool
	HardwareVerified bool
	Fallback         *FallbackReason
	Generation       uint64
}

// ReduceSignalPath is the only place that promotes observations into product
// claims. Unknown facts fail closed instead of being treated as successful.
func ReduceSignalPath(observation SignalPathObservation) PlaybackSnapshot {
	snapshot := PlaybackSnapshot{
		Requested:    observation.Requested,
		Verification: verificationForObservation(observation),
		Source:       observation.Source,
		Receipt:      observation.Receipt.Clone(),
		Decoder:      observation.Decoder,
		Processing:   slices.Clone(observation.Processing),
		Output:       observation.Output,
		Device:       observation.Device,
		Generation:   observation.Generation,
		UpdatedAt:    time.Now().UTC(),
	}
	if observation.Fallback != nil {
		fallback := *observation.Fallback
		snapshot.Fallback = &fallback
		snapshot.Effective = EffectiveFallback
		return snapshot
	}
	if observation.RemoteRenderer {
		snapshot.Effective = EffectiveRemoteRenderer
		snapshot.Transparency = TransparencyNone
		return snapshot
	}

	switch observation.Requested {
	case ModeNormal:
		snapshot.Effective = EffectiveSharedProcessed
		if observation.Output.ExclusiveKnown && observation.Output.Exclusive {
			snapshot.Effective = EffectiveExclusiveProcessed
		}
	case ModeExclusiveProcessed:
		snapshot.Effective = reduceExclusiveProcessed(&snapshot)
	case ModeStrict:
		reduceStrict(&snapshot)
	default:
		snapshot.Effective = EffectiveUnsupported
		snapshot.Issues = append(snapshot.Issues, FallbackReason{
			Code:   FallbackEffectiveModeUnknown,
			Detail: "unknown requested playback mode",
		})
	}
	return snapshot
}

func verificationForObservation(observation SignalPathObservation) VerificationLevel {
	switch {
	case observation.HardwareVerified:
		return VerificationHardwareVerified
	case observation.RuntimeObserved:
		return VerificationObserved
	case observation.Negotiated:
		return VerificationNegotiated
	default:
		return VerificationConfigured
	}
}

func reduceExclusiveProcessed(snapshot *PlaybackSnapshot) EffectiveMode {
	if !snapshot.Output.ExclusiveKnown {
		snapshot.Issues = append(snapshot.Issues, FallbackReason{
			Code:   FallbackEffectiveModeUnknown,
			Detail: "the output backend has not reported effective exclusive state",
		})
		return EffectiveUnverified
	}
	if !snapshot.Output.Exclusive {
		snapshot.Fallback = &FallbackReason{
			Code:   FallbackExclusiveDenied,
			Detail: "the output backend is not exclusive",
		}
		return EffectiveFallback
	}
	return EffectiveExclusiveProcessed
}

func reduceStrict(snapshot *PlaybackSnapshot) {
	if snapshot.Decoder.RawDSD || snapshot.Receipt.RawDSD {
		snapshot.Issues = strictDoPIssues(*snapshot)
		if len(snapshot.Issues) == 0 && snapshot.Verification.AtLeast(VerificationObserved) {
			snapshot.Effective = EffectiveStrictDoP
			snapshot.Transparency = transparencyForReceipt(snapshot.Receipt)
			return
		}
	} else {
		snapshot.Issues = strictPCMIssues(*snapshot)
		if len(snapshot.Issues) == 0 && snapshot.Verification.AtLeast(VerificationObserved) {
			snapshot.Effective = EffectiveStrictPCM
			snapshot.Transparency = transparencyForReceipt(snapshot.Receipt)
			return
		}
	}
	if len(snapshot.Issues) == 0 {
		snapshot.Issues = append(snapshot.Issues, FallbackReason{
			Code:   FallbackEffectiveModeUnknown,
			Detail: "strict playback has not been observed at runtime",
		})
	}
	snapshot.Effective = EffectiveStrictUnverified
}

func strictPCMIssues(snapshot PlaybackSnapshot) []FallbackReason {
	issues := make([]FallbackReason, 0, 4)
	if !snapshot.Decoder.Available {
		issues = append(issues, FallbackReason{Code: FallbackEffectiveModeUnknown, Detail: "decoder output is unknown"})
	} else if !snapshot.Decoder.Lossless {
		issues = append(issues, FallbackReason{Code: FallbackSourceLossy, Detail: "decoder output is not known lossless PCM"})
	}
	issues = append(issues, strictOutputIssues(snapshot)...)
	return issues
}

func strictDoPIssues(snapshot PlaybackSnapshot) []FallbackReason {
	issues := make([]FallbackReason, 0, 4)
	if !snapshot.Decoder.RawDSD && !snapshot.Receipt.RawDSD {
		issues = append(issues, FallbackReason{Code: FallbackRawDSDUnavailable, Detail: "raw DSD source is unavailable"})
	}
	if !snapshot.Output.DoP {
		issues = append(issues, FallbackReason{Code: FallbackDoPCarrierUnsupported, Detail: "DoP carrier is not active"})
	}
	if !snapshot.Output.DoPMarkersKnown || !snapshot.Output.DoPMarkersValid {
		issues = append(issues, FallbackReason{Code: FallbackEffectiveModeUnknown, Detail: "DoP marker continuity is not verified"})
	}
	if snapshot.Output.InjectedFillFrames > 0 {
		issues = append(issues, FallbackReason{Code: FallbackUnderrunFillInjected, Detail: "the output contains injected DoP mute frames"})
	}
	issues = append(issues, strictOutputIssues(snapshot)...)
	return issues
}

func strictOutputIssues(snapshot PlaybackSnapshot) []FallbackReason {
	issues := make([]FallbackReason, 0, 6)
	if !snapshot.Output.ExclusiveKnown {
		issues = append(issues, FallbackReason{Code: FallbackEffectiveModeUnknown, Detail: "effective exclusive state is unknown"})
	} else if !snapshot.Output.Exclusive {
		issues = append(issues, FallbackReason{Code: FallbackExclusiveDenied, Detail: "the output backend is not exclusive"})
	}
	if !snapshot.Device.IdentityKnown {
		issues = append(issues, FallbackReason{Code: FallbackDeviceMissing, Detail: "effective device identity is unknown"})
	} else if snapshot.Device.RequestedID != "" && snapshot.Device.RequestedID != snapshot.Device.EffectiveID {
		issues = append(issues, FallbackReason{Code: FallbackDeviceMissing, Detail: "effective device differs from the requested device"})
	}
	if !snapshot.Output.FormatKnown {
		issues = append(issues, FallbackReason{Code: FallbackEffectiveModeUnknown, Detail: "effective output format is unknown"})
	} else if snapshot.Decoder.Available && !snapshot.Decoder.RawDSD {
		if snapshot.Decoder.Format.SampleRate != snapshot.Output.Format.SampleRate {
			issues = append(issues, FallbackReason{Code: FallbackRateMismatch, Detail: "decoder and output sample rates differ"})
		}
		if snapshot.Decoder.Format.BitDepth != snapshot.Output.Format.BitDepth {
			issues = append(issues, FallbackReason{Code: FallbackBitDepthMismatch, Detail: "decoder and output bit depths differ"})
		}
		if snapshot.Decoder.Format.Channels != snapshot.Output.Format.Channels {
			issues = append(issues, FallbackReason{Code: FallbackChannelMappingRequired, Detail: "decoder and output channel counts differ"})
		}
	}
	if !snapshot.Output.ProcessingKnown {
		issues = append(issues, FallbackReason{Code: FallbackEffectiveModeUnknown, Detail: "active processing stages are unknown"})
	} else {
		for _, stage := range snapshot.Processing {
			if stage.Active && stage.ChangesSamples {
				issues = append(issues, reasonForProcessing(stage))
			}
		}
	}
	return issues
}

func reasonForProcessing(stage ProcessingStage) FallbackReason {
	code := FallbackFilterActive
	switch stage.Kind {
	case ProcessingResampler:
		code = FallbackResamplerActive
	case ProcessingSoftwareVolume, ProcessingReplayGain, ProcessingFade:
		code = FallbackSoftwareGainActive
	case ProcessingChannelMix:
		code = FallbackChannelMappingRequired
	}
	return FallbackReason{Code: code, Detail: stage.Reason}
}

func transparencyForReceipt(receipt SourceReceipt) TransparencyScope {
	if receipt.Provenance == ProvenanceOriginalConfirmed {
		return TransparencyOriginal
	}
	return TransparencyDelivered
}

// SignalPathState owns an immutable current snapshot. Callbacks are invoked
// after releasing the lock and receive independent copies.
type SignalPathState struct {
	mu        sync.RWMutex
	snapshot  PlaybackSnapshot
	callbacks []func(PlaybackSnapshot)
}

func NewSignalPathState(initial PlaybackSnapshot) *SignalPathState {
	if initial.UpdatedAt.IsZero() {
		initial.UpdatedAt = time.Now().UTC()
	}
	return &SignalPathState{snapshot: initial.Clone()}
}

func (s *SignalPathState) Snapshot() PlaybackSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot.Clone()
}

func (s *SignalPathState) Update(update func(*PlaybackSnapshot)) {
	s.mu.Lock()
	next := s.snapshot.Clone()
	update(&next)
	next.UpdatedAt = time.Now().UTC()
	s.snapshot = next.Clone()
	callbacks := slices.Clone(s.callbacks)
	s.mu.Unlock()

	for _, callback := range callbacks {
		callback(next.Clone())
	}
}

func (s *SignalPathState) OnChange(callback func(PlaybackSnapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if callback == nil {
		s.callbacks = nil
		return
	}
	s.callbacks = append(s.callbacks, callback)
}

type SignalPathProvider interface {
	SignalPathSnapshot() PlaybackSnapshot
	OnSignalPathChange(func(PlaybackSnapshot))
}
