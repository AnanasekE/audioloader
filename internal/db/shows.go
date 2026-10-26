package db

import (
	"database/sql"
	"errors"
	"io"
	"os"
	"strings"
	"time"
	"uuid"

	"audioloader/internal/audio"
)

// ResolveShow returns the show with title (ignoring case), creating it
// with author if there is none. A show of the other mode with that title is
// used too, since titles are unique: it is then on both sides. A show made
// in secret mode stays hidden from normal mode until it has an episode.
func ResolveShow(v Viewer, title, author string) (*audio.Show, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("show title is required")
	}
	s, err := scanShow(DB.QueryRow(selectShow(All)+` AND lower(s.title) = lower(?);`, title))
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	s = &audio.Show{ID: uuid.New(), Title: title, Author: author, ImgURL: DefaultCover}
	_, err = DB.Exec(`INSERT INTO shows(uuid, title, author, secret) VALUES(?, ?, ?, ?);`, s.ID.String(), title, nullIfEmpty(author), v.IsSecret())
	return s, err
}

// selectShow selects shows aliased s, with episode counts as v sees them.
func selectShow(v Viewer) string {
	return `
	SELECT s.uuid, s.title, COALESCE(s.author, ''), s.img_url,
		(SELECT COUNT(*) FROM tracks t WHERE t.show_id = s.uuid AND t.has_file = 1 AND ` + v.trackCond("t") + `),
		(SELECT COUNT(*) FROM tracks t LEFT JOIN playback_positions p ON p.track_id = t.uuid
			WHERE t.show_id = s.uuid AND t.has_file = 1 AND COALESCE(p.finished, 0) = 0 AND ` + v.trackCond("t") + `)
	FROM shows s WHERE ` + v.showCond("s")
}

func scanShow(s scanner) (*audio.Show, error) {
	var sh audio.Show
	if err := s.Scan(&sh.ID, &sh.Title, &sh.Author, &sh.ImgURL, &sh.Count, &sh.Unplayed); err != nil {
		return nil, err
	}
	return &sh, nil
}

func ListShows(v Viewer) ([]*audio.Show, error) {
	rows, err := DB.Query(selectShow(v) + ` ORDER BY s.title COLLATE NOCASE;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*audio.Show
	for rows.Next() {
		s, err := scanShow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetShow returns a show with its episodes, newest first, and where
// playback of each stopped.
func GetShow(v Viewer, id string) (*audio.Show, error) {
	s, err := scanShow(DB.QueryRow(selectShow(v)+` AND s.uuid = ?;`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := DB.Query(selectTrack+` WHERE t.show_id = ? AND t.has_file = 1 AND `+v.trackCond("t")+`
		ORDER BY t.published IS NULL, t.published DESC, t.rowid DESC;`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		t.ImgURL = showCover(t, s)
		s.Episodes = append(s.Episodes, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := attachArtists(s.Episodes); err != nil {
		return nil, err
	}
	return s, attachPositions(s.Episodes)
}

// showCover shows the show's image on its episodes without their own.
func showCover(t *audio.Track, s *audio.Show) string {
	if t.ImgURL == DefaultCover {
		return s.ImgURL
	}
	return t.ImgURL
}

// AddEpisode stores an episode of a show, a secret one for a secret mode
// viewer. It returns ErrExists if the file is already stored, and ErrHidden
// if it is stored on the other side.
func AddEpisode(v Viewer, showID string, track *audio.Track, file io.ReadSeeker, ext string) (string, error) {
	fp, err := FingerprintOf(file)
	if err != nil {
		return "", err
	}
	if existing, err := trackByFingerprint(fp); err != nil || existing != "" {
		if err == nil {
			err = ErrExists
			if hidden, herr := hiddenFingerprint(v, fp); herr != nil {
				err = herr
			} else if hidden {
				err = ErrHidden
			}
		}
		return existing, err
	}
	id := uuid.New().String()
	path, err := saveTrackFile(id, file, ext)
	if err != nil {
		return "", err
	}
	_, err = DB.Exec(`
		INSERT INTO tracks(uuid, title, duration_seconds, file_size, head_hash, kind, show_id, published, secret)
		VALUES(?, ?, ?, ?, ?, 'episode', ?, ?, ?);`,
		id, track.Title, int(track.Duration.Seconds()), fp.Size, fp.HeadHash, showID, nullIfEmpty(track.Published), v.IsSecret())
	if err != nil {
		os.Remove(path)
		if isForeignKeyViolation(err) {
			return "", ErrBadReference
		}
		if isUniqueViolation(err) {
			return "", ErrExists
		}
		return "", err
	}
	return id, nil
}

// SavePosition records where playback of a track stopped.
func SavePosition(trackID string, seconds float64, finished bool) error {
	_, err := DB.Exec(`
		INSERT INTO playback_positions(track_id, seconds, finished, updated_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(track_id) DO UPDATE SET
			seconds = excluded.seconds, finished = excluded.finished, updated_at = excluded.updated_at;`,
		trackID, seconds, finished, sqlTime(time.Now()))
	if isForeignKeyViolation(err) {
		return ErrNotFound
	}
	return err
}

// ClearPosition marks a track unplayed.
func ClearPosition(trackID string) error {
	_, err := DB.Exec(`DELETE FROM playback_positions WHERE track_id = ?;`, trackID)
	return err
}

// attachPositions fills in Position and Finished.
func attachPositions(tracks []*audio.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	byID := make(map[string]*audio.Track, len(tracks))
	args := make([]any, 0, len(tracks))
	for _, t := range tracks {
		byID[t.ID.String()] = t
		args = append(args, t.ID.String())
	}
	rows, err := DB.Query(`SELECT track_id, seconds, finished FROM playback_positions WHERE track_id IN (`+placeholders(len(tracks))+`);`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var seconds float64
		var finished bool
		if err := rows.Scan(&id, &seconds, &finished); err != nil {
			return err
		}
		if t := byID[id]; t != nil {
			t.Position = time.Duration(seconds * float64(time.Second))
			t.Finished = finished
		}
	}
	return rows.Err()
}

// ContinueListening returns started, unfinished episodes, the most
// recently played first.
func ContinueListening(v Viewer, limit int) ([]*audio.Track, error) {
	rows, err := DB.Query(selectTrack+` JOIN playback_positions p ON p.track_id = t.uuid
		WHERE t.kind = 'episode' AND t.has_file = 1 AND p.finished = 0 AND p.seconds > 0 AND `+v.trackCond("t")+`
		ORDER BY p.updated_at DESC LIMIT ?;`, limit)
	if err != nil {
		return nil, err
	}
	var tracks []*audio.Track
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		tracks = append(tracks, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := attachShowCovers(tracks); err != nil {
		return nil, err
	}
	return tracks, attachPositions(tracks)
}

func attachShowCovers(tracks []*audio.Track) error {
	for _, t := range tracks {
		if t.ShowID == "" || t.ImgURL != DefaultCover {
			continue
		}
		var img string
		if err := DB.QueryRow(`SELECT img_url FROM shows WHERE uuid = ?;`, t.ShowID).Scan(&img); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if img != "" {
			t.ImgURL = img
		}
	}
	return nil
}

// Chapter is a named point in a long track.
type Chapter = audio.Chapter

// SetChapters replaces a track's chapters.
func SetChapters(trackID string, chapters []Chapter) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM chapters WHERE track_id = ?;`, trackID); err != nil {
		return err
	}
	for _, c := range chapters {
		_, err := tx.Exec(`INSERT OR IGNORE INTO chapters(track_id, start_ms, title) VALUES(?, ?, ?);`,
			trackID, c.Start.Milliseconds(), c.Title)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func GetChapters(trackID string) ([]Chapter, error) {
	rows, err := DB.Query(`SELECT start_ms, title FROM chapters WHERE track_id = ? ORDER BY start_ms;`, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chapter
	for rows.Next() {
		var ms int64
		var c Chapter
		if err := rows.Scan(&ms, &c.Title); err != nil {
			return nil, err
		}
		c.Start = time.Duration(ms) * time.Millisecond
		out = append(out, c)
	}
	return out, rows.Err()
}
