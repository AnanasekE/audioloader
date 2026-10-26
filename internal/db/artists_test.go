package db

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"audioloader/internal/audio"
)

func TestArtistPageLists(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist("Main")
	b, _ := AddArtist("Other")
	own, _ := CreateRelease("Own Album", audio.ReleaseAlbum, "2020", []string{a.ID.String()})
	theirs, _ := CreateRelease("Their Album", audio.ReleaseAlbum, "2021", []string{b.ID.String()})
	onOwn := addTestTrack(t, "On Own", "1", a.ID.String())
	feature := addTestTrack(t, "Feature", "2", a.ID.String(), b.ID.String())
	loose := addTestTrack(t, "Loose", "3", a.ID.String())
	addTestTrack(t, "Not Main", "4", b.ID.String())
	// uploads without a release made singles; drop them to get loose tracks
	DB.Exec(`DELETE FROM releases WHERE type = 'single';`)
	PlaceTrack(onOwn, ReleaseLink{ReleaseID: own.ID.String(), Disc: 1, Position: 1})
	PlaceTrack(feature, ReleaseLink{ReleaseID: theirs.ID.String(), Disc: 1, Position: 1})

	rels, err := ListArtistReleases(All, a.ID.String())
	if err != nil || len(rels) != 1 || rels[0].Title != "Own Album" {
		t.Fatalf("releases = %+v, %v", rels, err)
	}
	tracks, err := ListArtistTracks(All, a.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tr := range tracks {
		got[tr.ID.String()] = true
	}
	if len(tracks) != 2 || !got[feature] || !got[loose] {
		t.Fatalf("tracks = %+v", tracks)
	}
}

func TestArtistImage(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist("Pic")
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	jpg := append([]byte("\xff\xd8\xff\xe0"), make([]byte, 32)...)

	if err := SaveArtistImage(a.ID, strings.NewReader("<html>not an image</html>")); !errors.Is(err, ErrBadImage) {
		t.Fatalf("html = %v", err)
	}
	if err := SaveArtistImage(a.ID, bytes.NewReader(png)); err != nil {
		t.Fatal(err)
	}
	first, err := ArtistImagePath(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := GetArtist(All, a.ID.String()); !strings.HasPrefix(got.ImgURL, "/artist-images/"+a.ID.String()) {
		t.Fatalf("ImgURL = %q", got.ImgURL)
	}

	// replacing removes the old file
	if err := SaveArtistImage(a.ID, bytes.NewReader(jpg)); err != nil {
		t.Fatal(err)
	}
	second, _ := ArtistImagePath(a.ID)
	if second == first || filepath.Ext(second) != ".jpg" {
		t.Fatalf("second = %s", second)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("old picture still there: %v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(ArtistImagesDir, "*")); len(m) != 1 {
		t.Fatalf("files = %v", m)
	}

	// removing goes back to the default
	if err := RemoveArtistImage(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ArtistImagePath(a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("path after remove = %v", err)
	}
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatalf("picture still there: %v", err)
	}
	if got, _ := GetArtist(All, a.ID.String()); got.ImgURL != "/assets/img/default_pfp.png" {
		t.Fatalf("ImgURL = %q", got.ImgURL)
	}
	// the track and release lists show it too
	SaveArtistImage(a.ID, bytes.NewReader(png))
	id := addTestTrack(t, "Song", "s", a.ID.String())
	if tr, _ := GetTrack(All, id); !strings.HasPrefix(tr.Artists[0].ImgURL, "/artist-images/") {
		t.Fatalf("track artist ImgURL = %q", tr.Artists[0].ImgURL)
	}
}

func TestRenameAndMergeArtists(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist("Alpha")
	b, _ := AddArtist("Beta")
	if err := RenameArtist(a.ID.String(), "beta"); !errors.Is(err, ErrArtistExists) {
		t.Fatalf("rename to a taken name = %v", err)
	}
	if err := RenameArtist(a.ID.String(), "ALPHA Band"); err != nil {
		t.Fatal(err)
	}
	if err := RenameArtist("nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rename missing = %v", err)
	}

	r, _ := CreateRelease("Split", audio.ReleaseAlbum, "", []string{a.ID.String(), b.ID.String()})
	only := addTestTrack(t, "Only A", "1", a.ID.String())
	both := addTestTrack(t, "Both", "2", a.ID.String(), b.ID.String())
	PlaceTrack(both, ReleaseLink{ReleaseID: r.ID.String(), Disc: 1, Position: 1})
	DB.Exec(`UPDATE artists SET mbid = 'mb-alpha' WHERE uuid = ?;`, a.ID.String())
	if err := SetFollow(a.ID.String(), "mb-alpha", DefaultFilters()); err != nil {
		t.Fatal(err)
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	SaveArtistImage(a.ID, bytes.NewReader(png))
	if err := SetArtistTwitch(a.ID.String(), "alpha_band"); err != nil {
		t.Fatal(err)
	}

	if err := MergeArtists(a.ID.String(), a.ID.String()); !errors.Is(err, ErrBadMerge) {
		t.Fatalf("self merge = %v", err)
	}
	if err := MergeArtists(a.ID.String(), b.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := GetArtist(All, a.ID.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("merged artist = %v", err)
	}
	for _, id := range []string{only, both} {
		tr, _ := GetTrack(All, id)
		if len(tr.Artists) != 1 || tr.Artists[0].ID != b.ID {
			t.Fatalf("%s artists = %+v", tr.Title, tr.Artists)
		}
	}
	rel, _ := GetRelease(All, r.ID.String())
	if len(rel.Artists) != 1 || rel.Artists[0].ID != b.ID {
		t.Fatalf("release artists = %+v", rel.Artists)
	}
	got, _ := GetArtist(All, b.ID.String())
	if got.TwitchLogin != "alpha_band" {
		t.Fatalf("twitch after merge = %q", got.TwitchLogin)
	}
	if got.MBID != "mb-alpha" || !got.Followed || !strings.HasPrefix(got.ImgURL, "/artist-images/") {
		t.Fatalf("kept artist = %+v", got)
	}
}
