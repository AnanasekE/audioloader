package db

import (
	"testing"

	"audioloader/internal/audio"
)

func TestSearch(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist("Björk")
	addTestTrack(t, "Jóga", "joga", a.ID.String())
	addTestTrack(t, "Hunter", "hunter", a.ID.String())
	if _, err := CreateRelease("Homogenic", audio.ReleaseAlbum, "1997", []string{a.ID.String()}); err != nil {
		t.Fatal(err)
	}

	res, err := Search(All, "bjork", 10)
	if err != nil {
		t.Fatal(err)
	}
	// both tracks by artist name, the release, and the artist
	if len(res.Tracks) != 2 || len(res.Releases) != 3 || len(res.Artists) != 1 {
		t.Fatalf("bjork: %d tracks, %d releases, %d artists", len(res.Tracks), len(res.Releases), len(res.Artists))
	}

	// prefix while typing, and accents ignored
	res, _ = Search(All, "joga", 10)
	if len(res.Tracks) != 1 || res.Tracks[0].Title != "Jóga" {
		t.Fatalf("joga: %+v", res.Tracks)
	}
	res, _ = Search(All, "homog", 10)
	if len(res.Releases) != 1 || res.Releases[0].Title != "Homogenic" {
		t.Fatalf("homog: %+v", res.Releases)
	}

	// changes show up without an explicit reindex
	a2, _ := AddArtist("Massive Attack")
	if res, _ := Search(All, "massive", 10); len(res.Artists) != 1 || res.Artists[0].ID != a2.ID {
		t.Fatalf("new artist not found: %+v", res.Artists)
	}

	// FTS syntax is just text
	if _, err := Search(All, `"unbalanced AND OR NEAR(`, 10); err != nil {
		t.Fatalf("syntax leaked into the query: %v", err)
	}
	if res, _ := Search(All, "   ", 10); !res.Empty() {
		t.Fatal("blank query matched")
	}
}

func TestSearchSkipsPlaceholders(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "rel", audio.ReleaseAlbum)
	addTestPlaceholder(t, r, "Ghost Song", 1)
	if res, _ := Search(All, "ghost", 10); len(res.Tracks) != 0 {
		t.Fatalf("placeholder found: %+v", res.Tracks)
	}
}
