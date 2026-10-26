package db

import (
	"database/sql"
	"errors"
	"time"
	"uuid"

	"audioloader/internal/audio"
)

func CreatePlaylist(name string) (*audio.Playlist, error) {
	p := &audio.Playlist{ID: uuid.New(), Name: name}
	_, err := DB.Exec(`INSERT INTO playlists(uuid, name) VALUES(?, ?);`, p.ID.String(), name)
	return p, err
}

func RenamePlaylist(id, name string) error {
	return expectRow(DB.Exec(`UPDATE playlists SET name = ? WHERE uuid = ?;`, name, id))
}

func DeletePlaylist(id string) error {
	return expectRow(DB.Exec(`DELETE FROM playlists WHERE uuid = ?;`, id))
}

// expectRow turns "no row affected" into ErrNotFound.
func expectRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPlaylists returns all playlists with their track count, length and
// image, newest first.
func ListPlaylists(v Viewer) ([]*audio.Playlist, error) {
	rows, err := DB.Query(`
		SELECT p.uuid, p.name, COUNT(t.uuid), COALESCE(SUM(t.duration_seconds), 0),
			COALESCE((SELECT t2.img_url FROM playlist_tracks pt2 JOIN tracks t2 ON t2.uuid = pt2.track_id
				WHERE pt2.playlist_id = p.uuid AND `+v.trackCond("t2")+` ORDER BY pt2.position LIMIT 1), ?)
		FROM playlists p
		LEFT JOIN playlist_tracks pt ON pt.playlist_id = p.uuid
		LEFT JOIN tracks t ON t.uuid = pt.track_id AND `+v.trackCond("t")+`
		GROUP BY p.uuid
		ORDER BY p.created_at DESC, p.rowid DESC;`, DefaultCover)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*audio.Playlist
	for rows.Next() {
		var p audio.Playlist
		var seconds int
		if err := rows.Scan(&p.ID, &p.Name, &p.Count, &seconds, &p.ImgURL); err != nil {
			return nil, err
		}
		p.Duration = time.Duration(seconds) * time.Second
		out = append(out, &p)
	}
	return out, rows.Err()
}

// GetPlaylist returns a playlist with its tracks in order.
func GetPlaylist(v Viewer, id string) (*audio.Playlist, error) {
	var p audio.Playlist
	err := DB.QueryRow(`SELECT uuid, name FROM playlists WHERE uuid = ?;`, id).Scan(&p.ID, &p.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := DB.Query(selectTrack+` JOIN playlist_tracks pt ON pt.track_id = t.uuid
		WHERE pt.playlist_id = ? AND `+v.trackCond("t")+` ORDER BY pt.position;`, id)
	if err != nil {
		return nil, err
	}
	// the same track can appear twice; attach its details once
	byID := map[string]*audio.Track{}
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if seen := byID[t.ID.String()]; seen != nil {
			t = seen
		} else {
			byID[t.ID.String()] = t
		}
		p.Tracks = append(p.Tracks, t)
		p.Duration += t.Duration
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	unique := make([]*audio.Track, 0, len(byID))
	for _, t := range byID {
		unique = append(unique, t)
	}
	if err := attachArtists(unique); err != nil {
		return nil, err
	}
	if err := attachReleases(unique); err != nil {
		return nil, err
	}
	if err := attachCovers(v, unique); err != nil {
		return nil, err
	}
	p.Count = len(p.Tracks)
	p.ImgURL = DefaultCover
	if len(p.Tracks) > 0 {
		p.ImgURL = p.Tracks[0].ImgURL
	}
	return &p, nil
}

// AppendToPlaylist adds a track at the end of a playlist.
func AppendToPlaylist(playlistID, trackID string) error {
	_, err := DB.Exec(`
		INSERT INTO playlist_tracks(playlist_id, position, track_id)
		SELECT ?, COALESCE(MAX(position), 0) + 1, ? FROM playlist_tracks WHERE playlist_id = ?;`,
		playlistID, trackID, playlistID)
	if isForeignKeyViolation(err) {
		return ErrBadReference
	}
	return err
}

// SetPlaylistTracks replaces a playlist's tracks with trackIDs, in order.
// Secret entries a locked viewer didn't see keep their places.
func SetPlaylistTracks(v Viewer, playlistID string, trackIDs []string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM playlists WHERE uuid = ?);`, playlistID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	taken := map[int]bool{}
	if !v.Secret {
		rows, err := tx.Query(`
			SELECT pt.position FROM playlist_tracks pt JOIN tracks t ON t.uuid = pt.track_id
			WHERE pt.playlist_id = ? AND t.secret = 1;`, playlistID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var pos int
			if err := rows.Scan(&pos); err != nil {
				rows.Close()
				return err
			}
			taken[pos] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`
		DELETE FROM playlist_tracks WHERE playlist_id = ?
		AND track_id IN (SELECT uuid FROM tracks t WHERE `+v.trackCond("t")+`);`, playlistID)
	if err != nil {
		return err
	}
	pos := 0
	for _, id := range trackIDs {
		pos++
		for taken[pos] {
			pos++
		}
		_, err := tx.Exec(`INSERT INTO playlist_tracks(playlist_id, position, track_id) VALUES(?, ?, ?);`, playlistID, pos, id)
		if isForeignKeyViolation(err) {
			return ErrBadReference
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
