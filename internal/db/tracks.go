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

const selectTrack = `SELECT t.uuid, t.title, t.duration_seconds, t.img_url, t.has_file, COALESCE(t.mbid, ''), COALESCE(t.isrc, ''),
	t.kind, COALESCE(t.show_id, ''), COALESCE(t.published, ''), t.secret FROM tracks t`

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
	if err := s.Scan(&t.ID, &t.Title, &seconds, &t.ImgURL, &t.HasFile, &t.MBID, &t.ISRC, &t.Kind, &t.ShowID, &t.Published, &t.Secret); err != nil {
		return nil, err
	}
	t.Duration = time.Duration(seconds) * time.Second
	return &t, nil
}

func ListTracks(v Viewer) ([]*audio.Track, error) {
	rows, err := DB.Query(selectTrack + ` WHERE ` + libraryTracks + ` AND ` + v.trackCond("t") + `;`)
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
		SELECT m.track_id, a.uuid, a.name, a.img_url, COALESCE(a.mbid, '')
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
// For a viewer who can't see it, a file stored as a secret track gives
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
	if err := insertTrack(id, artistIDs, track, fp, link, singleDate); err != nil {
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
func insertTrack(id string, artistIDs []string, track *audio.Track, fp Fingerprint, link *ReleaseLink, singleDate string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	// no-op after a successful Commit
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO tracks(uuid, title, duration_seconds, file_size, head_hash, mbid, isrc) VALUES(?, ?, ?, ?, ?, ?, ?);`,
		id, track.Title, int(track.Duration.Seconds()), fp.Size, fp.HeadHash, nullIfEmpty(track.MBID), nullIfEmpty(track.ISRC),
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

// hiddenFingerprint reports whether the file with fp is a secret track v can't see.
func hiddenFingerprint(v Viewer, fp Fingerprint) (bool, error) {
	if v.Secret {
		return false, nil
	}
	var secret bool
	err := DB.QueryRow(`SELECT secret FROM tracks WHERE file_size = ? AND head_hash = ?;`, fp.Size, fp.HeadHash).Scan(&secret)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return secret, err
}
