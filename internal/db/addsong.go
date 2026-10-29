package db

import (
	"database/sql"
	"errors"
	"uuid"

	"audioloader/internal/audio"
)

// NewSong is a MusicBrainz recording to add as a placeholder track.
type NewSong struct {
	RecordingMBID string
	Title         string
	ISRC          string
	LengthMs      int
	SpotifyURL    string
	Artists       []MatchArtist
	Release       *NewSongRelease // nil: the track becomes its own single
}

type NewSongRelease struct {
	MBID, ReleaseGroupMBID string
	Title, Type, Date      string // Type is ours: album, ep, single, compilation
	SpotifyURL             string
	Artists                []MatchArtist
	Disc, Position         int
}

// AddSong creates the placeholder with its artists (resolved by MBID, then
// name, else created) on its release (reused or created), and a queued
// downloads row. It returns the track's and the release's IDs, and whether
// the release is new (to fetch its cover).
func AddSong(s NewSong) (trackID, releaseID string, newRelease bool, err error) {
	tx, err := DB.Begin()
	if err != nil {
		return "", "", false, err
	}
	defer tx.Rollback()

	artistIDs, err := resolveArtistsTx(tx, s.Artists)
	if err != nil {
		return "", "", false, err
	}
	if len(artistIDs) == 0 {
		return "", "", false, errors.New("song without an artist")
	}

	t := InboxTrack{
		Title: s.Title, RecordingMBID: s.RecordingMBID, ISRC: s.ISRC,
		LengthMs: s.LengthMs, SpotifyURL: s.SpotifyURL, Artists: s.Artists,
	}
	disc, position := 1, 1
	if r := s.Release; r == nil {
		single := &audio.Release{ID: uuid.New(), Title: s.Title, Type: audio.ReleaseSingle}
		if err := insertRelease(tx, single, artistIDs); err != nil {
			return "", "", false, err
		}
		releaseID, newRelease = single.ID.String(), true
	} else {
		disc, position = max(r.Disc, 1), r.Position
		releaseArtists, err := resolveArtistsTx(tx, r.Artists)
		if err != nil {
			return "", "", false, err
		}
		if len(releaseArtists) == 0 {
			releaseArtists = artistIDs
		}
		releaseID, err = libraryReleaseIDTx(tx, releaseArtists[0], r)
		if err != nil {
			return "", "", false, err
		}
		if releaseID == "" {
			typ := audio.ReleaseType(r.Type)
			if !typ.Valid() {
				typ = audio.ReleaseSingle
			}
			rel := &audio.Release{ID: uuid.New(), Title: r.Title, Type: typ, Date: r.Date}
			if err := insertRelease(tx, rel, releaseArtists); err != nil {
				return "", "", false, err
			}
			// the release MBID is unique; another release may have it already
			var taken bool
			if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM releases WHERE mbid = ?);`, r.MBID).Scan(&taken); err != nil {
				return "", "", false, err
			}
			var mbid any
			if !taken {
				mbid = nullIfEmpty(r.MBID)
			}
			if _, err := tx.Exec(`UPDATE releases SET mbid = ?, release_group_mbid = ?, spotify_url = ? WHERE uuid = ?;`,
				mbid, nullIfEmpty(r.ReleaseGroupMBID), nullIfEmpty(r.SpotifyURL), rel.ID.String()); err != nil {
				return "", "", false, err
			}
			releaseID, newRelease = rel.ID.String(), true
		}
	}

	trackID, err = insertPlaceholderTx(tx, t, artistIDs)
	if err != nil {
		return "", "", false, err
	}
	link := ReleaseLink{ReleaseID: releaseID, Disc: disc, Position: position}
	err = nil
	if position > 0 {
		err = insertReleaseTrack(tx, trackID, link)
	}
	if position <= 0 || errors.Is(err, ErrSlotTaken) {
		// the slot is taken (or unknown): next free position on the disc
		if err := tx.QueryRow(`SELECT COALESCE(MAX(position), 0) + 1 FROM release_tracks WHERE release_id = ? AND disc = ?;`,
			releaseID, disc).Scan(&link.Position); err != nil {
			return "", "", false, err
		}
		err = insertReleaseTrack(tx, trackID, link)
	}
	if err != nil {
		return "", "", false, err
	}
	return trackID, releaseID, newRelease, tx.Commit()
}

func resolveArtistsTx(tx *sql.Tx, artists []MatchArtist) ([]string, error) {
	var ids []string
	for _, a := range artists {
		id, err := resolveArtistTx(tx, a)
		if err != nil {
			return nil, err
		}
		ids = appendUnique(ids, id)
	}
	return ids, nil
}

// libraryReleaseIDTx finds the library release for r: by release MBID, then
// release group MBID, then the same title by the artist. "" if there is none.
func libraryReleaseIDTx(tx *sql.Tx, artistID string, r *NewSongRelease) (string, error) {
	var id string
	if r.MBID != "" {
		err := tx.QueryRow(`SELECT uuid FROM releases WHERE mbid = ?;`, r.MBID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	if r.ReleaseGroupMBID == "" {
		return "", nil
	}
	return libraryReleaseForTx(tx, artistID, r.ReleaseGroupMBID, r.Title)
}

// QueuedSong returns the placeholder for a recording that was added or
// confirmed but isn't downloaded yet, and its release. "" if there is none.
func QueuedSong(recordingMBID string) (trackID, releaseID string, err error) {
	err = DB.QueryRow(`
		SELECT t.uuid, COALESCE((SELECT release_id FROM release_tracks WHERE track_id = t.uuid LIMIT 1), '')
		FROM tracks t WHERE t.mbid = ? AND t.has_file = 0 AND t.kind = 'music' LIMIT 1;`, recordingMBID).Scan(&trackID, &releaseID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return trackID, releaseID, err
}
