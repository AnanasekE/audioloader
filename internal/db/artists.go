package db

import (
	"database/sql"
	"errors"
	"strings"
	"uuid"

	"audioloader/internal/audio"
)

// ErrArtistExists is returned when an artist with the same name (ignoring case) exists.
var ErrArtistExists = errors.New("an artist with that name already exists")

func ListArtists(v Viewer) ([]*audio.Artist, error) {
	rows, err := DB.Query(`
		SELECT a.uuid, a.name, a.img_url, COALESCE(a.mbid, ''), f.artist_id IS NOT NULL
		FROM artists a LEFT JOIN artist_follows f ON f.artist_id = a.uuid
		WHERE ` + v.artistCond("a") + `
		ORDER BY a.name COLLATE NOCASE;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	artists := make([]*audio.Artist, 0)
	for rows.Next() {
		var a audio.Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.ImgURL, &a.MBID, &a.Followed); err != nil {
			return nil, err
		}
		artists = append(artists, &a)
	}
	return artists, rows.Err()
}

// GetArtist returns one artist.
func GetArtist(v Viewer, id string) (*audio.Artist, error) {
	var a audio.Artist
	err := DB.QueryRow(`
		SELECT a.uuid, a.name, a.img_url, COALESCE(a.mbid, ''), f.artist_id IS NOT NULL
		FROM artists a LEFT JOIN artist_follows f ON f.artist_id = a.uuid
		WHERE a.uuid = ? AND `+v.artistCond("a")+`;`, id).Scan(&a.ID, &a.Name, &a.ImgURL, &a.MBID, &a.Followed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &a, err
}

func AddArtist(name string) (*audio.Artist, error) {
	a := &audio.Artist{ID: uuid.New(), Name: name}
	_, err := DB.Exec(
		`INSERT INTO artists(uuid, name) VALUES(?, ?);`,
		a.ID.String(), a.Name,
	)
	if isUniqueViolation(err) {
		return nil, ErrArtistExists
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ResolveArtists maps each name to an artist ID, matching existing artists
// case-insensitively and creating the missing ones, all in one transaction.
func ResolveArtists(names []string) (map[string]string, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ids := make(map[string]string, len(names))
	for _, name := range names {
		if _, done := ids[name]; done {
			continue
		}
		var id string
		err := tx.QueryRow(`SELECT uuid FROM artists WHERE lower(name) = lower(?) LIMIT 1;`, name).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			id = uuid.New().String()
			_, err = tx.Exec(`INSERT INTO artists(uuid, name) VALUES(?, ?);`, id, name)
		}
		if err != nil {
			return nil, err
		}
		ids[name] = id
	}
	return ids, tx.Commit()
}

// mergeDuplicateArtists folds artists whose trimmed, lowercased names match
// into the oldest of them, moving their track and release links over.
func mergeDuplicateArtists(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT uuid, name FROM artists ORDER BY rowid;`)
	if err != nil {
		return err
	}
	keep := map[string]string{} // normalized name -> kept uuid
	dups := map[string]string{} // duplicate uuid -> kept uuid
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return err
		}
		key := strings.ToLower(strings.TrimSpace(name))
		if k, ok := keep[key]; ok {
			dups[id] = k
		} else {
			keep[key] = id
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for dup, k := range dups {
		stmts := []string{
			`UPDATE track_artist_map SET artist_id = ? WHERE artist_id = ?;`,
			// the release map has a primary key, so skip rows the kept artist already has
			`UPDATE OR IGNORE release_artist_map SET artist_id = ? WHERE artist_id = ?;`,
		}
		for _, q := range stmts {
			if _, err := tx.Exec(q, k, dup); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`DELETE FROM artists WHERE uuid = ?;`, dup); err != nil {
			return err
		}
	}
	// a track credited to both artists now has the kept one twice
	_, err = tx.Exec(`
		DELETE FROM track_artist_map WHERE rowid NOT IN (
			SELECT MIN(rowid) FROM track_artist_map GROUP BY track_id, artist_id
		);`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE artists SET name = trim(name) WHERE name != trim(name);`)
	return err
}

// ListArtistReleases returns the releases credited to an artist, newest
// first, as ListReleases orders them.
func ListArtistReleases(v Viewer, artistID string) ([]*audio.Release, error) {
	rows, err := DB.Query(selectRelease+` r WHERE `+v.releaseCond("r")+`
		AND r.uuid IN (SELECT release_id FROM release_artist_map WHERE artist_id = ?)
		ORDER BY release_date IS NULL, release_date DESC, title;`, artistID)
	if err != nil {
		return nil, err
	}
	releases := make([]*audio.Release, 0)
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		releases = append(releases, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return releases, attachReleaseArtists(releases)
}

// ListArtistTracks returns the library tracks credited to an artist that
// are on none of the artist's own releases, e.g. features on other
// artists' albums.
func ListArtistTracks(v Viewer, artistID string) ([]*audio.Track, error) {
	rows, err := DB.Query(selectTrack+` WHERE `+libraryTracks+` AND `+v.trackCond("t")+`
		AND t.uuid IN (SELECT track_id FROM track_artist_map WHERE artist_id = ?)
		AND NOT EXISTS (
			SELECT 1 FROM release_tracks rt JOIN release_artist_map ra ON ra.release_id = rt.release_id
			WHERE rt.track_id = t.uuid AND ra.artist_id = ?)
		ORDER BY t.title COLLATE NOCASE;`, artistID, artistID)
	if err != nil {
		return nil, err
	}
	tracks := make([]*audio.Track, 0)
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		tracks = append(tracks, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := attachArtists(tracks); err != nil {
		return nil, err
	}
	return tracks, attachReleases(tracks)
}
