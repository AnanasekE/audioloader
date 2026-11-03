package db

import (
	"database/sql"
	"errors"
)

// Import statuses.
const (
	ImportQueued  = "queued"
	ImportRunning = "running"
	ImportDone    = "done"
	ImportFailed  = "failed"
)

// States of a song of an Apple Music import.
const (
	AppleSongPending  = "pending"
	AppleSongMatched  = "matched"   // a sure match on MusicBrainz
	AppleSongUnsure   = "unsure"    // a candidate that needs a look
	AppleSongNotFound = "not_found" // nothing on MusicBrainz
	AppleSongAdded    = "added"     // queued from this import
	AppleSongOwned    = "owned"     // the library had it already
)

// AppleImport is an import of a public Apple Music playlist.
type AppleImport struct {
	ID         int64
	URL        string
	Name       string
	TrackCount int // what Apple says the playlist has; the page may list fewer
	Status     string
	Error      string
	PlaylistID string // the playlist made from it; "" for none yet
	CreatedAt  string
}

// AppleImportTrack is one song of an import and what it was matched to.
type AppleImportTrack struct {
	ImportID   int64
	Position   int // 1-based, Apple's order
	Title      string
	Artist     string
	Album      string
	DurationMs int

	// the MusicBrainz recording and release it was matched to (the best
	// candidate when unsure), with their names; "" when not_found
	RecordingID  string
	ReleaseID    string
	MatchTitle   string
	MatchArtist  string
	ReleaseTitle string
	Score        float64
	State        string
}

// CreateAppleImport adds an import of the playlist at url, waiting for its
// job to read the page.
func CreateAppleImport(url string) (int64, error) {
	res, err := DB.Exec(`INSERT INTO apple_imports(url) VALUES(?);`, url)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const appleImportCols = `id, url, name, track_count, status, error, COALESCE(playlist_id, ''), created_at`

func scanAppleImport(s scanner) (*AppleImport, error) {
	var i AppleImport
	err := s.Scan(&i.ID, &i.URL, &i.Name, &i.TrackCount, &i.Status, &i.Error, &i.PlaylistID, &i.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &i, err
}

// GetAppleImport returns the import, or ErrNotFound.
func GetAppleImport(id int64) (*AppleImport, error) {
	return scanAppleImport(DB.QueryRow(`SELECT `+appleImportCols+` FROM apple_imports WHERE id = ?;`, id))
}

// ListAppleImports returns the newest imports.
func ListAppleImports(limit int) ([]*AppleImport, error) {
	rows, err := DB.Query(`SELECT `+appleImportCols+` FROM apple_imports ORDER BY id DESC LIMIT ?;`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AppleImport
	for rows.Next() {
		i, err := scanAppleImport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// SetAppleImportStatus sets the status and the error to show with it.
func SetAppleImportStatus(id int64, status, errMsg string) error {
	return expectRow(DB.Exec(`UPDATE apple_imports SET status = ?, error = ? WHERE id = ?;`, status, errMsg, id))
}

// SetAppleImportPlaylist remembers the playlist made from the import.
func SetAppleImportPlaylist(id int64, playlistID string) error {
	return expectRow(DB.Exec(`UPDATE apple_imports SET playlist_id = ? WHERE id = ?;`, playlistID, id))
}

// StartAppleImport records what the page said: the playlist's name and
// count, and its songs as pending (the Position of each is set from its
// place in tracks). A second call is ignored, so a retried job keeps the
// matches it has made.
func StartAppleImport(id int64, name string, trackCount int, tracks []AppleImportTrack) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM apple_import_tracks WHERE import_id = ?;`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if err := expectRow(tx.Exec(`UPDATE apple_imports SET name = ?, track_count = ? WHERE id = ?;`, name, trackCount, id)); err != nil {
		return err
	}
	for i, t := range tracks {
		_, err := tx.Exec(`
			INSERT INTO apple_import_tracks(import_id, position, title, artist, album, duration_ms)
			VALUES(?, ?, ?, ?, ?, ?);`, id, i+1, t.Title, t.Artist, t.Album, t.DurationMs)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AppleImportTracks returns the songs of an import in Apple's order.
func AppleImportTracks(id int64) ([]*AppleImportTrack, error) {
	rows, err := DB.Query(`
		SELECT import_id, position, title, artist, album, duration_ms, recording_id, release_id,
			match_title, match_artist, release_title, score, state
		FROM apple_import_tracks WHERE import_id = ? ORDER BY position;`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AppleImportTrack
	for rows.Next() {
		var t AppleImportTrack
		if err := rows.Scan(&t.ImportID, &t.Position, &t.Title, &t.Artist, &t.Album, &t.DurationMs, &t.RecordingID, &t.ReleaseID,
			&t.MatchTitle, &t.MatchArtist, &t.ReleaseTitle, &t.Score, &t.State); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// SetAppleTrackMatch sets what a song was matched to, and its state. The
// Title, Artist and Album of t are left alone.
func SetAppleTrackMatch(t AppleImportTrack) error {
	return expectRow(DB.Exec(`
		UPDATE apple_import_tracks SET recording_id = ?, release_id = ?, match_title = ?, match_artist = ?,
			release_title = ?, score = ?, state = ?
		WHERE import_id = ? AND position = ?;`,
		t.RecordingID, t.ReleaseID, t.MatchTitle, t.MatchArtist, t.ReleaseTitle, t.Score, t.State, t.ImportID, t.Position))
}

// SetAppleTrackState sets only the state of a song.
func SetAppleTrackState(importID int64, position int, state string) error {
	return expectRow(DB.Exec(`UPDATE apple_import_tracks SET state = ? WHERE import_id = ? AND position = ?;`, state, importID, position))
}

// AppleImportCounts returns how many songs of an import are in each state.
func AppleImportCounts(id int64) (map[string]int, error) {
	rows, err := DB.Query(`SELECT state, COUNT(*) FROM apple_import_tracks WHERE import_id = ? GROUP BY state;`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		counts[s] = n
	}
	return counts, rows.Err()
}
