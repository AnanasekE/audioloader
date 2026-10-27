package db

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"uuid"
)

// TracksDir is where uploaded track files are stored as <uuid><ext>.
var TracksDir = "data/tracks"

// ErrBadFileType is returned for a track file whose extension isn't an audio
// type. Files are served with a Content-Type from their extension, so an
// uploaded .html file would otherwise run as the site.
var ErrBadFileType = errors.New("file must be MP3, FLAC, Ogg, Opus, M4A, MP4 or WAV")

var audioExts = map[string]bool{".mp3": true, ".flac": true, ".ogg": true, ".opus": true, ".m4a": true, ".mp4": true, ".wav": true}

// AudioExt is the lowercased extension of name, and whether it's an audio
// type a track file may have.
func AudioExt(name string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	return ext, audioExts[ext]
}

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
	ext, ok := AudioExt(ext)
	if !ok {
		return "", ErrBadFileType
	}
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
