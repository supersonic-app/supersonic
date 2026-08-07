package jellyfin

import (
	"io"
	"net/http"
	"strings"
	"testing"

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

	provider := &JellyfinMediaProvider{client: client}
	reader, err := provider.DownloadTrack("track-id")
	if err != nil {
		t.Fatalf("downloading track: %v", err)
	}
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
