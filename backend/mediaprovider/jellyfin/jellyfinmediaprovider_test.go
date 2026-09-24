package jellyfin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jellyfinapi "github.com/dweymouth/go-jellyfin"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDownloadTrackUsesProviderHTTPClient(t *testing.T) {
	var requestedPath string
	client, err := jellyfinapi.NewClient("http://music.example.test", "test", "1", jellyfinapi.WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			requestedPath = req.URL.Path
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("track")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("creating Jellyfin client: %v", err)
	}

	provider := newJellyfinMediaProvider(client).(*JellyfinMediaProvider)
	reader, err := provider.DownloadTrack("track-id")
	if err != nil {
		t.Fatalf("downloading track: %v", err)
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading track: %v", err)
	}
	if requestedPath != "/audio/track-id/stream" {
		t.Fatalf("request path = %q, want stream endpoint", requestedPath)
	}
	if string(body) != "track" {
		t.Fatalf("track body = %q, want %q", body, "track")
	}
}

func TestDownloadTrackDoesNotInheritAPITimeout(t *testing.T) {
	const apiTimeout = 200 * time.Millisecond
	const bodyDelay = 2 * apiTimeout
	const track = "slow track"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(bodyDelay)
		_, _ = io.WriteString(w, track)
	}))
	defer server.Close()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	apiHTTPClient := &http.Client{
		Transport: transport,
		Timeout:   apiTimeout,
	}
	client, err := jellyfinapi.NewClient(server.URL, "test", "1", jellyfinapi.WithHTTPClient(apiHTTPClient))
	if err != nil {
		t.Fatalf("creating Jellyfin client: %v", err)
	}
	provider := newJellyfinMediaProvider(client).(*JellyfinMediaProvider)
	if provider.downloadClient == apiHTTPClient {
		t.Fatal("download client must not reuse the API client")
	}
	if got, want := provider.downloadClient.Transport, apiHTTPClient.Transport; got != want {
		t.Fatalf("download transport = %p, want API transport %p", got, want)
	}
	if got := provider.downloadClient.Timeout; got != 0 {
		t.Fatalf("download client timeout = %v, want 0", got)
	}
	if got := apiHTTPClient.Timeout; got != apiTimeout {
		t.Fatalf("API client timeout = %v, want %v", got, apiTimeout)
	}

	reader, err := provider.DownloadTrack("track-id")
	if err != nil {
		t.Fatalf("downloading track: %v", err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading slow track: %v", err)
	}
	if string(body) != track {
		t.Fatalf("track body = %q, want %q", body, track)
	}
}
