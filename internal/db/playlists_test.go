package db

import (
	"errors"
	"testing"
)

func TestPlaylists(t *testing.T) {
	newTestDB(t)
	a := addTestTrack(t, "A", "a")
	b := addTestTrack(t, "B", "b")
	p, err := CreatePlaylist(All, "Mix")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a, b, a} {
		if err := AppendToPlaylist(All, p.ID.String(), id); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := GetPlaylist(All, p.ID.String())
	if titles(got.Tracks) != "A,B,A" {
		t.Fatalf("tracks = %s", titles(got.Tracks))
	}

	// reorder
	if err := SetPlaylistTracks(All, p.ID.String(), []string{b, a}); err != nil {
		t.Fatal(err)
	}
	got, _ = GetPlaylist(All, p.ID.String())
	if titles(got.Tracks) != "B,A" {
		t.Fatalf("after reorder = %s", titles(got.Tracks))
	}
	if err := AppendToPlaylist(All, p.ID.String(), "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrBadReference) {
		t.Fatalf("unknown track err = %v", err)
	}

	// a deleted track leaves the playlist
	DB.Exec(`DELETE FROM tracks WHERE uuid = ?;`, b)
	got, _ = GetPlaylist(All, p.ID.String())
	if titles(got.Tracks) != "A" {
		t.Fatalf("after delete = %s", titles(got.Tracks))
	}
	list, _ := ListPlaylists(All)
	if len(list) != 1 || list[0].Count != 1 {
		t.Fatalf("list = %+v", list)
	}
	if err := DeletePlaylist(All, p.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := GetPlaylist(All, p.ID.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted playlist err = %v", err)
	}
}
