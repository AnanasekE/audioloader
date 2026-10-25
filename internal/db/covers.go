package db

import (
	"bytes"
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
	if err := os.MkdirAll(CoversDir, 0o755); err != nil {
		return err
	}

	// write to a temp file first, so a failed upload keeps the old cover
	tmp, err := os.CreateTemp(CoversDir, ".upload-*")
	if err != nil {
		return err
	}
	_, err = io.Copy(tmp, r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
	}
	// the old cover may have another extension
	removeCover(id)
	if err := os.Rename(tmp.Name(), filepath.Join(CoversDir, id.String()+ext)); err != nil {
		os.Remove(tmp.Name())
		return err
	}

	// the version query busts browser caches when the cover changes
	url := fmt.Sprintf("/covers/%s?v=%d", id, time.Now().Unix())
	_, err = DB.Exec(`UPDATE releases SET img_url = ? WHERE uuid = ?;`, url, id.String())
	return err
}

func removeCover(id uuid.UUID) {
	matches, _ := filepath.Glob(filepath.Join(CoversDir, id.String()+".*"))
	for _, m := range matches {
		os.Remove(m)
	}
}

// coverPriority orders release types for picking a track's cover: a song
// on an album shows the album's cover rather than its single's.
var coverPriority = map[string]int{"album": 0, "ep": 1, "single": 2, "compilation": 3}

// MaxCoverSize caps stored cover images.
const MaxCoverSize = 10 << 20

// ErrBadImage is returned for a cover that isn't a JPEG, PNG or WebP image,
// or is larger than MaxCoverSize.
var ErrBadImage = errors.New("cover must be a JPEG, PNG or WebP image of at most 10 MiB")

// coverTypes maps the sniffed content types we accept to their extension.
var coverTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// SaveCoverImage checks that r is an accepted image, by its bytes rather
// than any name or claimed type, and stores it as the release's cover.
func SaveCoverImage(id uuid.UUID, r io.Reader) error {
	data, err := io.ReadAll(io.LimitReader(r, MaxCoverSize+1))
	if err != nil {
		return err
	}
	if len(data) > MaxCoverSize {
		return ErrBadImage
	}
	ext, ok := coverTypes[http.DetectContentType(data)]
	if !ok {
		return ErrBadImage
	}
	return SetReleaseCover(id, bytes.NewReader(data), ext)
}
