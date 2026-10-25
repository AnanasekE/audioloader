package db

import (
	"errors"
	"testing"

	"audioloader/internal/audio"
)

func newTestRelease(t *testing.T, title string, typ audio.ReleaseType) (*audio.Release, string) {
	t.Helper()
	a, err := AddArtist("artist " + title)
	if err != nil {
		t.Fatal(err)
	}
	r, err := CreateRelease(title, typ, "", []string{a.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	return r, a.ID.String()
}

func TestUpdateReleaseKeepsTracklist(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "old", audio.ReleaseAlbum)
	tr := addTestTrack(t, "song", "x")
	if err := SetReleaseTracks(r.ID.String(), []ReleaseTrackInput{{TrackID: tr, Disc: 1}}); err != nil {
		t.Fatal(err)
	}
	b, err := AddArtist("other")
	if err != nil {
		t.Fatal(err)
	}

	if err := UpdateRelease(r.ID.String(), "new", audio.ReleaseEP, "2020", []string{b.ID.String()}); err != nil {
		t.Fatal(err)
	}
	got, err := GetRelease(r.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "new" || got.Type != audio.ReleaseEP || got.Date != "2020" {
		t.Fatalf("got %+v", got)
	}
	if len(got.Artists) != 1 || got.Artists[0].Name != "other" {
		t.Fatalf("artists = %v", got.Artists)
	}
	if len(got.Tracks) != 1 {
		t.Fatalf("tracks = %d, want 1", len(got.Tracks))
	}

	if err := UpdateRelease("00000000-0000-0000-0000-000000000000", "x", audio.ReleaseEP, "", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing release err = %v", err)
	}
}

func TestDeleteReleaseKeepsTracks(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "rel", audio.ReleaseAlbum)
	tr := addTestTrack(t, "song", "x")
	if err := SetReleaseTracks(r.ID.String(), []ReleaseTrackInput{{TrackID: tr, Disc: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteRelease(r.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := GetRelease(r.ID.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRelease err = %v", err)
	}
	if _, err := GetTrack(tr); err != nil {
		t.Fatalf("track gone: %v", err)
	}
}
