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
	// 7: followed artists, the inbox of their releases missing from the
	// library, and downloads filling in placeholder tracks
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			CREATE TABLE artist_follows (
				artist_id TEXT PRIMARY KEY REFERENCES artists(uuid) ON DELETE CASCADE,
				filters TEXT NOT NULL DEFAULT '{}', -- JSON, see FollowFilters
				last_checked TEXT
			);

			CREATE TABLE inbox_releases (
				release_group_mbid TEXT PRIMARY KEY,
				artist_id TEXT NOT NULL REFERENCES artists(uuid) ON DELETE CASCADE,
				release_mbid TEXT, -- the release picked to stand for the group
				title TEXT NOT NULL,
				type TEXT NOT NULL,
				date TEXT,
				data TEXT NOT NULL, -- JSON, see InboxData
				status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'confirmed', 'ignored')),
				found_at TEXT NOT NULL DEFAULT (datetime('now'))
			);

			CREATE TABLE downloads (
				track_id TEXT PRIMARY KEY REFERENCES tracks(uuid) ON DELETE CASCADE,
				status TEXT NOT NULL DEFAULT 'queued'
					CHECK (status IN ('queued', 'searching', 'downloading', 'done', 'not_found', 'failed')),
				source TEXT,
				progress REAL,
				last_error TEXT,
				updated_at TEXT NOT NULL DEFAULT (datetime('now'))
			);

			-- where a downloaded file came from, e.g. a YouTube video URL
			ALTER TABLE tracks ADD COLUMN source_url TEXT;
			CREATE INDEX tracks_source_url ON tracks(source_url) WHERE source_url IS NOT NULL;`)
		return err
	},
	// 8: full-text search over tracks, releases and artists. Triggers mark
	// the index stale on any change; Search rebuilds it before querying.
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			CREATE VIRTUAL TABLE search USING fts5(
				kind UNINDEXED, ref UNINDEXED, title, artists,
				tokenize = 'unicode61 remove_diacritics 2'
			);
			CREATE TABLE search_state (dirty INTEGER NOT NULL);
			INSERT INTO search_state(dirty) VALUES(1);
			CREATE TRIGGER search_dirty_tracks_insert AFTER INSERT ON tracks BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_tracks_update AFTER UPDATE ON tracks BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_tracks_delete AFTER DELETE ON tracks BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_releases_insert AFTER INSERT ON releases BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_releases_update AFTER UPDATE ON releases BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_releases_delete AFTER DELETE ON releases BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_artists_insert AFTER INSERT ON artists BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_artists_update AFTER UPDATE ON artists BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_artists_delete AFTER DELETE ON artists BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_track_artist_map_insert AFTER INSERT ON track_artist_map BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_track_artist_map_update AFTER UPDATE ON track_artist_map BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_track_artist_map_delete AFTER DELETE ON track_artist_map BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_release_artist_map_insert AFTER INSERT ON release_artist_map BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_release_artist_map_update AFTER UPDATE ON release_artist_map BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_release_artist_map_delete AFTER DELETE ON release_artist_map BEGIN UPDATE search_state SET dirty = 1; END;`)
		return err
	},
	// 9: playlists; a track may be on one more than once
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			CREATE TABLE playlists (
				uuid TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				created_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE TABLE playlist_tracks (
				playlist_id TEXT NOT NULL REFERENCES playlists(uuid) ON DELETE CASCADE,
				position INTEGER NOT NULL,
				track_id TEXT NOT NULL REFERENCES tracks(uuid) ON DELETE CASCADE,
				PRIMARY KEY (playlist_id, position)
			);`)
		return err
	},
	// 10: podcasts: episodes are tracks of kind "episode" grouped into
	// shows, with a saved position and chapters
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			CREATE TABLE shows (
				uuid TEXT PRIMARY KEY,
				title TEXT NOT NULL,
				author TEXT,
				img_url TEXT NOT NULL DEFAULT "/assets/img/default_track.png"
			);
			CREATE UNIQUE INDEX shows_title ON shows(lower(title));
			ALTER TABLE tracks ADD COLUMN kind TEXT NOT NULL DEFAULT 'music' CHECK (kind IN ('music', 'episode'));
			ALTER TABLE tracks ADD COLUMN show_id TEXT REFERENCES shows(uuid) ON DELETE SET NULL;
			ALTER TABLE tracks ADD COLUMN published TEXT; -- an episode's date

			CREATE TABLE playback_positions (
				track_id TEXT PRIMARY KEY REFERENCES tracks(uuid) ON DELETE CASCADE,
				seconds REAL NOT NULL,
				finished INTEGER NOT NULL DEFAULT 0,
				updated_at TEXT NOT NULL DEFAULT (datetime('now'))
			);

			CREATE TABLE chapters (
				track_id TEXT NOT NULL REFERENCES tracks(uuid) ON DELETE CASCADE,
				start_ms INTEGER NOT NULL,
				title TEXT NOT NULL,
				PRIMARY KEY (track_id, start_ms)
			);

			CREATE TRIGGER search_dirty_shows_insert AFTER INSERT ON shows BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_shows_update AFTER UPDATE ON shows BEGIN UPDATE search_state SET dirty = 1; END;
			CREATE TRIGGER search_dirty_shows_delete AFTER DELETE ON shows BEGIN UPDATE search_state SET dirty = 1; END;`)
		return err
	},
	// 11: secret tracks, visible only to a viewer who unlocked the vault;
	// the search index gets a column to filter them, so it is rebuilt
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			ALTER TABLE tracks ADD COLUMN secret INTEGER NOT NULL DEFAULT 0;
			DROP TABLE search;
			CREATE VIRTUAL TABLE search USING fts5(
				kind UNINDEXED, ref UNINDEXED, secret UNINDEXED, title, artists,
				tokenize = 'unicode61 remove_diacritics 2'
			);
			UPDATE search_state SET dirty = 1;`)
		return err
	},
	// 12: downloads can wait for a pick ('needs_pick': only YouTube results
	// are left); SQLite can't alter a CHECK, so the table is rebuilt.
	// Streaming links from MusicBrainz let spotDL skip its search.
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			CREATE TABLE downloads_new (
				track_id TEXT PRIMARY KEY REFERENCES tracks(uuid) ON DELETE CASCADE,
				status TEXT NOT NULL DEFAULT 'queued'
					CHECK (status IN ('queued', 'searching', 'downloading', 'done', 'not_found', 'failed', 'needs_pick')),
				source TEXT,
				progress REAL,
				last_error TEXT,
				updated_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
			INSERT INTO downloads_new(track_id, status, source, progress, last_error, updated_at)
				SELECT track_id, status, source, progress, last_error, updated_at FROM downloads;
			DROP TABLE downloads;
			ALTER TABLE downloads_new RENAME TO downloads;

			ALTER TABLE releases ADD COLUMN spotify_url TEXT; -- album link from MusicBrainz url-rels
			ALTER TABLE tracks ADD COLUMN spotify_url TEXT;   -- track link, from url-rels or the album mapping`)
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
