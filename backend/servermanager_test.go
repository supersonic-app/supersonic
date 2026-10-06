package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNormalizeServerURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"http://192.168.1.1:8096", "http://192.168.1.1:8096"},
		{"https://music.example.com", "https://music.example.com"},
		{"192.168.1.1:4533", "http://192.168.1.1:4533"},
		{"music.example.com", "http://music.example.com"},
		{"http://192.168.1.1:8096/", "http://192.168.1.1:8096"},
		{"http://192.168.1.1:8096///", "http://192.168.1.1:8096"},
		{"192.168.1.1:8096/", "http://192.168.1.1:8096"},
	}
	for _, tt := range tests {
		got := NormalizeServerURL(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeServerURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeJellyfinURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"http://192.168.1.1:8096", "http://192.168.1.1:8096"},
		{"192.168.1.1:8096", "http://192.168.1.1:8096"},
		{"192.168.1.1:8096/", "http://192.168.1.1:8096"},
		{"192.168.1.1:8096/web/index.html", "http://192.168.1.1:8096"},
		{"http://192.168.1.1:8096/web/index.html", "http://192.168.1.1:8096"},
		{"http://192.168.1.1:8096/web/", "http://192.168.1.1:8096"},
		{"http://192.168.1.1:8096/web", "http://192.168.1.1:8096"},
		{"https://jellyfin.example.com/web/index.html", "https://jellyfin.example.com"},
		{"https://jellyfin.example.com/web/", "https://jellyfin.example.com"},
	}
	for _, tt := range tests {
		got := NormalizeJellyfinURL(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeJellyfinURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCertFileStem(t *testing.T) {
	id := uuid.New()
	if got := certFileStem(id, "/tmp/carlson.p12"); got != id.String() {
		t.Errorf("certFileStem with server ID = %q, want %q", got, id.String())
	}

	// Without a server ID the path must not leak into the file name.
	stem := certFileStem(uuid.Nil, "/home/alice/secret/carlson.pfx")
	if stem == "" || strings.Contains(stem, "alice") || strings.Contains(stem, "/") {
		t.Errorf("certFileStem leaked the path: %q", stem)
	}
	if stem != certFileStem(uuid.Nil, "/home/alice/secret/carlson.pfx") {
		t.Error("certFileStem is not deterministic")
	}
	if stem == certFileStem(uuid.Nil, "/home/alice/secret/other.pfx") {
		t.Error("certFileStem collided for different paths")
	}
}

func TestCertTempBasePrefersXDGRuntimeDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	if got := certTempBase(); got != dir {
		t.Errorf("certTempBase() = %q, want %q", got, dir)
	}

	t.Setenv("XDG_RUNTIME_DIR", "")
	if got := certTempBase(); got != os.TempDir() {
		t.Errorf("certTempBase() without XDG_RUNTIME_DIR = %q, want %q", got, os.TempDir())
	}
}

func TestSweepStaleCertTempDirs(t *testing.T) {
	base := t.TempDir()
	stale := filepath.Join(base, "supersonic-certs-old")
	fresh := filepath.Join(base, "supersonic-certs-new")
	unrelated := filepath.Join(base, "other-dir")
	for _, dir := range []string{stale, fresh, unrelated} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	sweepStaleCertTempDirs(base)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale cert dir was not removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("fresh cert dir was removed")
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Error("unrelated dir was removed")
	}
}
