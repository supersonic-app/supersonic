package backend

import "testing"

func TestParseAvahiLinkPlayOutput(t *testing.T) {
	output := `=;wlan0;IPv4;Kitchen Speaker;_linkplay._tcp;local;192.0.2.42;59152;"upnp=1.0.0"
=;wlan0;IPv6;Kitchen Speaker;_linkplay._tcp;local;Kitchen-Speaker.local;59152;"upnp=1.0.0"
=;wlan0;IPv4;Kitchen Speaker;_linkplay._tcp;local;192.0.2.42;59152;"upnp=1.0.0"`

	renderers := parseAvahiLinkPlayOutput(output)
	if len(renderers) != 1 {
		t.Fatalf("got %d renderers, want 1", len(renderers))
	}
	if renderers[0].Name != "Kitchen Speaker" || renderers[0].URL != "http://192.0.2.42:49152/description.xml" {
		t.Fatalf("got %#v", renderers[0])
	}
}
