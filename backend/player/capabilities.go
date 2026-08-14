package player

type CapabilityStatus string

const (
	CapabilityUnsupported CapabilityStatus = "unsupported"
	CapabilityUnverified  CapabilityStatus = "unverified"
	CapabilitySupported   CapabilityStatus = "supported"
)

type ModeCapability struct {
	Status CapabilityStatus
	Reason string
}

// PlaybackCapabilities is intentionally about provable product modes rather
// than a backend's raw option list.
type PlaybackCapabilities struct {
	EngineID           string
	Normal             ModeCapability
	ExclusiveProcessed ModeCapability
	StrictPCM          ModeCapability
	StrictDoP          ModeCapability
	RemoteRenderer     ModeCapability
}

func (c PlaybackCapabilities) ForMode(mode RequestedMode) ModeCapability {
	switch mode {
	case ModeNormal:
		return c.Normal
	case ModeExclusiveProcessed:
		return c.ExclusiveProcessed
	case ModeStrict:
		if c.StrictPCM.Status == CapabilitySupported || c.StrictDoP.Status == CapabilitySupported {
			return ModeCapability{Status: CapabilitySupported}
		}
		if c.StrictPCM.Status == CapabilityUnverified || c.StrictDoP.Status == CapabilityUnverified {
			return ModeCapability{Status: CapabilityUnverified, Reason: "strict output requires runtime verification"}
		}
		return ModeCapability{Status: CapabilityUnsupported, Reason: "strict output is unsupported"}
	default:
		return ModeCapability{Status: CapabilityUnsupported, Reason: "unknown requested mode"}
	}
}

type CapabilityProvider interface {
	PlaybackCapabilities() PlaybackCapabilities
}
