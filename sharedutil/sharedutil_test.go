package sharedutil

import (
	"slices"
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
