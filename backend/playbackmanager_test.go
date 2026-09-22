package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverConfiguredRemotePlayers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/description.xml" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0"><device><deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType><friendlyName>Test Renderer</friendlyName><modelName>Test Model</modelName><serviceList><service><serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>upnp/control/transport</controlURL><eventSubURL>upnp/event/transport</eventSubURL></service><service><serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType><controlURL>/upnp/control/render</controlURL></service></serviceList></device></root>`)
	}))
	defer server.Close()

	players := discoverConfiguredRemotePlayers(context.Background(), " , "+server.URL+"/description.xml,invalid://")
	if len(players) != 1 {
		t.Fatalf("got %d players, want 1", len(players))
	}
	if players[0].FriendlyName != "Test Renderer" {
		t.Fatalf("got renderer %q", players[0].FriendlyName)
	}
}

func TestDiscoverConfiguredRemotePlayersEmpty(t *testing.T) {
	if players := discoverConfiguredRemotePlayers(context.Background(), " "); players != nil {
		t.Fatalf("got %#v, want nil", players)
	}
}

func TestConfiguredDLNAProxyIPDefaultAndMissing(t *testing.T) {
	if got := configuredDLNAProxyIP(&AppConfig{UseDefaultSSDPInterface: true}); got != "" {
		t.Fatalf("default mode returned %q", got)
	}
	if got := configuredDLNAProxyIP(&AppConfig{SSDPInterfaceName: "does-not-exist"}); got != "" {
		t.Fatalf("missing interface returned %q", got)
	}
}

func TestReadConfigFileDefaultsLegacySSDPSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[Application]\nMaxImageCacheSizeMB = 50\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := ReadConfigFile(path, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !config.Application.UseDefaultSSDPInterface {
		t.Fatal("legacy config did not preserve default SSDP behavior")
	}
}
