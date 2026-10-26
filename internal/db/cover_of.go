package db

import (
	"database/sql"
	"errors"

	"audioloader/internal/audio"
)

// attachCovers fills in CoverOf for all tracks with a single query. A
// library original the viewer can't see (a secret track) is left out
// entirely: even its title would give it away.
func attachCovers(v Viewer, tracks []*audio.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	byID := make(map[string]*audio.Track, len(tracks))
	args := make([]any, 0, len(tracks))
	for _, t := range tracks {
		byID[t.ID.String()] = t
		args = append(args, t.ID.String())
	}

	// the original's artists in the order they were picked, and the release
	// to link to: an album, EP or compilation before a single, earliest first
	rows, err := DB.Query(`
		SELECT t.uuid, COALESCE(t.original_title, ''), COALESCE(t.original_artists, ''), COALESCE(t.original_mbid, ''),
			COALESCE(o.uuid, ''), COALESCE(o.title, ''),
			COALESCE((SELECT group_concat(name, ', ') FROM (
				SELECT a.name FROM track_artist_map m JOIN artists a ON a.uuid = m.artist_id
				WHERE m.track_id = o.uuid ORDER BY m.rowid)), ''),
			COALESCE((SELECT rt.release_id FROM release_tracks rt JOIN releases r ON r.uuid = rt.release_id
				WHERE rt.track_id = o.uuid
				ORDER BY r.type = 'single', r.release_date IS NULL, r.release_date LIMIT 1), '')
		FROM tracks t
		LEFT JOIN tracks o ON o.uuid = t.original_track_id AND `+v.trackCond("o")+`
		WHERE t.uuid IN (`+placeholders(len(tracks))+`)
		AND (o.uuid IS NOT NULL OR t.original_title IS NOT NULL);`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var c audio.CoverRef
		var libTitle, libArtists string
		if err := rows.Scan(&id, &c.Title, &c.Artists, &c.MBID, &c.TrackID, &libTitle, &libArtists, &c.ReleaseID); err != nil {
			return err
		}
		if c.TrackID != "" {
			c.Title, c.Artists, c.MBID = libTitle, libArtists, ""
		}
		if t := byID[id]; t != nil {
			t.CoverOf = &c
		}
	}
	return rows.Err()
}

// CoverEdit sets which song a track covers, as part of a TrackEdit. Give
// either TrackID (an original in the library) or a Title with optional
// Artists and MBID (a text reference), never both. The zero value removes
// the link. The cover's own artists are never touched.
type CoverEdit struct {
	TrackID string
	Title   string
	Artists string // display text, e.g. "A & B"
	MBID    string // MusicBrainz recording ID
}

// setCover writes c onto track id. It returns ErrBadEdit for a link to
// itself or for both kinds at once, and ErrBadReference for an original
// that doesn't exist, isn't a song or that v can't see.
func setCover(tx querier, v Viewer, id string, c CoverEdit) error {
	hasText := c.Title != "" || c.Artists != "" || c.MBID != ""
	switch {
	case c.TrackID != "" && hasText:
		return ErrBadEdit
	case c.TrackID == id:
		return ErrBadEdit
	case c.TrackID != "":
		var ok bool
		err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM tracks t WHERE t.uuid = ? AND t.kind = 'music' AND `+v.trackCond("t")+`);`, c.TrackID).Scan(&ok)
		if err != nil {
			return err
		}
		if !ok {
			return ErrBadReference
		}
	case hasText && c.Title == "":
		return ErrBadEdit
	}
	_, err := tx.Exec(`UPDATE tracks SET original_track_id = ?, original_title = ?, original_artists = ?, original_mbid = ? WHERE uuid = ?;`,
		nullIfEmpty(c.TrackID), nullIfEmpty(c.Title), nullIfEmpty(c.Artists), nullIfEmpty(c.MBID), id)
	return err
}

// ErrCoverSet is returned when a suggested cover would replace an original
// that is already set.
var ErrCoverSet = errors.New("this track already has an original")

// CoverSuggestion is an original found by auto-tagging for a track that has
// none, waiting to be accepted or rejected.
type CoverSuggestion struct {
	Track   *audio.Track
	Title   string
	Artists string
	MBID    string
}

// String is "Title by Artists".
func (s CoverSuggestion) String() string {
	return (&audio.CoverRef{Title: s.Title, Artists: s.Artists}).String()
}

// SaveCoverSuggestion stores s for the track unless one exists, pending or
// not: a rejected suggestion isn't proposed again.
func SaveCoverSuggestion(trackID string, s CoverSuggestion) error {
	_, err := DB.Exec(`INSERT INTO cover_suggestions(track_id, title, artists, mbid) VALUES(?, ?, ?, ?) ON CONFLICT(track_id) DO NOTHING;`,
		trackID, s.Title, s.Artists, nullIfEmpty(s.MBID))
	return err
}

// HasCoverSuggestion reports whether the track got a suggestion before.
func HasCoverSuggestion(trackID string) (bool, error) {
	var ok bool
	err := DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM cover_suggestions WHERE track_id = ?);`, trackID).Scan(&ok)
	return ok, err
}

// PendingCoverSuggestions lists the suggestions v can see, skipping tracks
// that got an original in the meantime.
func PendingCoverSuggestions(v Viewer) ([]CoverSuggestion, error) {
	rows, err := DB.Query(`
		SELECT s.track_id, s.title, s.artists, COALESCE(s.mbid, '') FROM cover_suggestions s
		JOIN tracks t ON t.uuid = s.track_id
		WHERE s.status = 'pending' AND t.original_track_id IS NULL AND t.original_title IS NULL AND `+v.trackCond("t")+`
		ORDER BY t.title COLLATE NOCASE, t.uuid;`)
	if err != nil {
		return nil, err
	}
	type row struct {
		id string
		CoverSuggestion
	}
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.Title, &r.Artists, &r.MBID); err != nil {
			rows.Close()
			return nil, err
		}
		found = append(found, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	out := make([]CoverSuggestion, 0, len(found))
	for _, r := range found {
		t, err := GetTrack(v, r.id)
		if err != nil {
			return nil, err
		}
		r.Track = t
		out = append(out, r.CoverSuggestion)
	}
	return out, nil
}

// ApplyCoverSuggestion sets the suggested original as a text reference. It
// never replaces an original that is already set (ErrCoverSet).
func ApplyCoverSuggestion(v Viewer, trackID string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var s CoverSuggestion
	var set bool
	err = tx.QueryRow(`
		SELECT s.title, s.artists, COALESCE(s.mbid, ''), t.original_track_id IS NOT NULL OR t.original_title IS NOT NULL
		FROM cover_suggestions s JOIN tracks t ON t.uuid = s.track_id
		WHERE s.track_id = ? AND s.status = 'pending' AND `+v.trackCond("t")+`;`, trackID).Scan(&s.Title, &s.Artists, &s.MBID, &set)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if set {
		return ErrCoverSet
	}
	if err := setCover(tx, v, trackID, CoverEdit{Title: s.Title, Artists: s.Artists, MBID: s.MBID}); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE cover_suggestions SET status = 'applied' WHERE track_id = ?;`, trackID); err != nil {
		return err
	}
	return tx.Commit()
}

// RejectCoverSuggestion drops a suggestion for good.
func RejectCoverSuggestion(v Viewer, trackID string) error {
	return expectRow(DB.Exec(`
		UPDATE cover_suggestions SET status = 'rejected'
		WHERE track_id = ? AND status = 'pending'
		AND track_id IN (SELECT t.uuid FROM tracks t WHERE `+v.trackCond("t")+`);`, trackID))
}
