package db

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"audioloader/internal/audio"
)

// newTestDB points DB and TracksDir at a fresh, migrated database in a temp dir.
func newTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	oldDB, oldDir := DB, TracksDir
	DB, TracksDir = d, filepath.Join(dir, "tracks")
	t.Cleanup(func() {
		d.Close()
		DB, TracksDir = oldDB, oldDir
	})
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
