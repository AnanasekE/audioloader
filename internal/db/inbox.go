package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"uuid"

	"audioloader/internal/audio"
	"audioloader/internal/match"
)

// InboxRelease is a followed artist's release that isn't in the library.
type InboxRelease struct {
	ReleaseGroupMBID string
	ArtistID         string
	ArtistName       string
	ReleaseMBID      string
	Title            string
	Type             string
	Date             string
	Status           string // new, confirmed, ignored
	FoundAt          string
	Data             InboxData
}

type InboxData struct {
	CoverURL string `json:"cover_url"`
	// the album's Spotify link from MusicBrainz, if it has one
	SpotifyURL string        `json:"spotify_url,omitempty"`
	Artists    []MatchArtist `json:"artists"`
	Tracks     []InboxTrack  `json:"tracks"`
}

type InboxTrack struct {
	Disc          int           `json:"disc"`
	Position      int           `json:"position"`
	Title         string        `json:"title"`
	RecordingMBID string        `json:"recording_mbid"`
	ISRC          string        `json:"isrc,omitempty"`
	LengthMs      int           `json:"length_ms"`
	SpotifyURL    string        `json:"spotify_url,omitempty"`
	Artists       []MatchArtist `json:"artists"`
	// set when the library already has this track
	OwnedTrackID string `json:"owned_track_id,omitempty"`
}

// Owned counts the tracks already in the library.
func (r *InboxRelease) Owned() int {
	n := 0
	for _, t := range r.Data.Tracks {
		if t.OwnedTrackID != "" {
			n++
		}
	}
	return n
}

// InboxKnows reports whether a release group was already put in the inbox,
// whatever became of it: an ignored release stays ignored.
func InboxKnows(releaseGroupMBID string) (bool, error) {
	var known bool
	err := DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM inbox_releases WHERE release_group_mbid = ?);`, releaseGroupMBID).Scan(&known)
	return known, err
}

func AddInboxRelease(r InboxRelease) error {
	data, err := json.Marshal(r.Data)
	if err != nil {
		return err
	}
	_, err = DB.Exec(`
		INSERT OR IGNORE INTO inbox_releases(release_group_mbid, artist_id, release_mbid, title, type, date, data)
		VALUES(?, ?, ?, ?, ?, ?, ?);`,
		r.ReleaseGroupMBID, r.ArtistID, nullIfEmpty(r.ReleaseMBID), r.Title, r.Type, nullIfEmpty(r.Date), string(data))
	return err
}

const selectInbox = `
	SELECT i.release_group_mbid, i.artist_id, a.name, COALESCE(i.release_mbid, ''), i.title, i.type,
		COALESCE(i.date, ''), i.status, i.found_at, i.data
	FROM inbox_releases i JOIN artists a ON a.uuid = i.artist_id`

func scanInbox(s scanner) (*InboxRelease, error) {
	var r InboxRelease
	var data string
	err := s.Scan(&r.ReleaseGroupMBID, &r.ArtistID, &r.ArtistName, &r.ReleaseMBID, &r.Title, &r.Type,
		&r.Date, &r.Status, &r.FoundAt, &data)
	if err != nil {
		return nil, err
	}
	return &r, json.Unmarshal([]byte(data), &r.Data)
}

// ListInbox returns the inbox releases with the given status, newest release first.
func ListInbox(status string) ([]*InboxRelease, error) {
	rows, err := DB.Query(selectInbox+` WHERE i.status = ? ORDER BY i.date IS NULL, i.date DESC, i.title;`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*InboxRelease
	for rows.Next() {
		r, err := scanInbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func GetInboxRelease(releaseGroupMBID string) (*InboxRelease, error) {
	r, err := scanInbox(DB.QueryRow(selectInbox+` WHERE i.release_group_mbid = ?;`, releaseGroupMBID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func InboxCount() (int, error) {
	var n int
	err := DB.QueryRow(`SELECT COUNT(*) FROM inbox_releases WHERE status = 'new';`).Scan(&n)
	return n, err
}

// IgnoreInboxRelease hides a release for good ("don't ask again").
func IgnoreInboxRelease(releaseGroupMBID string) error {
	res, err := DB.Exec(`UPDATE inbox_releases SET status = 'ignored' WHERE release_group_mbid = ? AND status = 'new';`, releaseGroupMBID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RestoreInboxRelease moves an ignored release back to the new ones.
func RestoreInboxRelease(releaseGroupMBID string) error {
	res, err := DB.Exec(`UPDATE inbox_releases SET status = 'new' WHERE release_group_mbid = ? AND status = 'ignored';`, releaseGroupMBID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// InboxCounts returns how many releases are new and how many are ignored.
func InboxCounts() (newCount, ignored int, err error) {
	err = DB.QueryRow(`
		SELECT COALESCE(SUM(status = 'new'), 0), COALESCE(SUM(status = 'ignored'), 0) FROM inbox_releases;`).Scan(&newCount, &ignored)
	return
}

// ConfirmInboxRelease creates the release with its artists and tracklist.
// Tracks the library has are linked; the others become placeholder tracks
// with a queued download each. It returns the new release's ID and the
// placeholders' IDs.
func ConfirmInboxRelease(releaseGroupMBID string) (releaseID string, placeholders []string, err error) {
	ir, err := GetInboxRelease(releaseGroupMBID)
	if err != nil {
		return "", nil, err
	}
	if ir.Status != "new" {
		return "", nil, ErrNotFound
	}
	typ := audio.ReleaseType(ir.Type)
	if !typ.Valid() {
		typ = audio.ReleaseAlbum
	}

	tx, err := DB.Begin()
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()

	rel := &audio.Release{ID: uuid.New(), Title: ir.Title, Type: typ, Date: ir.Date}
	var artistIDs []string
	for _, a := range ir.Data.Artists {
		id, err := resolveArtistTx(tx, a)
		if err != nil {
			return "", nil, err
		}
		artistIDs = appendUnique(artistIDs, id)
	}
	if len(artistIDs) == 0 {
		artistIDs = []string{ir.ArtistID}
	}
	if err := insertRelease(tx, rel, artistIDs); err != nil {
		return "", nil, err
	}
	// the release MBID is unique; another release may have it already
	var taken bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM releases WHERE mbid = ?);`, ir.ReleaseMBID).Scan(&taken); err != nil {
		return "", nil, err
	}
	var mbid any
	if !taken {
		mbid = nullIfEmpty(ir.ReleaseMBID)
	}
	if _, err := tx.Exec(`UPDATE releases SET mbid = ?, release_group_mbid = ?, spotify_url = ? WHERE uuid = ?;`,
		mbid, ir.ReleaseGroupMBID, nullIfEmpty(ir.Data.SpotifyURL), rel.ID.String()); err != nil {
		return "", nil, err
	}

	linked := map[string]bool{}
	for _, t := range ir.Data.Tracks {
		link := ReleaseLink{ReleaseID: rel.ID.String(), Disc: max(t.Disc, 1), Position: t.Position}
		if t.OwnedTrackID != "" && !linked[t.OwnedTrackID] {
			// the owned track may have been deleted since the check
			var exists bool
			if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM tracks WHERE uuid = ? AND has_file = 1);`, t.OwnedTrackID).Scan(&exists); err != nil {
				return "", nil, err
			}
			if exists {
				if err := insertReleaseTrack(tx, t.OwnedTrackID, link); err != nil {
					return "", nil, err
				}
				linked[t.OwnedTrackID] = true
				continue
			}
		}

		id := uuid.New().String()
		_, err := tx.Exec(`
			INSERT INTO tracks(uuid, title, duration_seconds, has_file, mbid, isrc, spotify_url, tag_status)
			VALUES(?, ?, ?, 0, ?, ?, ?, 'matched');`,
			id, t.Title, t.LengthMs/1000, nullIfEmpty(t.RecordingMBID), nullIfEmpty(t.ISRC), nullIfEmpty(t.SpotifyURL))
		if err != nil {
			return "", nil, err
		}
		credits := t.Artists
		if len(credits) == 0 {
			credits = ir.Data.Artists
		}
		var trackArtists []string
		for _, a := range credits {
			aid, err := resolveArtistTx(tx, a)
			if err != nil {
				return "", nil, err
			}
			trackArtists = appendUnique(trackArtists, aid)
		}
		if len(trackArtists) == 0 {
			trackArtists = artistIDs
		}
		for _, aid := range trackArtists {
			if _, err := tx.Exec(`INSERT INTO track_artist_map(track_id, artist_id) VALUES(?, ?);`, id, aid); err != nil {
				return "", nil, err
			}
		}
		if err := insertReleaseTrack(tx, id, link); err != nil {
			return "", nil, err
		}
		if _, err := tx.Exec(`INSERT INTO downloads(track_id) VALUES(?);`, id); err != nil {
			return "", nil, err
		}
		placeholders = append(placeholders, id)
	}

	if _, err := tx.Exec(`UPDATE inbox_releases SET status = 'confirmed' WHERE release_group_mbid = ?;`, releaseGroupMBID); err != nil {
		return "", nil, err
	}
	return rel.ID.String(), placeholders, tx.Commit()
}

// resolveArtistTx finds the artist by MusicBrainz ID, else by name (giving
// it the ID), else creates it.
func resolveArtistTx(tx *sql.Tx, a MatchArtist) (string, error) {
	var id string
	if a.ID != "" {
		err := tx.QueryRow(`SELECT uuid FROM artists WHERE mbid = ?;`, a.ID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		return "", errors.New("artist without a name")
	}
	var mbid sql.NullString
	err := tx.QueryRow(`SELECT uuid, mbid FROM artists WHERE lower(name) = lower(?);`, name).Scan(&id, &mbid)
	if err == nil {
		if !mbid.Valid && a.ID != "" {
			if _, err := tx.Exec(`UPDATE artists SET mbid = ? WHERE uuid = ?;`, a.ID, id); err != nil {
				return "", err
			}
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id = uuid.New().String()
	_, err = tx.Exec(`INSERT INTO artists(uuid, name, mbid) VALUES(?, ?, ?);`, id, name, nullIfEmpty(a.ID))
	return id, err
}

func appendUnique(ids []string, id string) []string {
	for _, x := range ids {
		if x == id {
			return ids
		}
	}
	return append(ids, id)
}

// LibraryReleaseFor returns the ID of a release in the library that stands
// for a MusicBrainz release group: one with its ID, else one by the artist
// with the same title (which then gets the ID). "" if there is none.
func LibraryReleaseFor(artistID, releaseGroupMBID, title string) (string, error) {
	var id string
	err := DB.QueryRow(`SELECT uuid FROM releases WHERE release_group_mbid = ? LIMIT 1;`, releaseGroupMBID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	rows, err := DB.Query(`
		SELECT r.uuid, r.title FROM releases r
		JOIN release_artist_map m ON m.release_id = r.uuid
		WHERE m.artist_id = ? AND r.release_group_mbid IS NULL;`, artistID)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var rid, rtitle string
		if err := rows.Scan(&rid, &rtitle); err != nil {
			rows.Close()
			return "", err
		}
		if match.Same(rtitle, title) {
			id = rid
			break
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil || id == "" {
		return "", err
	}
	_, err = DB.Exec(`UPDATE releases SET release_group_mbid = ? WHERE uuid = ?;`, releaseGroupMBID, id)
	return id, err
}

// OwnedTrack finds a library track (with a file) for a tracklist entry: by
// recording MBID, then ISRC, then title and artist with a length within 3 s.
// library is ListTracks(), passed in so a whole release is matched against
// one read of the library.
func OwnedTrack(library []*audio.Track, t InboxTrack) string {
	for _, lt := range library {
		if t.RecordingMBID != "" && lt.MBID == t.RecordingMBID {
			return lt.ID.String()
		}
	}
	for _, lt := range library {
		if t.ISRC != "" && lt.ISRC == t.ISRC {
			return lt.ID.String()
		}
	}
	for _, lt := range library {
		if !match.Same(lt.Title, t.Title) {
			continue
		}
		if t.LengthMs > 0 && lt.Duration > 0 {
			diff := lt.Duration - time.Duration(t.LengthMs)*time.Millisecond
			if diff < -3*time.Second || diff > 3*time.Second {
				continue
			}
		}
		for _, a := range lt.Artists {
			for _, c := range t.Artists {
				if (c.ID != "" && a.MBID == c.ID) || match.Same(a.Name, c.Name) {
					return lt.ID.String()
				}
			}
		}
	}
	return ""
}

// MergePlaceholder moves a placeholder's release placements onto an
// existing track (one with the same file), then deletes the placeholder.
func MergePlaceholder(placeholderID, trackID string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// a release that already has the track keeps that placement
	_, err = tx.Exec(`
		DELETE FROM release_tracks WHERE track_id = ?
		AND release_id IN (SELECT release_id FROM release_tracks WHERE track_id = ?);`, placeholderID, trackID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE release_tracks SET track_id = ? WHERE track_id = ?;`, trackID, placeholderID); err != nil {
		return err
	}
	_, err = tx.Exec(`
		UPDATE tracks SET
			mbid = COALESCE(mbid, (SELECT mbid FROM tracks WHERE uuid = ?)),
			isrc = COALESCE(isrc, (SELECT isrc FROM tracks WHERE uuid = ?))
		WHERE uuid = ?;`, placeholderID, placeholderID, trackID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tracks WHERE uuid = ? AND has_file = 0;`, placeholderID); err != nil {
		return err
	}
	return tx.Commit()
}
