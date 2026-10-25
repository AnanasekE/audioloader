package db

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"audioloader/internal/audio"
)

// addTestPlaceholder adds a track without a file on release r.
func addTestPlaceholder(t *testing.T, r *audio.Release, title string, position int) string {
	t.Helper()
	id := "00000000-0000-0000-0000-00000000000" + string(rune('0'+position))
	if _, err := DB.Exec(`INSERT INTO tracks(uuid, title, duration_seconds, has_file) VALUES(?, ?, 60, 0);`, id, title); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO release_tracks(release_id, track_id, disc, position) VALUES(?, ?, 1, ?);`, r.ID.String(), id, position); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPlaceholderTracks(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "rel", audio.ReleaseAlbum)
	id := addTestPlaceholder(t, r, "missing", 1)

	tracks, err := ListTracks(All)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 0 {
		t.Fatalf("placeholder listed in library: %v", tracks)
	}
	rel, err := GetRelease(All, r.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(rel.Tracks) != 1 || rel.Tracks[0].Track.HasFile {
		t.Fatalf("release tracks = %+v", rel.Tracks)
	}

	if _, err := AttachFile(id, bytes.NewReader([]byte("audio")), ".mp3", 90*time.Second); err != nil {
		t.Fatal(err)
	}
	tracks, _ = ListTracks(All)
	if len(tracks) != 1 || !tracks[0].HasFile || tracks[0].Duration != 90*time.Second {
		t.Fatalf("after attach: %+v", tracks)
	}
	if _, err := AttachFile(id, bytes.NewReader([]byte("other")), ".mp3", time.Second); !errors.Is(err, ErrHasFile) {
		t.Fatalf("second attach err = %v", err)
	}
}

func TestAttachFileDuplicate(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "rel", audio.ReleaseAlbum)
	existing := addTestTrack(t, "song", "same")
	id := addTestPlaceholder(t, r, "song", 1)

	got, err := AttachFile(id, bytes.NewReader([]byte("same")), ".mp3", time.Second)
	if !errors.Is(err, ErrExists) || got != existing {
		t.Fatalf("got %q, %v; want %q, ErrExists", got, err, existing)
	}
}
