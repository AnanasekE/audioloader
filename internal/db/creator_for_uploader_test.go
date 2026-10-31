package db

import (
	"bytes"
	"testing"

	"audioloader/internal/audio"
)

func addSecretTrackFrom(t *testing.T, title, body, sourceURL string, artistIDs ...string) {
	t.Helper()
	res, err := AddTrack(Secret, artistIDs, &audio.Track{Title: title}, bytes.NewReader([]byte(body)), ".mp3", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSourceURL(res.TrackID, sourceURL); err != nil {
		t.Fatal(err)
	}
}

func TestCreatorForUploader(t *testing.T) {
	newTestDB(t)
	const parent = "https://pages.example.test/items/someone"
	linked, _ := AddArtist(Secret, "Linked")
	other, _ := AddArtist(Secret, "Other")
	named, _ := AddArtist(Secret, "SomeOne")
	pub, _ := AddArtist(Normal, "Publicname")

	got := func(parentPage, name string) string {
		t.Helper()
		a, err := CreatorForUploader(Secret, parentPage, name)
		if err != nil {
			t.Fatal(err)
		}
		if a == nil {
			return ""
		}
		return a.Name
	}

	// nothing matches yet, except the name
	if g := got(parent, "nobody"); g != "" {
		t.Errorf("no match = %q", g)
	}
	if g := got(parent, "someone"); g != "SomeOne" {
		t.Errorf("name match, ignoring case = %q", g)
	}

	// a linked creator beats the name; the one with the most tracks wins
	addSecretTrackFrom(t, "a", "1", parent+"/a", other.ID.String())
	if g := got(parent, "someone"); g != "Other" {
		t.Errorf("link before name = %q", g)
	}
	addSecretTrackFrom(t, "b", "2", parent+"/b", linked.ID.String())
	addSecretTrackFrom(t, "c", "3", parent+"/c", linked.ID.String())
	if g := got(parent, "someone"); g != "Linked" {
		t.Errorf("most tracks = %q", g)
	}

	// a track deeper below, or under a longer name, is not the same parent
	addSecretTrackFrom(t, "d", "4", parent+"/deeper/d", named.ID.String())
	addSecretTrackFrom(t, "e", "5", parent+"x/e", named.ID.String())
	addSecretTrackFrom(t, "f", "6", "https://pages.example.test/items/else/f", named.ID.String())
	if g := got(parent, "nobody"); g != "Linked" {
		t.Errorf("same parent only = %q", g)
	}
	if g := got("https://pages.example.test/items/else", "nobody"); g != "SomeOne" {
		t.Errorf("other parent = %q", g)
	}

	// a normal mode artist is never returned in secret mode
	if g := got("", "publicname"); g != "" {
		t.Errorf("normal artist = %q", g)
	}
	_ = pub
}
