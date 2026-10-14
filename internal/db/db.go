package db

import (
	"database/sql"
	"fmt"

	_ "github.com/glebarez/go-sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS artists (
    id      TEXT PRIMARY KEY,
    name    TEXT NOT NULL,
    img_url TEXT
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS tracks (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    duration_ms INTEGER NOT NULL CHECK (duration_ms >= 0),
    img_url     TEXT
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS albums (
    id      TEXT PRIMARY KEY,
    name    TEXT NOT NULL,
    img_url TEXT
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS playlists (
    id      TEXT PRIMARY KEY,
    name    TEXT NOT NULL,
    img_url TEXT
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS track_artists (
    track_id  TEXT NOT NULL REFERENCES tracks(id)  ON DELETE CASCADE,
    artist_id TEXT NOT NULL REFERENCES artists(id) ON DELETE RESTRICT,
    position  INTEGER NOT NULL,
    PRIMARY KEY (track_id, artist_id),
    UNIQUE (track_id, position)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_track_artists_artist ON track_artists(artist_id);

CREATE TABLE IF NOT EXISTS album_tracks (
    album_id TEXT NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE RESTRICT,
    PRIMARY KEY (album_id, position)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_album_tracks_track ON album_tracks(track_id);

CREATE TABLE IF NOT EXISTS playlist_tracks (
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    track_id    TEXT NOT NULL REFERENCES tracks(id) ON DELETE RESTRICT,
    PRIMARY KEY (playlist_id, position)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_playlist_tracks_track ON playlist_tracks(track_id);

CREATE TABLE IF NOT EXISTS releases (
    id           TEXT PRIMARY KEY,
    release_date TEXT NOT NULL CHECK (date(release_date) IS release_date),
    track_id     TEXT REFERENCES tracks(id) ON DELETE RESTRICT,
    album_id     TEXT REFERENCES albums(id) ON DELETE RESTRICT,
    CHECK ((track_id IS NULL) <> (album_id IS NULL))
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_releases_track ON releases(track_id);
CREATE INDEX IF NOT EXISTS idx_releases_album ON releases(album_id);
CREATE INDEX IF NOT EXISTS idx_releases_date  ON releases(release_date);
`

var DB *sql.DB

func Init() {
	DB = connect()
	Migrate(DB)
}

func connect() *sql.DB {
	// Connect to the SQLite database
	db, err := sql.Open("sqlite", "./database.db")
	if err != nil {
		fmt.Println(err)
		return nil
	}

	fmt.Println("Connected to the SQLite database successfully.")
	return db
}

func Migrate(db *sql.DB) error {
	_, err := db.Exec(schema)
	return err
}
