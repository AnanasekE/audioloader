package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/glebarez/go-sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS tracks (
    uuid TEXT UNIQUE NOT NULL PRIMARY KEY,
    title TEXT NOT NULL,
    duration_seconds INTEGER NOT NULL,
    img_url TEXT DEFAULT "/assets/img/default_track.png",
    file_size INTEGER,
    head_hash TEXT
    );

CREATE TABLE IF NOT EXISTS artists (
    uuid TEXT UNIQUE NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    img_url TEXT DEFAULT "/assets/img/default_pfp.png"
    );

CREATE TABLE IF NOT EXISTS track_artist_map (
    track_id TEXT,
    artist_id TEXT,
    FOREIGN KEY(track_id) REFERENCES tracks(uuid) ON DELETE CASCADE,
    FOREIGN KEY(artist_id) REFERENCES artists(uuid) ON DELETE CASCADE
    );

CREATE TABLE IF NOT EXISTS releases (
    uuid TEXT NOT NULL PRIMARY KEY,
    title TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('album', 'ep', 'single', 'compilation')),
    release_date TEXT,
    img_url TEXT DEFAULT "/assets/img/default_track.png"
    );

CREATE TABLE IF NOT EXISTS release_artist_map (
    release_id TEXT NOT NULL REFERENCES releases(uuid) ON DELETE CASCADE,
    artist_id TEXT NOT NULL REFERENCES artists(uuid) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    PRIMARY KEY (release_id, artist_id)
    );

-- a track can be on several releases, e.g. a single and later the album
CREATE TABLE IF NOT EXISTS release_tracks (
    release_id TEXT NOT NULL REFERENCES releases(uuid) ON DELETE CASCADE,
    track_id TEXT NOT NULL REFERENCES tracks(uuid) ON DELETE CASCADE,
    disc INTEGER NOT NULL DEFAULT 1 CHECK (disc >= 1),
    position INTEGER NOT NULL CHECK (position >= 1),
    PRIMARY KEY (release_id, disc, position),
    UNIQUE (release_id, track_id)
    );
`

var DB *sql.DB

// Init opens the database at AUDIOLOADER_DB, ./database.db by default, and
// migrates it.
func Init() {
	path := os.Getenv("AUDIOLOADER_DB")
	if path == "" {
		path = "./database.db"
	}
	db, err := Open(path)
	if err != nil {
		panic(err)
	}
	if err := migrate(db, backupBeforeMigration(db, path)); err != nil {
		panic(err)
	}
	DB = db
	fmt.Println("Connected to the SQLite database successfully.")
}

// Open opens the SQLite database at path with foreign keys enforced.
func Open(path string) (*sql.DB, error) {
	// SQLite leaves foreign keys off unless enabled per connection; the DSN
	// pragmas apply to every connection in the pool. Background workers and
	// requests write at the same time: WAL lets reads go on during a write,
	// and the busy timeout makes a writer wait for the lock instead of failing.
	return sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
}
