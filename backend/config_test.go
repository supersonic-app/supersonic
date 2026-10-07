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
[Application]
HTTPProxy = "http://user:pass@proxy:8080"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := ReadConfigFile(configPath, "test")
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	if cfg.Application.HTTPProxy != "http://user:pass@proxy:8080" {
		t.Errorf("Expected HTTPProxy 'http://user:pass@proxy:8080', got '%s'", cfg.Application.HTTPProxy)
	}
}

func TestHTTPProxyConfigDefault(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	configContent := `
[Application]
Language = "auto"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := ReadConfigFile(configPath, "test")
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	if cfg.Application.HTTPProxy != "" {
		t.Errorf("Expected empty HTTPProxy, got '%s'", cfg.Application.HTTPProxy)
	}
}
