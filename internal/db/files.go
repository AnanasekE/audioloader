package db

import (
	"io"
	"os"
	"path/filepath"
	"uuid"
)

// TracksDir is where uploaded track files are stored as <uuid><ext>.
var TracksDir = "data/tracks"

// TrackFilePath returns the path of the stored file for track id.
func TrackFilePath(id uuid.UUID) (string, error) {
	// The ext isn't in the DB, so glob for it. A parsed uuid can't contain
	// "/", ".." or glob metacharacters, so this can't escape TracksDir.
	matches, err := filepath.Glob(filepath.Join(TracksDir, id.String()+".*"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", ErrNotFound
	}
	return matches[0], nil
}

// saveTrackFile writes file to TracksDir as <id><ext> and returns its path.
// On failure, any partially written file is removed.
func saveTrackFile(id string, file io.ReadSeeker, ext string) (string, error) {
	if err := os.MkdirAll(TracksDir, 0o755); err != nil {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	path := filepath.Join(TracksDir, id+ext)
	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(out, file)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}
