package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"audioloader/internal/audio"
)

// ErrMBIDTaken is returned when another artist already has the MusicBrainz ID.
var ErrMBIDTaken = errors.New("another artist already has that MusicBrainz ID")

// FollowFilters picks which of a followed artist's releases reach the inbox.
type FollowFilters struct {
	// release types to track: album, ep, single, compilation
	Types []string `json:"types"`
	// MusicBrainz secondary types to skip, e.g. "Live", "Remix"
	SkipSecondary []string `json:"skip_secondary"`
}

// DefaultFilters tracks studio releases and skips the usual extras.
func DefaultFilters() FollowFilters {
	return FollowFilters{
		Types:         []string{"album", "ep", "single"},
		SkipSecondary: []string{"Compilation", "Live", "Remix", "DJ-mix", "Demo", "Interview", "Spokenword"},
	}
}

// SecondaryTypes are MusicBrainz's secondary release group types, for the filter form.
var SecondaryTypes = []string{
	"Compilation", "Live", "Remix", "DJ-mix", "Mixtape/Street", "Demo", "Soundtrack",
	"Spokenword", "Interview", "Audiobook", "Audio drama", "Field recording",
}

type Follow struct {
	Artist      *audio.Artist
	Filters     FollowFilters
	LastChecked string // "" if never
}

// SetFollow follows an artist, linked to MusicBrainz artist mbid, with the
// given filters. Following again updates the filters.
func SetFollow(artistID, mbid string, f FollowFilters) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE artists SET mbid = ? WHERE uuid = ?;`, mbid, artistID)
	if isUniqueViolation(err) {
		return ErrMBIDTaken
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = tx.Exec(`
		INSERT INTO artist_follows(artist_id, filters) VALUES(?, ?)
		ON CONFLICT(artist_id) DO UPDATE SET filters = excluded.filters;`, artistID, string(data))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func Unfollow(artistID string) error {
	_, err := DB.Exec(`DELETE FROM artist_follows WHERE artist_id = ?;`, artistID)
	return err
}

const selectFollow = `
	SELECT a.uuid, a.name, a.img_url, COALESCE(a.mbid, ''), f.filters, COALESCE(f.last_checked, '')
	FROM artist_follows f JOIN artists a ON a.uuid = f.artist_id`

func scanFollow(s scanner) (*Follow, error) {
	f := Follow{Artist: &audio.Artist{}}
	var filters string
	if err := s.Scan(&f.Artist.ID, &f.Artist.Name, &f.Artist.ImgURL, &f.Artist.MBID, &filters, &f.LastChecked); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(filters), &f.Filters); err != nil {
		return nil, err
	}
	return &f, nil
}

// GetFollow returns the follow of an artist, or ErrNotFound.
func GetFollow(artistID string) (*Follow, error) {
	f, err := scanFollow(DB.QueryRow(selectFollow+` WHERE f.artist_id = ?;`, artistID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return f, err
}

func ListFollows() ([]*Follow, error) {
	rows, err := DB.Query(selectFollow + ` ORDER BY a.name COLLATE NOCASE;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Follow
	for rows.Next() {
		f, err := scanFollow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FollowsDue returns the followed artists not checked within every.
func FollowsDue(every time.Duration) ([]string, error) {
	rows, err := DB.Query(
		`SELECT artist_id FROM artist_follows WHERE last_checked IS NULL OR last_checked <= ? ORDER BY last_checked;`,
		sqlTime(time.Now().Add(-every)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func MarkFollowChecked(artistID string) error {
	_, err := DB.Exec(`UPDATE artist_follows SET last_checked = ? WHERE artist_id = ?;`, sqlTime(time.Now()), artistID)
	return err
}

// sqlTime formats t like SQLite's datetime(), so it compares as text.
func sqlTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}
