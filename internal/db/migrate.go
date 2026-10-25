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
