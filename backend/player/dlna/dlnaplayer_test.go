package dlna

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

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

func TestParseClockTime(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"0:02:35", 2*time.Minute + 35*time.Second, true}, // Sonos does not pad the hours
		{"00:02:35", 2*time.Minute + 35*time.Second, true},
		{"1:00:07.500", time.Hour + 7*time.Second, true},
		{"0:00:00", 0, true},
		{"NOT_IMPLEMENTED", 0, false},
		{"02:35", 0, false},
		{"", 0, false},
		{"0:-1:00", 0, false},
	}
	for _, c := range cases {
		got, err := parseClockTime(c.in)
		if (err == nil) != c.ok {
			t.Errorf("parseClockTime(%q) error = %v, want ok=%v", c.in, err, c.ok)
		} else if got != c.want {
			t.Errorf("parseClockTime(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// A GetPositionInfo response as a Sonos player sends it.
const sonosPositionInfo = `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:GetPositionInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><Track>1</Track><TrackDuration>0:03:24</TrackDuration><TrackMetaData></TrackMetaData><TrackURI>http://192.168.1.237:39889/0JujwzEvS9dUT/U27Cr4xw==</TrackURI><RelTime>0:02:35</RelTime><AbsTime>NOT_IMPLEMENTED</AbsTime><RelCount>2147483647</RelCount><AbsCount>2147483647</AbsCount></u:GetPositionInfoResponse></s:Body></s:Envelope>`

func TestPositionInfo(t *testing.T) {
	var action string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action = r.Header.Get("SOAPAction")
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		w.Write([]byte(sonosPositionInfo))
	}))
	defer srv.Close()

	d := &DLNAPlayer{avtRequests: &httpClientHandler{client: srv.Client(), controlURL: srv.URL + "/MediaRenderer/AVTransport/Control"}}
	pos, err := d.positionInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := 2*time.Minute + 35*time.Second; pos != want {
		t.Errorf("position = %v, want %v", pos, want)
	}
	if want := `"urn:schemas-upnp-org:service:AVTransport:1#GetPositionInfo"`; action != want {
		t.Errorf("SOAPAction = %s, want %s", action, want)
	}
}

// onTrackChange wires the player up so that a firing of the track change
// timer is observable without a renderer: with a next track queued, the
// change is reported through the OnTrackChange callback.
func onTrackChange(d *DLNAPlayer) <-chan struct{} {
	fired := make(chan struct{}, 1)
	d.nextTrackMeta = mediaprovider.MediaItemMetadata{ID: "next", Duration: time.Hour}
	d.OnTrackChange(func() { fired <- struct{}{} })
	return fired
}

func TestTrackChangeTimerFires(t *testing.T) {
	d := &DLNAPlayer{}
	fired := onTrackChange(d)
	d.setTrackChangeTimer(10 * time.Millisecond)
	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("track change timer never fired")
	}
}

func TestTrackChangeTimerCancelAndReschedule(t *testing.T) {
	d := &DLNAPlayer{}
	fired := onTrackChange(d)

	d.setTrackChangeTimer(10 * time.Millisecond)
	d.setTrackChangeTimer(0)
	select {
	case <-fired:
		t.Fatal("cancelled track change timer fired")
	case <-time.After(50 * time.Millisecond):
	}

	d.setTrackChangeTimer(10 * time.Millisecond)
	d.setTrackChangeTimer(time.Hour)
	select {
	case <-fired:
		t.Fatal("rescheduled track change timer fired on its old schedule")
	case <-time.After(50 * time.Millisecond):
	}
	d.setTrackChangeTimer(0)
}

// A pause that lands just as the timer fires must win. The timer can no
// longer be stopped at that point, so its firing arrives after the cancel
// and has to be recognized as stale.
func TestTrackChangeTimerFiringLosesToCancel(t *testing.T) {
	d := &DLNAPlayer{}
	fired := onTrackChange(d)

	d.setTrackChangeTimer(time.Hour)
	gen := d.timerGen
	d.setTrackChangeTimer(0)
	d.trackChangeTimerFired(gen)

	select {
	case <-fired:
		t.Fatal("a cancelled firing still changed the track")
	case <-time.After(50 * time.Millisecond):
	}
}

// A renderer that accepts a control request and never answers it must not
// hang the caller: the request has to be given up, retries included.
func TestControlRequestGivesUpOnSilentRenderer(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release) // let the handler finish before the server is closed

	cli := newControlClient(100 * time.Millisecond)
	start := time.Now()
	resp, err := cli.Post(srv.URL, "text/xml", strings.NewReader("<s:Envelope/>"))
	if err == nil {
		resp.Body.Close()
		t.Fatal("a request the renderer never answered succeeded")
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Errorf("gave up after %v, want about 100ms", waited)
	}
}
