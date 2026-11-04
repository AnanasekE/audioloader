package db

import (
	"database/sql"
	"errors"
	"time"
)

// AppleFollow is a public Apple Music playlist that is checked now and then
// for new songs, which go onto its import.
type AppleFollow struct {
	ID            int64
	URL           string
	Name          string
	ImportID      int64
	LastCheckedAt string // "" before the first check
	LastError     string
	ToPick        int // songs of the import that are unsure or not found
	CreatedAt     string
}

const appleFollowCols = `f.id, f.url, f.name, f.import_id, COALESCE(f.last_checked_at, ''), f.last_error,
	(SELECT COUNT(*) FROM apple_import_tracks t WHERE t.import_id = f.import_id AND t.state IN ('unsure', 'not_found')),
	f.created_at`

func scanAppleFollow(s scanner) (*AppleFollow, error) {
	var f AppleFollow
	err := s.Scan(&f.ID, &f.URL, &f.Name, &f.ImportID, &f.LastCheckedAt, &f.LastError, &f.ToPick, &f.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &f, err
}

// FollowApplePlaylist follows the playlist of an import and marks its songs
// as seen. Following the same URL again returns the follow it has.
func FollowApplePlaylist(importID int64) (*AppleFollow, error) {
	imp, err := GetAppleImport(importID)
	if err != nil {
		return nil, err
	}
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRow(`SELECT id FROM apple_follows WHERE url = ?;`, imp.URL).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.Exec(`INSERT INTO apple_follows(url, name, import_id) VALUES(?, ?, ?);`, imp.URL, imp.Name, importID)
		if err != nil {
			return nil, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return nil, err
		}
		_, err = tx.Exec(`
			INSERT OR IGNORE INTO apple_follow_songs(follow_id, apple_song_id)
			SELECT ?, apple_song_id FROM apple_import_tracks WHERE import_id = ? AND apple_song_id != '';`, id, importID)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetAppleFollow(id)
}

// GetAppleFollow returns the follow, or ErrNotFound.
func GetAppleFollow(id int64) (*AppleFollow, error) {
	return scanAppleFollow(DB.QueryRow(`SELECT `+appleFollowCols+` FROM apple_follows f WHERE f.id = ?;`, id))
}

// AppleFollowOfImport returns the follow whose songs go onto the import, or
// ErrNotFound.
func AppleFollowOfImport(importID int64) (*AppleFollow, error) {
	return scanAppleFollow(DB.QueryRow(`SELECT `+appleFollowCols+` FROM apple_follows f WHERE f.import_id = ?;`, importID))
}

// ListAppleFollows returns the followed playlists, oldest first.
func ListAppleFollows() ([]*AppleFollow, error) {
	rows, err := DB.Query(`SELECT ` + appleFollowCols + ` FROM apple_follows f ORDER BY f.id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AppleFollow
	for rows.Next() {
		f, err := scanAppleFollow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteAppleFollow stops following, and forgets the songs seen. The import
// and what it added stay.
func DeleteAppleFollow(id int64) error {
	return expectRow(DB.Exec(`DELETE FROM apple_follows WHERE id = ?;`, id))
}

// AppleFollowsDue returns the follows never checked, or last checked more
// than every ago, oldest check first.
func AppleFollowsDue(every time.Duration) ([]int64, error) {
	rows, err := DB.Query(
		`SELECT id FROM apple_follows WHERE last_checked_at IS NULL OR last_checked_at <= ? ORDER BY last_checked_at;`,
		sqlTime(time.Now().Add(-every)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MarkAppleFollowChecked records a check and its error ("" for none).
func MarkAppleFollowChecked(id int64, errMsg string) error {
	return expectRow(DB.Exec(`UPDATE apple_follows SET last_checked_at = ?, last_error = ? WHERE id = ?;`,
		sqlTime(time.Now()), errMsg, id))
}

// AppleFollowSeen returns the IDs of the songs the follow has seen.
func AppleFollowSeen(id int64) (map[string]bool, error) {
	rows, err := DB.Query(`SELECT apple_song_id FROM apple_follow_songs WHERE follow_id = ?;`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		seen[s] = true
	}
	return seen, rows.Err()
}

// MarkAppleFollowSeen adds song IDs to the seen set without adding songs.
func MarkAppleFollowSeen(id int64, songIDs []string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, s := range songIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO apple_follow_songs(follow_id, apple_song_id) VALUES(?, ?);`, id, s); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddAppleFollowSongs appends songs to the follow's import, as pending ones
// after its last position, and marks their Apple IDs seen, in one
// transaction. It returns the new songs' positions.
func AddAppleFollowSongs(id int64, songs []AppleImportTrack) ([]int, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var importID int64
	var last int
	err = tx.QueryRow(`
		SELECT f.import_id, COALESCE((SELECT MAX(position) FROM apple_import_tracks WHERE import_id = f.import_id), 0)
		FROM apple_follows f WHERE f.id = ?;`, id).Scan(&importID, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var positions []int
	for _, t := range songs {
		last++
		_, err := tx.Exec(`
			INSERT INTO apple_import_tracks(import_id, position, title, artist, album, duration_ms, apple_song_id)
			VALUES(?, ?, ?, ?, ?, ?, ?);`, importID, last, t.Title, t.Artist, t.Album, t.DurationMs, t.AppleID)
		if err != nil {
			return nil, err
		}
		if t.AppleID != "" {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO apple_follow_songs(follow_id, apple_song_id) VALUES(?, ?);`, id, t.AppleID); err != nil {
				return nil, err
			}
		}
		positions = append(positions, last)
	}
	return positions, tx.Commit()
}

// AppleFollowsToPick returns how many followed playlists have songs waiting
// for a pick, which the Inbox badge counts.
func AppleFollowsToPick() (int, error) {
	var n int
	err := DB.QueryRow(`
		SELECT COUNT(*) FROM apple_follows f WHERE EXISTS (
			SELECT 1 FROM apple_import_tracks t WHERE t.import_id = f.import_id AND t.state IN ('unsure', 'not_found'));`).Scan(&n)
	return n, err
}
