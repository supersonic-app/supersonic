package backend

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// DiscoveredDLNARenderer is a renderer found through a local service
// discovery mechanism.
type DiscoveredDLNARenderer struct {
	Name string
	URL  string
}

// DiscoverDLNARenderers combines the normal SSDP search with an Avahi fallback
// on Linux. The fallback is needed for devices such as the WiiM Pro that
// advertise LinkPlay via mDNS but do not answer SSDP M-SEARCH requests.
func DiscoverDLNARenderers(ctx context.Context, appCfg *AppConfig) ([]DiscoveredDLNARenderer, error) {
	devices := searchMediaRenderers(ctx, 1, appCfg)
	seen := make(map[string]bool, len(devices))
	renderers := make([]DiscoveredDLNARenderer, 0, len(devices))
	for _, renderer := range devices {
		seen[renderer.URL] = true
		renderers = append(renderers, DiscoveredDLNARenderer{
			Name: renderer.FriendlyName,
			URL:  renderer.URL,
		})
	}

	if runtime.GOOS != "linux" {
		return renderers, nil
	}

	if _, err := exec.LookPath("avahi-browse"); err != nil {
		if len(renderers) > 0 {
			return renderers, nil
		}
		return nil, fmt.Errorf("avahi-browse is unavailable: %w", err)
	}
	cmd := exec.CommandContext(ctx, "avahi-browse", "-prt", "_linkplay._tcp")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("avahi-browse failed: %w", err)
	}
	for _, renderer := range parseAvahiLinkPlayOutput(string(output)) {
		if !seen[renderer.URL] {
			seen[renderer.URL] = true
			renderers = append(renderers, renderer)
		}
	}
	return renderers, nil
}

func parseAvahiLinkPlayOutput(output string) []DiscoveredDLNARenderer {
	seen := make(map[string]bool)
	var renderers []DiscoveredDLNARenderer
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ";")
		if len(fields) < 8 || fields[0] != "=" || fields[2] != "IPv4" {
			continue
		}
		name := fields[3]
		address := fields[6]
		if name == "" || address == "" {
			continue
		}
		url := "http://" + address + ":49152/description.xml"
		if seen[url] {
			continue
		}
		seen[url] = true
		renderers = append(renderers, DiscoveredDLNARenderer{Name: name, URL: url})
	}
	return renderers
}
