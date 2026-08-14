package player

import "testing"

func TestPlaybackCapabilitiesCombinesStrictTransports(t *testing.T) {
	tests := []struct {
		name string
		pcm  CapabilityStatus
		dop  CapabilityStatus
		want CapabilityStatus
	}{
		{"supported PCM", CapabilitySupported, CapabilityUnsupported, CapabilitySupported},
		{"supported DoP", CapabilityUnsupported, CapabilitySupported, CapabilitySupported},
		{"unverified PCM", CapabilityUnverified, CapabilityUnsupported, CapabilityUnverified},
		{"unsupported", CapabilityUnsupported, CapabilityUnsupported, CapabilityUnsupported},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capabilities := PlaybackCapabilities{
				StrictPCM: ModeCapability{Status: test.pcm},
				StrictDoP: ModeCapability{Status: test.dop},
			}
			if got := capabilities.ForMode(ModeStrict).Status; got != test.want {
				t.Fatalf("strict status = %q, want %q", got, test.want)
			}
		})
	}
}
