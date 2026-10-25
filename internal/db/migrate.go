package db

import (
	"database/sql"
	"fmt"
)

// querier is implemented by both *sql.DB and *sql.Tx.
type querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// migrations run in order, each in its own transaction; PRAGMA user_version
// records how many have been applied. Append new ones at the end and never
// change one that has shipped: existing databases won't run it again.
//
// Databases from before versioning start at 0 with their tables already in
// place, so the first migrations must be idempotent.
var migrations = []func(tx *sql.Tx) error{
	// 1: base schema
	func(tx *sql.Tx) error {
		_, err := tx.Exec(schema)
		return err
	},
	// 2: fingerprint columns, backfill and unique index
	func(tx *sql.Tx) error { return migrateFingerprints(tx) },
	// 3: merge artists that differ only in case or surrounding spaces, then
	// keep names unique
	func(tx *sql.Tx) error {
		if err := mergeDuplicateArtists(tx); err != nil {
			return err
		}
		_, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS artists_name ON artists(lower(name));`)
		return err
	},
	// 4: placeholder tracks, known from metadata but without a file yet
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`ALTER TABLE tracks ADD COLUMN has_file INTEGER NOT NULL DEFAULT 1;`)
		return err
	},
	// 5: MusicBrainz IDs, the background job queue and tagging matches
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			ALTER TABLE tracks   ADD COLUMN mbid TEXT; -- recording MBID
			ALTER TABLE tracks   ADD COLUMN isrc TEXT;
			ALTER TABLE releases ADD COLUMN mbid TEXT; -- release MBID
			ALTER TABLE releases ADD COLUMN release_group_mbid TEXT;
			ALTER TABLE artists  ADD COLUMN mbid TEXT;
			CREATE UNIQUE INDEX artists_mbid  ON artists(mbid)  WHERE mbid IS NOT NULL;
			CREATE UNIQUE INDEX releases_mbid ON releases(mbid) WHERE mbid IS NOT NULL;

			CREATE TABLE jobs (
				id INTEGER PRIMARY KEY,
				type TEXT NOT NULL,
				payload TEXT NOT NULL, -- JSON
				-- optional: at most one queued or running job per (type, key)
				dedupe_key TEXT,
				status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
				attempts INTEGER NOT NULL DEFAULT 0,
				last_error TEXT,
				run_after TEXT NOT NULL DEFAULT (datetime('now')),
				created_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX jobs_next ON jobs(status, run_after);
			CREATE UNIQUE INDEX jobs_dedupe ON jobs(type, dedupe_key)
				WHERE dedupe_key IS NOT NULL AND status IN ('queued', 'running');

			CREATE TABLE tag_matches (
				track_id TEXT NOT NULL REFERENCES tracks(uuid) ON DELETE CASCADE,
				source TEXT NOT NULL CHECK (source IN ('tags', 'acoustid', 'search')),
				recording_mbid TEXT NOT NULL,
				release_mbid TEXT,
				score REAL NOT NULL,
				data TEXT NOT NULL, -- JSON snapshot of what would be applied
				status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'applied', 'rejected')),
				PRIMARY KEY (track_id, recording_mbid)
			);`)
		return err
	},
	// 6: outcome of the last auto-tagging run per track
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`ALTER TABLE tracks ADD COLUMN tag_status TEXT CHECK (tag_status IN ('matched', 'review', 'none'));`)
		return err
	},
}

// Migrate brings db up to the latest schema version.
func Migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version;`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		if err := runMigration(db, i); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	return nil
}

func runMigration(db *sql.DB, i int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := migrations[i](tx); err != nil {
		return err
	}
	// PRAGMA doesn't take bound parameters; i is an int, so this is safe
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d;`, i+1)); err != nil {
		return err
	}
	return tx.Commit()
}
