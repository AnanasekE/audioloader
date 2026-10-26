package db

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"audioloader/internal/audio"
)

// Limits for the tags of a secret track.
const (
	MaxTags      = 20
	MaxTagLength = 40
)

// NormalizeTags trims each tag, folds runs of spaces, drops empty ones and
// repeats (ignoring case) and keeps the first MaxTags. A tag longer than
// MaxTagLength is cut.
func NormalizeTags(tags []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range tags {
		t = strings.Join(strings.Fields(t), " ")
		if utf8.RuneCountInString(t) > MaxTagLength {
			t = string([]rune(t)[:MaxTagLength])
		}
		key := strings.ToLower(t)
		if t == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
		if len(out) == MaxTags {
			break
		}
	}
	return out
}

// setTags replaces the tags of a track.
func setTags(tx *sql.Tx, trackID string, tags []string) error {
	if _, err := tx.Exec(`DELETE FROM track_tags WHERE track_id = ?;`, trackID); err != nil {
		return err
	}
	for _, tag := range NormalizeTags(tags) {
		if _, err := tx.Exec(`INSERT INTO track_tags(track_id, tag) VALUES(?, ?);`, trackID, tag); err != nil {
			return err
		}
	}
	return nil
}

// attachTags fills in Tags for all tracks with a single query, in
// alphabetical order.
func attachTags(tracks []*audio.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	byID := make(map[string]*audio.Track, len(tracks))
	args := make([]any, 0, len(tracks))
	for _, t := range tracks {
		byID[t.ID.String()] = t
		args = append(args, t.ID.String())
	}
	rows, err := DB.Query(`SELECT track_id, tag FROM track_tags WHERE track_id IN (`+placeholders(len(tracks))+`) ORDER BY tag COLLATE NOCASE;`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, tag string
		if err := rows.Scan(&id, &tag); err != nil {
			return err
		}
		if t := byID[id]; t != nil {
			t.Tags = append(t.Tags, tag)
		}
	}
	return rows.Err()
}

// TrackImagesDir is where the pictures of secret tracks are stored as
// <track uuid>-<n><ext>.
var TrackImagesDir = "data/track-images"

// SecretCover is the picture of a secret track that has none of its own.
const SecretCover = "/assets/img/default_secret.png"

// trackImgURL is the SQL for a track's picture URL on a table aliased t: its
// own picture (the file name is the cache-busting version), the placeholder
// for a secret track, or the track's img_url.
const trackImgURL = `CASE WHEN t.image IS NOT NULL THEN '/track-images/' || t.uuid || '?v=' || t.image
	WHEN t.secret = 1 AND t.img_url = '` + DefaultCover + `' THEN '` + SecretCover + `' ELSE t.img_url END`

// TrackImagePath returns the path of the stored picture of track id, as
// seen by v.
func TrackImagePath(v Viewer, id uuid.UUID) (string, error) {
	var name sql.NullString
	err := DB.QueryRow(`SELECT t.image FROM tracks t WHERE t.uuid = ? AND `+v.trackCond("t")+`;`, id.String()).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !name.Valid) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(TrackImagesDir, filepath.Base(name.String)), nil
}

// SaveTrackImage checks that r is an accepted image and stores it as the
// picture of a secret track, replacing any previous one.
func SaveTrackImage(v Viewer, id uuid.UUID, r io.Reader) error {
	data, ext, err := sniffImage(r)
	if err != nil {
		return err
	}
	var old sql.NullString
	err = DB.QueryRow(`SELECT t.image FROM tracks t WHERE t.uuid = ? AND t.secret = 1 AND `+v.trackCond("t")+`;`, id.String()).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%s-%d%s", id, time.Now().UnixNano(), ext)
	if err := writeImage(TrackImagesDir, name, bytes.NewReader(data)); err != nil {
		return err
	}
	if _, err := DB.Exec(`UPDATE tracks SET image = ? WHERE uuid = ?;`, name, id.String()); err != nil {
		os.Remove(filepath.Join(TrackImagesDir, name))
		return err
	}
	if old.Valid {
		os.Remove(filepath.Join(TrackImagesDir, filepath.Base(old.String)))
	}
	return nil
}

// RemoveTrackImage returns a track to the placeholder picture.
func RemoveTrackImage(v Viewer, id uuid.UUID) error {
	var old sql.NullString
	err := DB.QueryRow(`SELECT t.image FROM tracks t WHERE t.uuid = ? AND `+v.trackCond("t")+`;`, id.String()).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := DB.Exec(`UPDATE tracks SET image = NULL WHERE uuid = ?;`, id.String()); err != nil {
		return err
	}
	if old.Valid {
		os.Remove(filepath.Join(TrackImagesDir, filepath.Base(old.String)))
	}
	return nil
}

// removeTrackImages deletes every stored picture of track id.
func removeTrackImages(id uuid.UUID) {
	removeImages(filepath.Join(TrackImagesDir, id.String()+"-*"), "")
}

// SetTrackTagsForTest sets the tags of a track outside an edit, for tests of
// other packages.
func SetTrackTagsForTest(trackID string, tags ...string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setTags(tx, trackID, tags); err != nil {
		return err
	}
	return tx.Commit()
}
