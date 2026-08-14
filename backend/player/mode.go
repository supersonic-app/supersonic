package player

// RequestedMode describes the playback policy selected by the user. It is an
// intent, not proof that the output path actually satisfies that policy.
type RequestedMode string

const (
	ModeNormal             RequestedMode = "normal"
	ModeExclusiveProcessed RequestedMode = "exclusive_processed"
	ModeStrict             RequestedMode = "strict_auto"
)

// EffectiveMode describes what the active signal path is known to be doing.
// EffectiveUnverified is intentionally distinct from a fallback: the player
// has not observed enough runtime facts to claim either success or failure.
type EffectiveMode string

const (
	EffectiveUnverified         EffectiveMode = "unverified"
	EffectiveSharedProcessed    EffectiveMode = "shared_processed"
	EffectiveExclusiveProcessed EffectiveMode = "exclusive_processed"
	EffectiveStrictPCM          EffectiveMode = "strict_pcm"
	EffectiveStrictDoP          EffectiveMode = "strict_dop"
	EffectiveNativeDSD          EffectiveMode = "native_dsd"
	EffectiveConvertedDSD       EffectiveMode = "converted_dsd_pcm"
	EffectiveStrictUnverified   EffectiveMode = "strict_unverified"
	EffectiveFallback           EffectiveMode = "fallback_processed"
	EffectiveUnsupported        EffectiveMode = "unsupported"
	EffectiveRemoteRenderer     EffectiveMode = "remote_renderer"
)

// VerificationLevel records the strongest evidence available for the current
// output generation. Levels are ordered by evidence strength.
type VerificationLevel string

const (
	VerificationUnknown          VerificationLevel = "unknown"
	VerificationConfigured       VerificationLevel = "configured"
	VerificationNegotiated       VerificationLevel = "negotiated"
	VerificationObserved         VerificationLevel = "observed"
	VerificationHardwareVerified VerificationLevel = "hardware_verified"
)

func (v VerificationLevel) AtLeast(other VerificationLevel) bool {
	return verificationRank(v) >= verificationRank(other)
}

func verificationRank(v VerificationLevel) int {
	switch v {
	case VerificationConfigured:
		return 1
	case VerificationNegotiated:
		return 2
	case VerificationObserved:
		return 3
	case VerificationHardwareVerified:
		return 4
	default:
		return 0
	}
}

type FallbackPolicy string

const (
	FallbackVisible FallbackPolicy = "allow_visible_fallback"
	FallbackStop    FallbackPolicy = "stop_on_failure"
)

type FallbackReasonCode string

const (
	FallbackServerTranscoded           FallbackReasonCode = "server_transcoded"
	FallbackSourceLossy                FallbackReasonCode = "source_lossy"
	FallbackDeviceMissing              FallbackReasonCode = "device_missing"
	FallbackExclusiveDenied            FallbackReasonCode = "exclusive_denied"
	FallbackEffectiveModeUnknown       FallbackReasonCode = "effective_mode_unknown"
	FallbackFormatUnsupported          FallbackReasonCode = "format_unsupported"
	FallbackRateMismatch               FallbackReasonCode = "rate_mismatch"
	FallbackBitDepthMismatch           FallbackReasonCode = "bit_depth_mismatch"
	FallbackChannelMappingRequired     FallbackReasonCode = "channel_mapping_required"
	FallbackResamplerActive            FallbackReasonCode = "resampler_active"
	FallbackSoftwareGainActive         FallbackReasonCode = "software_gain_active"
	FallbackFilterActive               FallbackReasonCode = "filter_active"
	FallbackDoPCarrierUnsupported      FallbackReasonCode = "dop_carrier_unsupported"
	FallbackRawDSDUnavailable          FallbackReasonCode = "raw_dsd_unavailable"
	FallbackDeviceDisconnected         FallbackReasonCode = "device_disconnected"
	FallbackUnderrunFillInjected       FallbackReasonCode = "underrun_fill_injected"
	FallbackRemoteRendererUnverifiable FallbackReasonCode = "remote_renderer_unverifiable"
)

type FallbackReason struct {
	Code   FallbackReasonCode
	Detail string
}
