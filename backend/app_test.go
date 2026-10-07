package backend

import "testing"

func TestRedactProxyURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"credentials", "http://user:pass@proxy:8080", "http://proxy:8080"},
		{"without credentials", "http://proxy:8080", "http://proxy:8080"},
		{"https credentials", "https://user:pass@proxy:8443", "https://proxy:8443"},
		{"invalid", "://invalid", "<invalid URL>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactProxyURL(tt.input); got != tt.want {
				t.Fatalf("redactProxyURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
