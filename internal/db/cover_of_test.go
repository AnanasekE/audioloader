package db

import (
	"database/sql"
	"errors"
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

func TestSetCover(t *testing.T) {
	newTestDB(t)
	orig, _ := AddArtist("Original Artist")
	cov, _ := AddArtist("Cover Artist")
	original := addTestTrack(t, "Song", "orig", orig.ID.String())
	cover := addTestTrack(t, "Cover", "cov", cov.ID.String())
	edit := func(c CoverEdit) error {
		return UpdateTrack(All, cover, TrackEdit{Title: "Cover", Kind: audio.KindMusic, ArtistIDs: []string{cov.ID.String()}, Cover: &c})
	}
	check := func(want string) {
		t.Helper()
		tr, err := GetTrack(All, cover)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if tr.CoverOf != nil {
			got = tr.CoverOf.String()
		}
		if got != want {
			t.Fatalf("cover of = %q, want %q", got, want)
		}
		if len(tr.Artists) != 1 || tr.Artists[0].ID != cov.ID {
			t.Fatalf("cover artists changed: %+v", tr.Artists)
		}
	}

	// text
	if err := edit(CoverEdit{Title: "Text Song", Artists: "A & B"}); err != nil {
		t.Fatal(err)
	}
	check("Text Song by A & B")
	// MusicBrainz: text plus the recording ID
	if err := edit(CoverEdit{Title: "Teardrop", Artists: "Massive Attack", MBID: "f3bba4cd-8018-468b-902e-bc8f029593e5"}); err != nil {
		t.Fatal(err)
	}
	if tr, _ := GetTrack(All, cover); tr.CoverOf.MBID == "" || tr.CoverOf.TrackID != "" {
		t.Fatalf("mbid cover = %+v", tr.CoverOf)
	}
	// library: replaces the text, never both
	if err := edit(CoverEdit{TrackID: original}); err != nil {
		t.Fatal(err)
	}
	check("Song by Original Artist")
	var title sql.NullString
	DB.QueryRow(`SELECT original_title FROM tracks WHERE uuid = ?;`, cover).Scan(&title)
	if title.Valid {
		t.Fatal("text reference left behind a library link")
	}

	// bad input changes nothing
	for name, c := range map[string]CoverEdit{
		"itself":     {TrackID: cover},
		"both":       {TrackID: original, Title: "x"},
		"no title":   {Artists: "A"},
		"no such id": {TrackID: "00000000-0000-0000-0000-000000000000"},
	} {
		if err := edit(c); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	check("Song by Original Artist")
	// a nil Cover leaves it alone
	if err := UpdateTrack(All, cover, TrackEdit{Title: "Cover", Kind: audio.KindMusic, ArtistIDs: []string{cov.ID.String()}}); err != nil {
		t.Fatal(err)
	}
	check("Song by Original Artist")

	// a locked viewer can't link a secret original
	SetSecret(original, true)
	err := UpdateTrack(Public, cover, TrackEdit{Title: "Cover", Kind: audio.KindMusic, ArtistIDs: []string{cov.ID.String()}, Cover: &CoverEdit{TrackID: original}})
	if !errors.Is(err, ErrBadReference) {
		t.Fatalf("secret original = %v", err)
	}
	SetSecret(original, false)

	// deleting the original unlinks the cover, which keeps only its own credits
	if err := DeleteTrack(All, original); err != nil {
		t.Fatal(err)
	}
	check("")

	// remove clears the text
	edit(CoverEdit{Title: "Text Song"})
	if err := edit(CoverEdit{}); err != nil {
		t.Fatal(err)
	}
	check("")
}
