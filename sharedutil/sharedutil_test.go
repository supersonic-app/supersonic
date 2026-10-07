package sharedutil

import (
	"context"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

func Test_ReorderItems(t *testing.T) {
	tracks := []*mediaprovider.Track{
		{ID: "a"}, // 0
		{ID: "b"}, // 1
		{ID: "c"}, // 2
		{ID: "d"}, // 3
		{ID: "e"}, // 4
		{ID: "f"}, // 5
	}

	// test MoveToTop:
	idxToMove := []int{0, 2, 3, 5}
	want := []*mediaprovider.Track{
		{ID: "a"},
		{ID: "c"},
		{ID: "d"},
		{ID: "f"},
		{ID: "b"},
		{ID: "e"},
	}
	newTracks := ReorderItems(tracks, idxToMove, 0)
	if !tracklistsEqual(t, newTracks, want) {
		t.Error("ReorderTracks: MoveToTop order incorrect")
	}

	// test MoveToBottom:
	idxToMove = []int{0, 2, 5}
	want = []*mediaprovider.Track{
		{ID: "b"},
		{ID: "d"},
		{ID: "e"},
		{ID: "a"},
		{ID: "c"},
		{ID: "f"},
	}
	newTracks = ReorderItems(tracks, idxToMove, len(tracks))
	if !tracklistsEqual(t, newTracks, want) {
		t.Error("ReorderTracks: MoveToBottom order incorrect")
	}
}

func tracklistsEqual(t *testing.T, a, b []*mediaprovider.Track) bool {
	t.Helper()
	return slices.EqualFunc(a, b, func(a, b *mediaprovider.Track) bool {
		return a.ID == b.ID
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDownloadFileWithContextUsesProvidedClient(t *testing.T) {
	var requested bool
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requested = req.URL.String() == "http://music.example.test/track"
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("audio")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	destination := t.TempDir() + "/track"
	completed, err := DownloadFileWithContext(context.Background(), client, "http://music.example.test/track", destination)
	if err != nil {
		t.Fatalf("downloading file: %v", err)
	}
	if !completed {
		t.Fatal("download did not complete")
	}
	if !requested {
		t.Fatal("provided HTTP client did not receive the request")
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(content) != "audio" {
		t.Fatalf("downloaded content = %q, want %q", content, "audio")
	}
}
