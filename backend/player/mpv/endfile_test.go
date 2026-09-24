package mpv

import (
	"testing"

	"github.com/supersonic-app/supersonic/backend/player"
)

func TestIsPrematureEOF(t *testing.T) {
	tests := []struct {
		name   string
		status player.Status
		want   bool
	}{
		{
			name:   "network stream ended early",
			status: player.Status{TimePos: 26.38, Duration: 274.11},
			want:   true,
		},
		{
			name:   "normal end within tolerance",
			status: player.Status{TimePos: 299, Duration: 300},
			want:   false,
		},
		{
			name:   "unknown duration",
			status: player.Status{TimePos: 26.38},
			want:   false,
		},
		{
			name:   "invalid negative position",
			status: player.Status{TimePos: -1, Duration: 300},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPrematureEOF(tt.status); got != tt.want {
				t.Fatalf("isPrematureEOF(%+v) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
