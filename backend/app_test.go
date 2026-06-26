package backend

import (
	"os"
	"testing"
)

func TestResolveHTTPProxy(t *testing.T) {
	// Save original environment
	origHTTPSProxy := os.Getenv("https_proxy")
	origHTTPSPROXY := os.Getenv("HTTPS_PROXY")
	defer func() {
		os.Setenv("https_proxy", origHTTPSProxy)
		os.Setenv("HTTPS_PROXY", origHTTPSPROXY)
	}()

	// Test case 1: No proxy set
	os.Unsetenv("https_proxy")
	os.Unsetenv("HTTPS_PROXY")
	
	cfg := LocalPlaybackConfig{}
	if proxy := resolveHTTPProxy(cfg); proxy != "" {
		t.Errorf("Test 1: Expected empty proxy, got '%s'", proxy)
	}

	// Test case 2: Set https_proxy
	os.Setenv("https_proxy", "http://proxy:8080")
	if proxy := resolveHTTPProxy(cfg); proxy != "http://proxy:8080" {
		t.Errorf("Test 2: Expected 'http://proxy:8080', got '%s'", proxy)
	}

	// Test case 3: Set HTTPS_PROXY (uppercase)
	os.Unsetenv("https_proxy")
	os.Setenv("HTTPS_PROXY", "http://proxy2:8080")
	if proxy := resolveHTTPProxy(cfg); proxy != "http://proxy2:8080" {
		t.Errorf("Test 3: Expected 'http://proxy2:8080', got '%s'", proxy)
	}

	// Test case 4: Config file takes precedence
	cfg.HTTPProxy = "http://config-proxy:8080"
	if proxy := resolveHTTPProxy(cfg); proxy != "http://config-proxy:8080" {
		t.Errorf("Test 4: Expected 'http://config-proxy:8080', got '%s'", proxy)
	}

	// Test case 5: Config precedence over both env vars
	os.Setenv("https_proxy", "http://env-proxy1:8080")
	os.Setenv("HTTPS_PROXY", "http://env-proxy2:8080")
	if proxy := resolveHTTPProxy(cfg); proxy != "http://config-proxy:8080" {
		t.Errorf("Test 5: Expected 'http://config-proxy:8080', got '%s'", proxy)
	}
}

func TestRedactProxyURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL with credentials",
			input:    "http://user:pass@proxy:8080",
			expected: "http://proxy:8080",
		},
		{
			name:     "URL without credentials",
			input:    "http://proxy:8080",
			expected: "http://proxy:8080",
		},
		{
			name:     "HTTPS URL with credentials",
			input:    "https://user:pass@proxy:8443",
			expected: "https://proxy:8443",
		},
		{
			name:     "Invalid URL",
			input:    "://invalid",
			expected: "<invalid URL>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactProxyURL(tt.input); got != tt.expected {
				t.Errorf("redactProxyURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}