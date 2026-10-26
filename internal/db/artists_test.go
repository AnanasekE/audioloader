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
