package backend

import (
	"testing"

	"github.com/supersonic-app/supersonic/backend/player"
)

func TestStreamRequestForConfig(t *testing.T) {
	tests := []struct {
		name         string
		config       TranscodingConfig
		wantPolicy   player.DeliveryPolicy
		wantForceRaw bool
		wantCodec    string
		wantBitrate  int
	}{
		{
			name:       "server default",
			wantPolicy: player.DeliveryServerDefault,
		},
		{
			name: "raw",
			config: TranscodingConfig{
				ForceRawFile: true,
			},
			wantPolicy:   player.DeliveryRawRequested,
			wantForceRaw: true,
		},
		{
			name: "explicit transcode takes precedence",
			config: TranscodingConfig{
				ForceRawFile:     true,
				RequestTranscode: true,
				Codec:            "opus",
				MaxBitRateKBPS:   320,
			},
			wantPolicy:  player.DeliveryTranscodeRequested,
			wantCodec:   "opus",
			wantBitrate: 320,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transcode, forceRaw, policy := streamRequestForConfig(test.config)
			if policy != test.wantPolicy || forceRaw != test.wantForceRaw {
				t.Fatalf("policy/forceRaw = %q/%t, want %q/%t", policy, forceRaw, test.wantPolicy, test.wantForceRaw)
			}
			if test.wantCodec == "" {
				if transcode != nil {
					t.Fatalf("transcode = %#v, want nil", transcode)
				}
				return
			}
			if transcode == nil || transcode.Codec != test.wantCodec || transcode.BitRateKBPS != test.wantBitrate {
				t.Fatalf("transcode = %#v, want %s at %d kbps", transcode, test.wantCodec, test.wantBitrate)
			}
		})
	}
}
