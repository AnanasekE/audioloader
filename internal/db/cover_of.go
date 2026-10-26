package db

import (
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
