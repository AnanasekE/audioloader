package db

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"uuid"
)

// CoversDir is where release covers are stored as <release uuid><ext>.
var CoversDir = "data/covers"

// DefaultCover is the img_url of tracks and releases without their own image.
const DefaultCover = "/assets/img/default_track.png"

// CoverFilePath returns the path of the stored cover for release id.
func CoverFilePath(id uuid.UUID) (string, error) {
	// as in TrackFilePath, a parsed uuid can't escape CoversDir
	matches, err := filepath.Glob(filepath.Join(CoversDir, id.String()+".*"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", ErrNotFound
	}
	return matches[0], nil
}

// SetReleaseCover stores r as the cover of release id, replacing any
// previous one. ext is the file extension including the dot.
func SetReleaseCover(id uuid.UUID, r io.Reader, ext string) error {
	var exists bool
	if err := DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM releases WHERE uuid = ?);`, id.String()).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	name := id.String() + ext
	if err := writeImage(CoversDir, name, r); err != nil {
		return err
	}
	// the old cover may have another extension
	removeImages(filepath.Join(CoversDir, id.String()+".*"), name)

	// the version query busts browser caches when the cover changes
	url := fmt.Sprintf("/covers/%s?v=%d", id, time.Now().Unix())
	_, err := DB.Exec(`UPDATE releases SET img_url = ? WHERE uuid = ?;`, url, id.String())
	return err
}

// writeImage stores r in dir as name. It writes to a temp file first, so a
// failed upload keeps the old image.
func writeImage(dir, name string, r io.Reader) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return err
	}
	_, err = io.Copy(tmp, r)
	if err == nil {
		// CreateTemp makes the file private; images are served to anyone
		err = tmp.Chmod(0o644)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// removeImages deletes the files matching pattern, except the one named keep.
func removeImages(pattern, keep string) {
	matches, _ := filepath.Glob(pattern)
	for _, m := range matches {
		if filepath.Base(m) != keep {
			os.Remove(m)
		}
	}
}

func removeCover(id uuid.UUID) {
	removeImages(filepath.Join(CoversDir, id.String()+".*"), "")
}

// coverPriority orders release types for picking a track's cover: a song
// on an album shows the album's cover rather than its single's.
var coverPriority = map[string]int{"album": 0, "ep": 1, "single": 2, "compilation": 3}

// MaxCoverSize caps stored cover and artist images.
const MaxCoverSize = 10 << 20

// ErrBadImage is returned for an image that isn't a JPEG, PNG or WebP image,
// or is larger than MaxCoverSize.
var ErrBadImage = errors.New("image must be a JPEG, PNG or WebP image of at most 10 MiB")

// imageTypes maps the sniffed content types we accept to their extension.
var imageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// sniffImage checks that r is an accepted image, by its bytes rather than
// any name or claimed type, and returns its content and extension.
func sniffImage(r io.Reader) ([]byte, string, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxCoverSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > MaxCoverSize {
		return nil, "", ErrBadImage
	}
	ext, ok := imageTypes[http.DetectContentType(data)]
	if !ok {
		return nil, "", ErrBadImage
	}
	return data, ext, nil
}

// SaveCoverImage checks that r is an accepted image and stores it as the
// release's cover.
func SaveCoverImage(id uuid.UUID, r io.Reader) error {
	data, ext, err := sniffImage(r)
	if err != nil {
		return err
	}
	return SetReleaseCover(id, bytes.NewReader(data), ext)
}

// ArtistImagesDir is where artist pictures are stored as <artist uuid>-<n><ext>.
var ArtistImagesDir = "data/artists"

// artistImgURL is the SQL for an artist's picture URL on a table aliased a:
// the stored picture when there is one (the file name is the cache-busting
// version), else the default image.
const artistImgURL = `CASE WHEN a.image IS NULL THEN a.img_url ELSE '/artist-images/' || a.uuid || '?v=' || a.image END`

// ArtistImagePath returns the path of the stored picture of artist id.
func ArtistImagePath(id uuid.UUID) (string, error) {
	var name sql.NullString
	err := DB.QueryRow(`SELECT image FROM artists WHERE uuid = ?;`, id.String()).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !name.Valid) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	// the name is ours, but a Base keeps it inside the directory whatever the DB says
	return filepath.Join(ArtistImagesDir, filepath.Base(name.String)), nil
}

// SaveArtistImage checks that r is an accepted image and stores it as the
// artist's picture, replacing any previous one.
func SaveArtistImage(id uuid.UUID, r io.Reader) error {
	data, ext, err := sniffImage(r)
	if err != nil {
		return err
	}
	var old sql.NullString
	err = DB.QueryRow(`SELECT image FROM artists WHERE uuid = ?;`, id.String()).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	// a new name per upload, so browsers fetch the new picture
	name := fmt.Sprintf("%s-%d%s", id, time.Now().UnixNano(), ext)
	if err := writeImage(ArtistImagesDir, name, bytes.NewReader(data)); err != nil {
		return err
	}
	if _, err := DB.Exec(`UPDATE artists SET image = ? WHERE uuid = ?;`, name, id.String()); err != nil {
		os.Remove(filepath.Join(ArtistImagesDir, name))
		return err
	}
	if old.Valid {
		os.Remove(filepath.Join(ArtistImagesDir, filepath.Base(old.String)))
	}
	return nil
}

// RemoveArtistImage returns the artist to the default picture.
func RemoveArtistImage(id uuid.UUID) error {
	var old sql.NullString
	err := DB.QueryRow(`SELECT image FROM artists WHERE uuid = ?;`, id.String()).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := DB.Exec(`UPDATE artists SET image = NULL WHERE uuid = ?;`, id.String()); err != nil {
		return err
	}
	if old.Valid {
		os.Remove(filepath.Join(ArtistImagesDir, filepath.Base(old.String)))
	}
	return nil
}
