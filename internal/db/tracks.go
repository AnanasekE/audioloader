package db

import (
	"database/sql"
	"errors"
	"io"
	"os"
	"time"
	"uuid"

	"audioloader/internal/audio"
)

// ErrNotFound is returned when a looked-up row or file doesn't exist.
var ErrNotFound = errors.New("not found")

const selectTrack = `SELECT t.uuid, t.title, t.duration_seconds, ` + trackImgURL + `, t.has_file, COALESCE(t.mbid, ''), COALESCE(t.isrc, ''),
	t.kind, COALESCE(t.show_id, ''), COALESCE(t.published, ''), t.secret, COALESCE(t.description, '') FROM tracks t`

// libraryTracks is the condition every listing of the music library uses,
// on a tracks table aliased t. Placeholders have no file to play, so they
// only show up on their releases; podcast episodes are listed by show.
const libraryTracks = `t.has_file = 1 AND t.kind = 'music'`

// scanner is implemented by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanTrack(s scanner) (*audio.Track, error) {
	var t audio.Track
	var seconds int
	if err := s.Scan(&t.ID, &t.Title, &seconds, &t.ImgURL, &t.HasFile, &t.MBID, &t.ISRC, &t.Kind, &t.ShowID, &t.Published, &t.Secret, &t.Description); err != nil {
		return nil, err
	}
	t.Duration = time.Duration(seconds) * time.Second
	return &t, nil
}

// ListTracks returns the library's tracks. In secret mode they come newest
// first: that is the "date added" order of its track list.
func ListTracks(v Viewer) ([]*audio.Track, error) {
	order := ""
	if v.IsSecret() {
		order = ` ORDER BY t.rowid DESC`
	}
	rows, err := DB.Query(selectTrack + ` WHERE ` + libraryTracks + ` AND ` + v.trackCond("t") + order + `;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tracks := make([]*audio.Track, 0)
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	if err := attachArtists(tracks); err != nil {
		return nil, err
	}
	if err := attachReleases(tracks); err != nil {
		return nil, err
	}
	if err := attachCovers(v, tracks); err != nil {
		return nil, err
	}
	if err := attachTags(tracks); err != nil {
		return nil, err
	}
	return tracks, nil
}

func GetTrack(v Viewer, id string) (*audio.Track, error) {
	t, err := scanTrack(DB.QueryRow(selectTrack+` WHERE t.uuid = ? AND `+v.trackCond("t")+`;`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := attachArtists([]*audio.Track{t}); err != nil {
		return nil, err
	}
	if err := attachReleases([]*audio.Track{t}); err != nil {
		return nil, err
	}
	if err := attachCovers(v, []*audio.Track{t}); err != nil {
		return nil, err
	}
	if err := attachTags([]*audio.Track{t}); err != nil {
		return nil, err
	}
	return t, nil
}

// attachArtists fills in Artists for all tracks with a single query,
// instead of one query per track.
func attachArtists(tracks []*audio.Track) error {
	if len(tracks) == 0 {
		return nil
	}

	byID := make(map[string]*audio.Track, len(tracks))
	args := make([]any, 0, len(tracks))
	for _, t := range tracks {
		byID[t.ID.String()] = t
		args = append(args, t.ID.String())
	}
	// ordered by rowid so artists show in the order they were picked
	rows, err := DB.Query(`
		SELECT m.track_id, a.uuid, a.name, ` + artistImgURL + `, COALESCE(a.mbid, '')
		FROM track_artist_map m
		JOIN artists a ON a.uuid = m.artist_id
		WHERE m.track_id IN (`+placeholders(len(tracks))+`)
		ORDER BY m.rowid;`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var trackID string
		var a audio.Artist
		if err := rows.Scan(&trackID, &a.ID, &a.Name, &a.ImgURL, &a.MBID); err != nil {
			return err
		}
		if t := byID[trackID]; t != nil {
			t.Artists = append(t.Artists, &a)
		}
	}
	return rows.Err()
}

// AddResult reports what AddTrack did.
type AddResult struct {
	TrackID string
	// Linked is true when the file was already stored and the existing
	// track was only placed on the release.
	Linked bool
}

// AddTrack saves the audio file and inserts the track linked to artistIDs.
// With a link the track is placed on that release; without one it becomes
// its own single, released on singleDate (may be empty).
// If the file is already stored it returns ErrExists, unless link is set:
// then the existing track is placed on the release instead, since a song
// often comes out as a single before the album.
// If the insert fails, the saved file is removed again.
// A secret mode upload creates a secret track and no release, whatever link
// says. For a viewer who can't see it, a file stored on the other side gives
// ErrHidden: a generic failure that doesn't reveal the track exists.
func AddTrack(v Viewer, artistIDs []string, track *audio.Track, file io.ReadSeeker, ext string, link *ReleaseLink, singleDate string) (AddResult, error) {
	fp, err := FingerprintOf(file)
	if err != nil {
		return AddResult{}, err
	}
	// fast path that skips writing the file; the unique index still
	// catches two identical uploads racing each other
	exists, err := TrackExists(All, fp)
	if err != nil {
		return AddResult{}, err
	}
	if exists {
		return linkOrExists(v, fp, link)
	}

	id := uuid.New().String()
	path, err := saveTrackFile(id, file, ext)
	if err != nil {
		return AddResult{}, err
	}
	if err := insertTrack(id, v.IsSecret(), artistIDs, track, fp, link, singleDate); err != nil {
		os.Remove(path)
		// insertReleaseTrack maps its own constraint errors, so a remaining
		// unique violation is the fingerprint index: an identical upload won the race
		if isUniqueViolation(err) {
			return linkOrExists(v, fp, link)
		}
		if isForeignKeyViolation(err) {
			return AddResult{}, ErrBadReference
		}
		return AddResult{}, err
	}
	return AddResult{TrackID: id}, nil
}

// ErrHidden is a deliberately vague failure for an action on a secret track
// by a viewer who can't see it.
var ErrHidden = errors.New("upload failed")

func linkOrExists(v Viewer, fp Fingerprint, link *ReleaseLink) (AddResult, error) {
	if hidden, err := hiddenFingerprint(v, fp); err != nil || hidden {
		if err == nil {
			err = ErrHidden
		}
		return AddResult{}, err
	}
	if link == nil {
		return AddResult{}, ErrExists
	}
	id, err := linkExistingTrack(fp, *link)
	if err != nil {
		return AddResult{}, err
	}
	return AddResult{TrackID: id, Linked: true}, nil
}

// insertTrack inserts the track row, its artist mappings and its release
// placement (the given release, or a new single) in one transaction.
func insertTrack(id string, secret bool, artistIDs []string, track *audio.Track, fp Fingerprint, link *ReleaseLink, singleDate string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	// no-op after a successful Commit
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO tracks(uuid, title, duration_seconds, file_size, head_hash, mbid, isrc, secret, description) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		id, track.Title, int(track.Duration.Seconds()), fp.Size, fp.HeadHash, nullIfEmpty(track.MBID), nullIfEmpty(track.ISRC), secret, nullIfEmpty(track.Description),
	)
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO track_artist_map(track_id, artist_id) VALUES(?, ?);`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, artistID := range artistIDs {
		if _, err := stmt.Exec(id, artistID); err != nil {
			return err
		}
	}
	if secret {
		// secret tracks are on no release
		if err := setTags(tx, id, track.Tags); err != nil {
			return err
		}
		return tx.Commit()
	}
	if link == nil {
		single := &audio.Release{ID: uuid.New(), Title: track.Title, Type: audio.ReleaseSingle, Date: singleDate}
		if err := insertRelease(tx, single, artistIDs); err != nil {
			return err
		}
		link = &ReleaseLink{ReleaseID: single.ID.String(), Disc: 1, Position: 1}
	}
	if err := insertReleaseTrack(tx, id, *link); err != nil {
		return err
	}
	return tx.Commit()
}

// ErrHasFile is returned when a file is attached to a track that already has one.
var ErrHasFile = errors.New("track already has a file")

// AttachFile stores file for the placeholder track id, turning it into a
// normal, playable track. If the file is already stored under another
// track it returns ErrExists and that track's ID, so the caller can merge
// the placeholder into it.
func AttachFile(id string, file io.ReadSeeker, ext string, duration time.Duration) (existingID string, err error) {
	var hasFile bool
	err = DB.QueryRow(`SELECT has_file FROM tracks WHERE uuid = ?;`, id).Scan(&hasFile)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if hasFile {
		return "", ErrHasFile
	}

	fp, err := FingerprintOf(file)
	if err != nil {
		return "", err
	}
	if existing, err := trackByFingerprint(fp); err != nil || existing != "" {
		if err == nil {
			err = ErrExists
		}
		return existing, err
	}

	path, err := saveTrackFile(id, file, ext)
	if err != nil {
		return "", err
	}
	_, err = DB.Exec(
		`UPDATE tracks SET has_file = 1, file_size = ?, head_hash = ?, duration_seconds = ? WHERE uuid = ?;`,
		fp.Size, fp.HeadHash, int(duration.Round(time.Second).Seconds()), id,
	)
	if err != nil {
		os.Remove(path)
		// the same file was attached elsewhere in the meantime
		if isUniqueViolation(err) {
			existing, lerr := trackByFingerprint(fp)
			if lerr != nil {
				return "", lerr
			}
			return existing, ErrExists
		}
		return "", err
	}
	return "", nil
}

// SetTrimmedLength stores a track's new length after its file was cut,
// and moves saved positions past the end back to it.
func SetTrimmedLength(id string, d time.Duration) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	secs := int(d.Round(time.Second).Seconds())
	res, err := tx.Exec(`UPDATE tracks SET duration_seconds = ? WHERE uuid = ? AND has_file = 1;`, secs, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(`UPDATE playback_positions SET seconds = MIN(seconds, ?) WHERE track_id = ?;`, secs, id); err != nil {
		return err
	}
	return tx.Commit()
}

// trackByFingerprint returns the ID of the track stored with fp, or "".
func trackByFingerprint(fp Fingerprint) (string, error) {
	var id string
	err := DB.QueryRow(`SELECT uuid FROM tracks WHERE file_size = ? AND head_hash = ?;`, fp.Size, fp.HeadHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// nullIfEmpty stores "" as NULL, for optional columns.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// hiddenFingerprint reports whether the file with fp is a track on the other
// side, which v can't see.
func hiddenFingerprint(v Viewer, fp Fingerprint) (bool, error) {
	if v.SeesAll() {
		return false, nil
	}
	var secret bool
	err := DB.QueryRow(`SELECT secret FROM tracks WHERE file_size = ? AND head_hash = ?;`, fp.Size, fp.HeadHash).Scan(&secret)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return secret != v.IsSecret(), err
}

// ErrBadEdit is returned for a track edit that isn't allowed, e.g. turning a
// song into an episode (which needs a show).
var ErrBadEdit = errors.New("that change isn't allowed")

// TrackEdit is what the edit form changes. It never touches the file, the
// MusicBrainz IDs or the releases.
type TrackEdit struct {
	Title     string
	Kind      string // KindMusic or KindEpisode; an episode can become a song
	ArtistIDs []string
	// Secret moves the track to the other mode (see SetSecret); nil leaves it
	Secret *bool
	// Cover sets or removes the original song; nil leaves it as it is
	Cover *CoverEdit
	// Description and Tags are for secret tracks; nil leaves them as they are
	Description *string
	Tags        *[]string
}

// UpdateTrack saves e in one transaction: the track's fields, then its
// artists, replaced in the order given. The search index is marked stale by
// the triggers and rebuilt on the next search.
func UpdateTrack(v Viewer, id string, e TrackEdit) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var kind string
	err = tx.QueryRow(`SELECT t.kind FROM tracks t WHERE t.uuid = ? AND `+v.trackCond("t")+`;`, id).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if e.Kind != kind && !(kind == audio.KindEpisode && e.Kind == audio.KindMusic) {
		return ErrBadEdit
	}

	if _, err := tx.Exec(`UPDATE tracks SET title = ?, kind = ? WHERE uuid = ?;`, e.Title, e.Kind, id); err != nil {
		return err
	}
	if e.Kind != kind {
		// a song belongs to no show
		if _, err := tx.Exec(`UPDATE tracks SET show_id = NULL, published = NULL WHERE uuid = ?;`, id); err != nil {
			return err
		}
	}
	if e.Secret != nil && !v.SeesAll() {
		if err := setSecret(tx, id, *e.Secret); err != nil {
			return err
		}
	}
	if e.Cover != nil {
		if err := setCover(tx, v, id, *e.Cover); err != nil {
			return err
		}
	}
	if e.Description != nil {
		if _, err := tx.Exec(`UPDATE tracks SET description = ? WHERE uuid = ?;`, nullIfEmpty(*e.Description), id); err != nil {
			return err
		}
	}
	if e.Tags != nil {
		if err := setTags(tx, id, *e.Tags); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM track_artist_map WHERE track_id = ?;`, id); err != nil {
		return err
	}
	for _, artistID := range e.ArtistIDs {
		_, err := tx.Exec(`INSERT INTO track_artist_map(track_id, artist_id) VALUES(?, ?);`, id, artistID)
		if isForeignKeyViolation(err) {
			return ErrBadReference
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteTrack removes a track, its audio file and everything that points at
// it. The foreign keys cascade to its releases' slots, playlist entries,
// playback position, chapters, download and tagging rows; the other tracks
// on its releases keep their numbers. Queued jobs for it are dropped too.
// The file goes after the commit, so a failed delete never loses audio.
func DeleteTrack(v Viewer, id string) error {
	tid, err := uuid.Parse(id)
	if err != nil {
		return ErrNotFound
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`DELETE FROM tracks WHERE uuid = ? AND `+v.trackCond("tracks")+`;`, id)
	if err := expectRow(res, err); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM jobs WHERE status = 'queued' AND instr(payload, ?) > 0;`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// the track is gone either way; a leftover file is only wasted space
	if path, err := TrackFilePath(tid); err == nil {
		os.Remove(path)
	}
	removeTrackImages(tid)
	return nil
}
