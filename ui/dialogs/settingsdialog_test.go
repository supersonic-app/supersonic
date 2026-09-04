package dialogs

import "testing"

func TestInterfaceDisplayName(t *testing.T) {
	recommended := map[string]bool{"wlan0": true}
	if got := interfaceDisplayName("wlan0", recommended); got != "wlan0 (recommended)" {
		t.Fatalf("got %q", got)
	}
	if got := interfaceDisplayName("lo", recommended); got != "lo" {
		t.Fatalf("got %q", got)
	}
}
