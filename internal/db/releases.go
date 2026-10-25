package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"uuid"

	sqlite "github.com/glebarez/go-sqlite"

	"audioloader/internal/audio"
)

var (
	// ErrBadReference is returned when a referenced artist, track or release doesn't exist.
	ErrBadReference = errors.New("referenced artist, track or release doesn't exist")
	// ErrSlotTaken is returned when a release already has a track at that disc and position.
	ErrSlotTaken = errors.New("that disc and position is already taken on this release")
	// ErrAlreadyOnRelease is returned when the track is already part of the release.
	ErrAlreadyOnRelease = errors.New("track is already on this release")
)

// ReleaseLink places a track on a release.
type ReleaseLink struct {
	ReleaseID string
	Disc      int
	Position  int
}

// ReleaseTrackInput is one entry of a release's tracklist, in order.
// Positions are assigned from the order within each disc.
type ReleaseTrackInput struct {
	TrackID string
	Disc    int
}

const selectRelease = `SELECT uuid, title, type, COALESCE(release_date, ''), img_url, COALESCE(mbid, ''), COALESCE(release_group_mbid, '') FROM releases`

func scanRelease(s scanner) (*audio.Release, error) {
	var r audio.Release
	if err := s.Scan(&r.ID, &r.Title, &r.Type, &r.Date, &r.ImgURL, &r.MBID, &r.ReleaseGroupMBID); err != nil {
		return nil, err
	}
	return &r, nil
}

func isForeignKeyViolation(err error) bool {
	var e *sqlite.Error
	// 787 = SQLITE_CONSTRAINT_FOREIGNKEY
	return errors.As(err, &e) && e.Code() == 787
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// ListReleases returns all releases with their artists, newest first;
// releases without a date come last.
func ListReleases() ([]*audio.Release, error) {
	rows, err := DB.Query(selectRelease + ` ORDER BY release_date IS NULL, release_date DESC, title;`)
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

// GetRelease returns a release with its artists and full tracklist.
func GetRelease(id string) (*audio.Release, error) {
	r, err := scanRelease(DB.QueryRow(selectRelease+` WHERE uuid = ?;`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := attachReleaseArtists([]*audio.Release{r}); err != nil {
		return nil, err
	}

	rows, err := DB.Query(`
		SELECT rt.disc, rt.position, t.uuid, t.title, t.duration_seconds, t.img_url, t.has_file, COALESCE(t.mbid, ''), COALESCE(t.isrc, '')
		FROM release_tracks rt
		JOIN tracks t ON t.uuid = rt.track_id
		WHERE rt.release_id = ?
		ORDER BY rt.disc, rt.position;`, id)
	if err != nil {
		return nil, err
	}
	var tracks []*audio.Track
	for rows.Next() {
		var rt audio.ReleaseTrack
		var t audio.Track
		var seconds int
		if err := rows.Scan(&rt.Disc, &rt.Position, &t.ID, &t.Title, &seconds, &t.ImgURL, &t.HasFile, &t.MBID, &t.ISRC); err != nil {
			rows.Close()
			return nil, err
		}
		t.Duration = time.Duration(seconds) * time.Second
		rt.Track = &t
		r.Tracks = append(r.Tracks, &rt)
		tracks = append(tracks, &t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := attachArtists(tracks); err != nil {
		return nil, err
	}
	// lets a single's page link to the album its song is also on
	return r, attachReleases(tracks)
}

func CreateRelease(title string, typ audio.ReleaseType, date string, artistIDs []string) (*audio.Release, error) {
	r := &audio.Release{ID: uuid.New(), Title: title, Type: typ, Date: date}

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := insertRelease(tx, r, artistIDs); err != nil {
		return nil, err
	}
	return r, tx.Commit()
}

func insertRelease(tx *sql.Tx, r *audio.Release, artistIDs []string) error {
	// an empty date is stored as NULL so it sorts after dated releases
	var dateArg any
	if r.Date != "" {
		dateArg = r.Date
	}
	_, err := tx.Exec(
		`INSERT INTO releases(uuid, title, type, release_date) VALUES(?, ?, ?, ?);`,
		r.ID.String(), r.Title, string(r.Type), dateArg,
	)
	if err != nil {
		return err
	}
	return insertReleaseArtists(tx, r.ID.String(), artistIDs)
}

func insertReleaseArtists(tx *sql.Tx, releaseID string, artistIDs []string) error {
	for i, artistID := range artistIDs {
		_, err := tx.Exec(
			`INSERT INTO release_artist_map(release_id, artist_id, position) VALUES(?, ?, ?);`,
			releaseID, artistID, i+1,
		)
		if isForeignKeyViolation(err) {
			return ErrBadReference
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdateRelease changes a release's title, type, date and artists.
func UpdateRelease(id, title string, typ audio.ReleaseType, date string, artistIDs []string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var dateArg any
	if date != "" {
		dateArg = date
	}
	res, err := tx.Exec(
		`UPDATE releases SET title = ?, type = ?, release_date = ? WHERE uuid = ?;`,
		title, string(typ), dateArg, id,
	)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(`DELETE FROM release_artist_map WHERE release_id = ?;`, id); err != nil {
		return err
	}
	if err := insertReleaseArtists(tx, id, artistIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteRelease removes a release. Its tracks stay in the library.
func DeleteRelease(id string) error {
	res, err := DB.Exec(`DELETE FROM releases WHERE uuid = ?;`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	if rid, err := uuid.Parse(id); err == nil {
		removeCover(rid)
	}
	return nil
}

// SetReleaseTracks replaces a release's tracklist.
func SetReleaseTracks(releaseID string, entries []ReleaseTrackInput) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// rewritten as a whole: updating positions in place would collide with
	// the (release, disc, position) primary key halfway through a reorder
	if _, err := tx.Exec(`DELETE FROM release_tracks WHERE release_id = ?;`, releaseID); err != nil {
		return err
	}
	next := map[int]int{}
	for _, e := range entries {
		next[e.Disc]++
		err := insertReleaseTrack(tx, e.TrackID, ReleaseLink{ReleaseID: releaseID, Disc: e.Disc, Position: next[e.Disc]})
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PlaceTrack puts an existing track on a release.
func PlaceTrack(trackID string, link ReleaseLink) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertReleaseTrack(tx, trackID, link); err != nil {
		return err
	}
	return tx.Commit()
}

// NextPositions returns, per release and disc, the position after the last track.
func NextPositions() (map[string]map[int]int, error) {
	rows, err := DB.Query(`SELECT release_id, disc, MAX(position) FROM release_tracks GROUP BY release_id, disc;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	next := map[string]map[int]int{}
	for rows.Next() {
		var releaseID string
		var disc, last int
		if err := rows.Scan(&releaseID, &disc, &last); err != nil {
			return nil, err
		}
		if next[releaseID] == nil {
			next[releaseID] = map[int]int{}
		}
		next[releaseID][disc] = last + 1
	}
	return next, rows.Err()
}

func insertReleaseTrack(tx *sql.Tx, trackID string, link ReleaseLink) error {
	_, err := tx.Exec(
		`INSERT INTO release_tracks(release_id, track_id, disc, position) VALUES(?, ?, ?, ?);`,
		link.ReleaseID, trackID, link.Disc, link.Position,
	)
	var e *sqlite.Error
	if errors.As(err, &e) {
		switch e.Code() {
		case 787: // SQLITE_CONSTRAINT_FOREIGNKEY
			return ErrBadReference
		case 1555: // SQLITE_CONSTRAINT_PRIMARYKEY: (release, disc, position)
			return ErrSlotTaken
		case 2067: // SQLITE_CONSTRAINT_UNIQUE: (release, track)
			return ErrAlreadyOnRelease
		}
	}
	return err
}

func linkExistingTrack(fp Fingerprint, link ReleaseLink) (string, error) {
	var trackID string
	err := DB.QueryRow(
		`SELECT uuid FROM tracks WHERE file_size = ? AND head_hash = ?;`,
		fp.Size, fp.HeadHash,
	).Scan(&trackID)
	if err != nil {
		return "", err
	}

	tx, err := DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := insertReleaseTrack(tx, trackID, link); err != nil {
		return "", err
	}
	return trackID, tx.Commit()
}

// attachReleaseArtists fills in Artists for all releases with a single query.
func attachReleaseArtists(releases []*audio.Release) error {
	if len(releases) == 0 {
		return nil
	}
	byID := make(map[string]*audio.Release, len(releases))
	args := make([]any, 0, len(releases))
	for _, r := range releases {
		byID[r.ID.String()] = r
		args = append(args, r.ID.String())
	}

	rows, err := DB.Query(`
		SELECT m.release_id, a.uuid, a.name, a.img_url, COALESCE(a.mbid, '')
		FROM release_artist_map m
		JOIN artists a ON a.uuid = m.artist_id
		WHERE m.release_id IN (`+placeholders(len(releases))+`)
		ORDER BY m.position;`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var releaseID string
		var a audio.Artist
		if err := rows.Scan(&releaseID, &a.ID, &a.Name, &a.ImgURL, &a.MBID); err != nil {
			return err
		}
		if r := byID[releaseID]; r != nil {
			r.Artists = append(r.Artists, &a)
		}
	}
	return rows.Err()
}

// attachReleases fills in Releases (without their tracks) for all tracks with a single query.
func attachReleases(tracks []*audio.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	byID := make(map[string]*audio.Track, len(tracks))
	args := make([]any, 0, len(tracks))
	for _, t := range tracks {
		byID[t.ID.String()] = t
		args = append(args, t.ID.String())
	}

	rows, err := DB.Query(`
		SELECT rt.track_id, r.uuid, r.title, r.type, COALESCE(r.release_date, ''), r.img_url, COALESCE(r.mbid, ''), COALESCE(r.release_group_mbid, '')
		FROM release_tracks rt
		JOIN releases r ON r.uuid = rt.release_id
		WHERE rt.track_id IN (`+placeholders(len(tracks))+`)
		ORDER BY r.release_date IS NULL, r.release_date;`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var trackID string
		var r audio.Release
		if err := rows.Scan(&trackID, &r.ID, &r.Title, &r.Type, &r.Date, &r.ImgURL, &r.MBID, &r.ReleaseGroupMBID); err != nil {
			return err
		}
		if t := byID[trackID]; t != nil {
			t.Releases = append(t.Releases, &r)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, t := range tracks {
		applyReleaseCover(t)
	}
	return nil
}

// applyReleaseCover shows the cover of the track's most important release
// (album, then EP, single, compilation) unless the track has its own image.
func applyReleaseCover(t *audio.Track) {
	if t.ImgURL != DefaultCover {
		return
	}
	var best *audio.Release
	for _, r := range t.Releases {
		if r.ImgURL == DefaultCover {
			continue
		}
		if best == nil || coverPriority[string(r.Type)] < coverPriority[string(best.Type)] {
			best = r
		}
	}
	if best != nil {
		t.ImgURL = best.ImgURL
	}
}
