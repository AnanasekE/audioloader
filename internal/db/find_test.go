package db

import "testing"

func TestOwnedRecordings(t *testing.T) {
	newTestDB(t)
	trackID, relID, _, err := AddSong(songOnRelease("Muse", "r1", 1))
	if err != nil {
		t.Fatal(err)
	}
	got, err := OwnedRecordings([]string{"r1", "nope"})
	if err != nil || len(got) != 1 {
		t.Fatalf("owned = %v, %v", got, err)
	}
	if o := got["r1"]; o.TrackID != trackID || o.ReleaseID != relID || o.HasFile {
		t.Fatalf("placeholder = %+v", o)
	}
	if _, err := DB.Exec(`UPDATE tracks SET has_file = 1 WHERE uuid = ?;`, trackID); err != nil {
		t.Fatal(err)
	}
	if o, _ := OwnedRecordings([]string{"r1"}); !o["r1"].HasFile {
		t.Fatalf("downloaded = %+v", o)
	}
	if o, err := OwnedRecordings(nil); err != nil || len(o) != 0 {
		t.Fatalf("none = %v, %v", o, err)
	}
	// a secret track is not part of the normal library
	if _, err := DB.Exec(`UPDATE tracks SET secret = 1 WHERE uuid = ?;`, trackID); err != nil {
		t.Fatal(err)
	}
	if o, _ := OwnedRecordings([]string{"r1"}); len(o) != 0 {
		t.Fatalf("secret = %v", o)
	}
}

func TestOwnedReleaseCounts(t *testing.T) {
	newTestDB(t)
	first, _, _, err := AddSong(songOnRelease("One", "r1", 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := AddSong(songOnRelease("Two", "r2", 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`UPDATE tracks SET has_file = 1 WHERE uuid = ?;`, first); err != nil {
		t.Fatal(err)
	}
	got, err := OwnedReleaseCounts([]string{"rg1", "rg2"})
	if err != nil || len(got) != 1 || got["rg1"] != (ReleaseCount{Have: 1, Total: 2}) {
		t.Fatalf("counts = %v, %v", got, err)
	}
}

func TestFollowedArtists(t *testing.T) {
	newTestDB(t)
	a, err := AddArtist(All, "Ella Red")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetFollow(a.ID.String(), "mbid-1", DefaultFilters()); err != nil {
		t.Fatal(err)
	}
	got, err := FollowedArtists([]string{"mbid-1", "mbid-2"})
	if err != nil || len(got) != 1 || !got["mbid-1"] {
		t.Fatalf("followed = %v, %v", got, err)
	}
}
