package db

import (
	"database/sql"
	"fmt"

	_ "github.com/glebarez/go-sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS tracks (
    uuid TEXT UNIQUE NOT NULL PRIMARY KEY,
    title TEXT NOT NULL,
    duration_seconds INTEGER NOT NULL,
    img_url TEXT DEFAULT "/assets/img/default_track.png"
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

func Migrate(db *sql.DB) {
	_, err := db.Exec(schema)
	if err != nil {
		panic(err)
	}
}
