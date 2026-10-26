package db

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupMigrationRemovesSecretReleases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for i := 0; i < 17; i++ { // the database as it was before secret mode
		if err := runMigration(d, i); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO tracks(uuid, title, duration_seconds, secret) VALUES ('n', 'Normal', 60, 0), ('s1', 'Secret One', 60, 1), ('s2', 'Secret Two', 60, 1);`)
	exec(`INSERT INTO releases(uuid, title, type) VALUES ('mixed', 'Mixed', 'album'), ('only', 'Only Secret', 'single'), ('none', 'No Tracks', 'ep');`)
	exec(`INSERT INTO release_tracks(release_id, track_id, disc, position) VALUES
		('mixed', 'n', 1, 1), ('mixed', 's1', 1, 2), ('only', 's1', 1, 1), ('only', 's2', 1, 2);`)
	exec(`INSERT INTO playlists(uuid, name) VALUES ('p', 'Mixed list');`)
	exec(`INSERT INTO playlist_tracks(playlist_id, position, track_id) VALUES ('p', 1, 'n'), ('p', 2, 's1');`)

	var logged bytes.Buffer
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)
	if err := migrate(d, backupBeforeMigration(d, path)); err != nil {
		t.Fatal(err)
	}

	count := func(q string) (n int) {
		t.Helper()
		if err := d.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM releases WHERE uuid = 'only';`); n != 0 {
		t.Error("the release of only secret tracks is still there")
	}
	if n := count(`SELECT COUNT(*) FROM releases;`); n != 2 {
		t.Errorf("releases = %d, want the mixed and the empty one", n)
	}
	if n := count(`SELECT COUNT(*) FROM release_tracks;`); n != 1 {
		t.Errorf("release_tracks = %d, want only the normal track's", n)
	}
	if n := count(`SELECT COUNT(*) FROM tracks;`); n != 3 {
		t.Errorf("tracks = %d, they must stay", n)
	}
	if n := count(`SELECT COUNT(*) FROM playlist_tracks;`); n != 2 {
		t.Errorf("playlist entries = %d, a mixed playlist stays as it is", n)
	}
	if n := count(`SELECT secret FROM playlists WHERE uuid = 'p';`); n != 0 {
		t.Error("a mixed playlist must go to normal mode")
	}
	if !strings.Contains(logged.String(), `"Only Secret"`) || strings.Contains(logged.String(), `"Mixed"`) {
		t.Errorf("log = %q", logged.String())
	}

	// the backup is the database from before the cleanup
	backup, err := Open(path + ".bak-18")
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	var n int
	if err := backup.QueryRow(`SELECT COUNT(*) FROM releases;`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("backup releases = %d, %v", n, err)
	}
	var v int
	if err := backup.QueryRow(`PRAGMA user_version;`).Scan(&v); err != nil || v != 17 {
		t.Fatalf("backup version = %d, %v", v, err)
	}
}

func TestBackupSkipsEmptyAndKeepsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := migrate(d, backupBeforeMigration(d, path)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak-18"); err == nil {
		t.Fatal("a new database has nothing to back up")
	}

	// a backup made earlier is not overwritten
	d.Exec(`INSERT INTO tracks(uuid, title, duration_seconds) VALUES ('a', 'A', 1);`)
	os.WriteFile(path+".bak-18", []byte("earlier"), 0o600)
	if err := backupBeforeMigration(d, path)(18); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path + ".bak-18"); string(b) != "earlier" {
		t.Fatal("overwrote the earlier backup")
	}
}
