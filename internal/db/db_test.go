package db

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"audioloader/internal/audio"
)

// newTestDB points DB and TracksDir at a fresh, migrated database in a temp dir.
func newTestDB(t *testing.T) {
	t.Helper()
	restore, err := UseTemp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
}

// addTestTrack stores a track whose file content is body.
func addTestTrack(t *testing.T, title string, body string, artistIDs ...string) string {
	t.Helper()
	res, err := AddTrack(artistIDs, &audio.Track{Title: title, Duration: time.Minute}, bytes.NewReader([]byte(body)), ".mp3", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return res.TrackID
}

func TestMigrateTwice(t *testing.T) {
	newTestDB(t)
	if err := Migrate(DB); err != nil {
		t.Fatal(err)
	}
	var v int
	if err := DB.QueryRow(`PRAGMA user_version;`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != len(migrations) {
		t.Fatalf("user_version = %d, want %d", v, len(migrations))
	}
}

func TestAddTrackDuplicate(t *testing.T) {
	newTestDB(t)
	addTestTrack(t, "a", "same bytes")
	_, err := AddTrack(nil, &audio.Track{Title: "b"}, bytes.NewReader([]byte("same bytes")), ".mp3", nil, "")
	if !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
}

func TestMergeDuplicateArtists(t *testing.T) {
	newTestDB(t)
	// seed duplicates the way a pre-migration database could have them
	if _, err := DB.Exec(`DROP INDEX artists_name;`); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, name := range []string{"Foo", "foo ", "FOO", "Bar"} {
		a, err := AddArtist(name)
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = a.ID.String()
	}
	tr := addTestTrack(t, "song", "x", ids["foo "], ids["FOO"], ids["Bar"])
	r, err := CreateRelease("rel", audio.ReleaseAlbum, "", []string{ids["Foo"], ids["FOO"]})
	if err != nil {
		t.Fatal(err)
	}

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations[2](tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	artists, err := ListArtists()
	if err != nil {
		t.Fatal(err)
	}
	if len(artists) != 2 {
		t.Fatalf("got %d artists, want 2", len(artists))
	}
	track, err := GetTrack(tr)
	if err != nil {
		t.Fatal(err)
	}
	if len(track.Artists) != 2 || track.Artists[0].ID.String() != ids["Foo"] {
		t.Fatalf("track artists = %v", track.Artists)
	}
	rel, err := GetRelease(r.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(rel.Artists) != 1 || rel.Artists[0].Name != "Foo" {
		t.Fatalf("release artists = %v", rel.Artists)
	}
	if _, err := AddArtist("bar"); !errors.Is(err, ErrArtistExists) {
		t.Fatalf("AddArtist(bar) err = %v, want ErrArtistExists", err)
	}
}
