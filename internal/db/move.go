package db

import (
	"database/sql"
	"errors"
	"uuid"

	"audioloader/internal/audio"
)

// MovedTrack is what MoveTrackToRelease did, for the caller's follow-up work.
type MovedTrack struct {
	ReleaseID  string
	NewRelease bool // created for the move: fetch its cover
	OldRemoved bool // the release the track left was empty and import-made
}

// MoveTrackToRelease takes a track off the release fromID and puts it on the
// library's release with target's MBID, creating that release from target
// when the library has none. Only the track moves: the release's other
// tracks aren't added, the release page shows them as missing. If the
// release has a placeholder for the track's recording, the track takes the
// placeholder's place and the placeholder goes; else target's disc and
// position are used (the next free one if that is taken). The release the
// track left is deleted when it has no tracks left and no MusicBrainz ID:
// the single made at import.
func MoveTrackToRelease(trackID, fromID string, target NewSongRelease) (MovedTrack, error) {
	var out MovedTrack
	tx, err := DB.Begin()
	if err != nil {
		return out, err
	}
	defer tx.Rollback()

	var recording sql.NullString
	err = tx.QueryRow(`SELECT mbid FROM tracks WHERE uuid = ?;`, trackID).Scan(&recording)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}

	// a release with the target's MusicBrainz ID is reused
	err = tx.QueryRow(`SELECT uuid FROM releases WHERE mbid = ?;`, target.MBID).Scan(&out.ReleaseID)
	if errors.Is(err, sql.ErrNoRows) {
		artistIDs, err := resolveArtistsTx(tx, target.Artists)
		if err != nil {
			return out, err
		}
		if len(artistIDs) == 0 { // the track's own artists
			rows, err := tx.Query(`SELECT artist_id FROM track_artist_map WHERE track_id = ?;`, trackID)
			if err != nil {
				return out, err
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return out, err
				}
				artistIDs = append(artistIDs, id)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return out, err
			}
		}
		typ := audio.ReleaseType(target.Type)
		if !typ.Valid() {
			typ = audio.ReleaseSingle
		}
		rel := &audio.Release{ID: uuid.New(), Title: target.Title, Type: typ, Date: target.Date}
		if err := insertRelease(tx, rel, artistIDs); err != nil {
			return out, err
		}
		if _, err := tx.Exec(`UPDATE releases SET mbid = ?, release_group_mbid = ?, spotify_url = ? WHERE uuid = ?;`,
			nullIfEmpty(target.MBID), nullIfEmpty(target.ReleaseGroupMBID), nullIfEmpty(target.SpotifyURL), rel.ID.String()); err != nil {
			return out, err
		}
		out.ReleaseID, out.NewRelease = rel.ID.String(), true
	} else if err != nil {
		return out, err
	}

	disc, position := max(target.Disc, 1), target.Position
	if recording.Valid && !out.NewRelease {
		var placeholder string
		var pd, pp int
		err := tx.QueryRow(`
			SELECT t.uuid, rt.disc, rt.position FROM release_tracks rt JOIN tracks t ON t.uuid = rt.track_id
			WHERE rt.release_id = ? AND t.has_file = 0 AND t.mbid = ? LIMIT 1;`, out.ReleaseID, recording.String).Scan(&placeholder, &pd, &pp)
		if err == nil {
			disc, position = pd, pp
			// plays and playlists of the placeholder carry over to the track
			if _, err := tx.Exec(`UPDATE playlist_tracks SET track_id = ? WHERE track_id = ?;`, trackID, placeholder); err != nil {
				return out, err
			}
			if _, err := tx.Exec(`DELETE FROM tracks WHERE uuid = ?;`, placeholder); err != nil {
				return out, err
			}
			if err := dropQueuedJobsTx(tx, placeholder); err != nil {
				return out, err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
	}

	if _, err := tx.Exec(`DELETE FROM release_tracks WHERE track_id = ? AND release_id = ?;`, trackID, fromID); err != nil {
		return out, err
	}
	link := ReleaseLink{ReleaseID: out.ReleaseID, Disc: disc, Position: position}
	err = nil
	if position > 0 {
		err = insertReleaseTrack(tx, trackID, link)
	}
	if position <= 0 || errors.Is(err, ErrSlotTaken) {
		if err := tx.QueryRow(`SELECT COALESCE(MAX(position), 0) + 1 FROM release_tracks WHERE release_id = ? AND disc = ?;`,
			out.ReleaseID, disc).Scan(&link.Position); err != nil {
			return out, err
		}
		err = insertReleaseTrack(tx, trackID, link)
	}
	if err != nil && !errors.Is(err, ErrAlreadyOnRelease) {
		return out, err
	}

	res, err := tx.Exec(`
		DELETE FROM releases WHERE uuid = ? AND uuid <> ? AND mbid IS NULL
		AND NOT EXISTS (SELECT 1 FROM release_tracks WHERE release_id = releases.uuid);`, fromID, out.ReleaseID)
	if err != nil {
		return out, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		out.OldRemoved = true
		if err := dropQueuedJobsTx(tx, fromID); err != nil {
			return out, err
		}
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	if out.OldRemoved {
		if rid, err := uuid.Parse(fromID); err == nil {
			removeCover(rid)
		}
	}
	return out, nil
}
