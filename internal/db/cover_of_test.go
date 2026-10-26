package db

import (
	"testing"

	"audioloader/internal/audio"
)

func TestCoverOfLoads(t *testing.T) {
	newTestDB(t)
	orig, _ := AddArtist("Original Artist")
	cov, _ := AddArtist("Cover Artist")
	original := addTestTrack(t, "Song", "orig", orig.ID.String())
	linked := addTestTrack(t, "Cover A", "a", cov.ID.String())
	text := addTestTrack(t, "Cover B", "b", cov.ID.String())
	plain := addTestTrack(t, "Plain", "c", cov.ID.String())
	if _, err := DB.Exec(`UPDATE tracks SET original_track_id = ? WHERE uuid = ?;`, original, linked); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`UPDATE tracks SET original_title = 'Text Song', original_artists = 'A & B', original_mbid = 'mbid' WHERE uuid = ?;`, text); err != nil {
		t.Fatal(err)
	}

	got := map[string]*audio.Track{}
	tracks, err := ListTracks(Public)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range tracks {
		got[tr.ID.String()] = tr
	}
	if c := got[linked].CoverOf; c == nil || c.Title != "Song" || c.Artists != "Original Artist" || c.TrackID != original || c.ReleaseID == "" {
		t.Errorf("linked cover = %+v", c)
	}
	if c := got[text].CoverOf; c == nil || c.Title != "Text Song" || c.Artists != "A & B" || c.MBID != "mbid" || c.TrackID != "" {
		t.Errorf("text cover = %+v", c)
	}
	if got[plain].CoverOf != nil || got[original].CoverOf != nil {
		t.Error("tracks that cover nothing have a CoverOf")
	}
	if got[linked].Artists[0].Name != "Cover Artist" || len(got[linked].Artists) != 1 {
		t.Errorf("cover artists changed: %+v", got[linked].Artists)
	}

	// a secret original is invisible to a locked viewer, even its title
	if err := SetSecret(original, true); err != nil {
		t.Fatal(err)
	}
	tr, err := GetTrack(Public, linked)
	if err != nil {
		t.Fatal(err)
	}
	if tr.CoverOf != nil {
		t.Errorf("hidden original shown: %+v", tr.CoverOf)
	}
	if tr, _ := GetTrack(All, linked); tr.CoverOf == nil || tr.CoverOf.Title != "Song" {
		t.Errorf("unlocked viewer: %+v", tr.CoverOf)
	}
}
