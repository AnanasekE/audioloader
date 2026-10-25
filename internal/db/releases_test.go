package db

import (
	"errors"
	"path/filepath"
	"strings"
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
	if err := SetReleaseTracks(All, r.ID.String(), []ReleaseTrackInput{{TrackID: tr, Disc: 1}}); err != nil {
		t.Fatal(err)
	}
	b, err := AddArtist("other")
	if err != nil {
		t.Fatal(err)
	}

	if err := UpdateRelease(r.ID.String(), "new", audio.ReleaseEP, "2020", []string{b.ID.String()}); err != nil {
		t.Fatal(err)
	}
	got, err := GetRelease(All, r.ID.String())
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
	if err := SetReleaseTracks(All, r.ID.String(), []ReleaseTrackInput{{TrackID: tr, Disc: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteRelease(r.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := GetRelease(All, r.ID.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRelease err = %v", err)
	}
	if _, err := GetTrack(All, tr); err != nil {
		t.Fatalf("track gone: %v", err)
	}
}

func TestReleaseCoverOnTracks(t *testing.T) {
	newTestDB(t)
	album, _ := newTestRelease(t, "album", audio.ReleaseAlbum)
	tr := addTestTrack(t, "song", "x") // also creates its single
	if err := SetReleaseTracks(All, album.ID.String(), []ReleaseTrackInput{{TrackID: tr, Disc: 1}}); err != nil {
		t.Fatal(err)
	}
	track, _ := GetTrack(All, tr)
	if track.ImgURL != DefaultCover {
		t.Fatalf("no covers yet, got %q", track.ImgURL)
	}

	single := track.Releases[0]
	if single.Type != audio.ReleaseSingle {
		single = track.Releases[1]
	}
	if err := SetReleaseCover(single.ID, strings.NewReader("png"), ".png"); err != nil {
		t.Fatal(err)
	}
	track, _ = GetTrack(All, tr)
	if !strings.HasPrefix(track.ImgURL, "/covers/"+single.ID.String()) {
		t.Fatalf("want the single's cover, got %q", track.ImgURL)
	}

	if err := SetReleaseCover(album.ID, strings.NewReader("jpg"), ".jpg"); err != nil {
		t.Fatal(err)
	}
	track, _ = GetTrack(All, tr)
	if !strings.HasPrefix(track.ImgURL, "/covers/"+album.ID.String()) {
		t.Fatalf("album cover should win, got %q", track.ImgURL)
	}
	// replacing with another extension leaves one file
	if err := SetReleaseCover(album.ID, strings.NewReader("png"), ".png"); err != nil {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(filepath.Join(CoversDir, album.ID.String()+".*")); len(m) != 1 {
		t.Fatalf("cover files = %v", m)
	}
}
