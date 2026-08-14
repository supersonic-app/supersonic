package player

import "slices"

// DeliveryPolicy records how the client asked the media provider to deliver a
// source. It must not be inferred later from a signed URL.
type DeliveryPolicy string

const (
	DeliveryRawRequested       DeliveryPolicy = "raw"
	DeliveryTranscodeRequested DeliveryPolicy = "transcode_requested"
	DeliveryServerDefault      DeliveryPolicy = "server_default"
	DeliveryExternalStream     DeliveryPolicy = "external_stream"
	DeliveryRemoteControlled   DeliveryPolicy = "remote_controlled"
)

type SourceProvenance string

const (
	ProvenanceUnknown             SourceProvenance = "unknown"
	ProvenanceOriginalConfirmed   SourceProvenance = "original_confirmed"
	ProvenanceTranscodedConfirmed SourceProvenance = "transcoded_confirmed"
	ProvenanceDeliveredOnly       SourceProvenance = "delivered_only"
)

type EvidenceItem struct {
	Kind  string
	Value string
}

// SourceDescriptor contains server-library metadata. These fields describe the
// stored object and must not be presented as facts about delivered bytes.
type SourceDescriptor struct {
	ServerID        string
	TrackID         string
	LibraryCodec    string
	LibrarySuffix   string
	LibrarySize     int64
	LibraryRate     int
	LibraryBits     int
	LibraryChannels int
}

// SourceReceipt contains evidence about the bytes delivered for one playback
// request. URLs and credentials are deliberately excluded.
type SourceReceipt struct {
	DeliveryPolicy     DeliveryPolicy
	Provenance         SourceProvenance
	DeliveredCodec     string
	Container          string
	DeliveredRate      int
	DeliveredBits      int
	DeliveredChannels  int
	ContentLength      int64
	RangeSupported     bool
	CompleteCache      bool
	DeliveredLossless  bool
	LosslessKnown      bool
	RawDSD             bool
	ContentFingerprint string
	Evidence           []EvidenceItem
}

// PlaybackSource keeps the transport URL beside, but outside, the immutable
// diagnostic receipt. Snapshots copy only Descriptor and Receipt.
type PlaybackSource struct {
	URL        string
	Descriptor SourceDescriptor
	Receipt    SourceReceipt
}

func (r SourceReceipt) Clone() SourceReceipt {
	r.Evidence = slices.Clone(r.Evidence)
	return r
}

func (s PlaybackSource) Clone() PlaybackSource {
	s.Receipt = s.Receipt.Clone()
	return s
}

func NewSourceReceipt(policy DeliveryPolicy) SourceReceipt {
	provenance := ProvenanceUnknown
	if policy == DeliveryTranscodeRequested {
		provenance = ProvenanceTranscodedConfirmed
	}
	return SourceReceipt{
		DeliveryPolicy: policy,
		Provenance:     provenance,
		Evidence: []EvidenceItem{{
			Kind:  "delivery_policy",
			Value: string(policy),
		}},
	}
}
