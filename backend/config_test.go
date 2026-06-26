package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPProxyConfigSerialization(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	configContent := `
[LocalPlayback]
HTTPProxy = "http://user:pass@proxy:8080"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := ReadConfigFile(configPath, "test")
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	if cfg.LocalPlayback.HTTPProxy != "http://user:pass@proxy:8080" {
		t.Errorf("Expected HTTPProxy 'http://user:pass@proxy:8080', got '%s'", cfg.LocalPlayback.HTTPProxy)
	}
}

func TestHTTPProxyConfigDefault(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	configContent := `
[LocalPlayback]
Volume = 50
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := ReadConfigFile(configPath, "test")
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	if cfg.LocalPlayback.HTTPProxy != "" {
		t.Errorf("Expected empty HTTPProxy, got '%s'", cfg.LocalPlayback.HTTPProxy)
	}
}
