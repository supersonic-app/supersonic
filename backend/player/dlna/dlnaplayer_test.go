package dlna

import "testing"

// The current and the next track each put a stream and a cover art URL
// into the proxy. All four have to stay reachable: if the current track's
// stream is evicted, a seek makes the renderer fetch a 404 and skip to
// whatever it has queued next.
func TestProxyKeepsCurrentAndNextTrackReachable(t *testing.T) {
	d := &DLNAPlayer{}

	added := []struct {
		name string
		url  string
		key  string
	}{
		{"current stream", "http://server/stream?id=cur", ""},
		{"current art", "/cache/art/cur.jpg", ""},
		{"next stream", "http://server/stream?id=next", ""},
		{"next art", "/cache/art/next.jpg", ""},
	}
	for i := range added {
		added[i].key = d.addURLToProxy(added[i].url)
	}

	for _, a := range added {
		got, ok := d.lookupProxyURL(a.key)
		if !ok {
			t.Errorf("%s was evicted from the proxy", a.name)
		} else if got != a.url {
			t.Errorf("%s resolved to %q, want %q", a.name, got, a.url)
		}
	}
}

func TestProxyEvictsLeastRecentlyUsed(t *testing.T) {
	d := &DLNAPlayer{}

	oldest := d.addURLToProxy("http://server/0")
	for i := 1; i < len(d.proxyURLs); i++ {
		d.addURLToProxy("http://server/" + string(rune('a'+i)))
	}
	// still the least recently used, so the next insert drops it
	if _, ok := d.lookupProxyURL(oldest); !ok {
		t.Fatal("proxy dropped an entry while it still had room")
	}

	// the lookup above promoted it, so refill past capacity to evict it
	for i := range d.proxyURLs {
		d.addURLToProxy("http://server/new" + string(rune('a'+i)))
	}
	if _, ok := d.lookupProxyURL(oldest); ok {
		t.Error("expected the least recently used entry to be evicted")
	}
}
