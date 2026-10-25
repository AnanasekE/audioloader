// Package folder finds downloads in a watched folder, such as where a
// store's purchases or an artist's free downloads end up. Files are matched
// by their tags and used in place: the library stores its own copy.
package folder

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/match"
	"audioloader/internal/sources"
	"audioloader/internal/tags"
)

// Name is the source name stored on downloads.
const Name = "folder"

var audioExts = map[string]bool{".mp3": true, ".flac": true, ".ogg": true, ".opus": true, ".m4a": true, ".mp4": true, ".wav": true}

var losslessExts = map[string]bool{".flac": true, ".wav": true}

// Source indexes Dir's audio files, re-reading only files that changed.
type Source struct {
	Dir string

	mu    sync.Mutex
	index map[string]entry // by path
}

type entry struct {
	mod      time.Time
	size     int64
	info     tags.Info
	duration time.Duration
}

// FromEnv returns the source for AUDIOLOADER_WATCH, or nil if it isn't set.
func FromEnv() *Source {
	dir := os.Getenv("AUDIOLOADER_WATCH")
	if dir == "" {
		return nil
	}
	return &Source{Dir: dir}
}

func (s *Source) Name() string { return Name }

func (s *Source) Find(ctx context.Context, want sources.Want) ([]sources.Candidate, error) {
	entries, err := s.scan(ctx)
	if err != nil {
		return nil, err
	}
	var out []sources.Candidate
	for path, e := range entries {
		if !matches(e, want) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		c := sources.Candidate{Source: Name, Ref: path, Label: path, Lossless: losslessExts[ext]}
		if e.duration > 0 {
			c.Bitrate = int(float64(e.size*8) / e.duration.Seconds() / 1000)
		}
		out = append(out, c)
	}
	return out, nil
}

// matches compares by MusicBrainz ID, then ISRC, then title, an artist and
// a length within 3 seconds.
func matches(e entry, want sources.Want) bool {
	if want.MBID != "" && e.info.RecordingID != "" {
		return want.MBID == e.info.RecordingID
	}
	if want.ISRC != "" && e.info.ISRC != "" {
		return want.ISRC == e.info.ISRC
	}
	if e.info.Title == "" || !match.Same(e.info.Title, want.Title) {
		return false
	}
	if want.Duration > 0 && e.duration > 0 {
		diff := want.Duration - e.duration
		if diff < -3*time.Second || diff > 3*time.Second {
			return false
		}
	}
	for _, a := range want.Artists {
		// tags often join several artists into one string
		if match.Same(a, e.info.Artist) || strings.Contains(match.Normalize(e.info.Artist), match.Normalize(a)) {
			return true
		}
	}
	return false
}

func (s *Source) Fetch(ctx context.Context, c sources.Candidate, progress func(float64)) (sources.File, error) {
	if _, err := os.Stat(c.Ref); err != nil {
		return sources.File{}, err
	}
	if progress != nil {
		progress(1)
	}
	// the user's file stays where it is
	return sources.File{Path: c.Ref, Cleanup: func() {}}, nil
}

// scan walks Dir and returns the current index.
func (s *Source) scan(ctx context.Context) (map[string]entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index == nil {
		s.index = map[string]entry{}
	}
	seen := map[string]bool{}
	err := filepath.WalkDir(s.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable parts
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() || !audioExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		seen[path] = true
		if e, ok := s.index[path]; ok && e.mod.Equal(fi.ModTime()) && e.size == fi.Size() {
			return nil
		}
		s.index[path] = read(path, fi)
		return nil
	})
	for path := range s.index {
		if !seen[path] {
			delete(s.index, path)
		}
	}
	out := make(map[string]entry, len(s.index))
	for k, v := range s.index {
		out[k] = v
	}
	return out, err
}

func read(path string, fi fs.FileInfo) entry {
	e := entry{mod: fi.ModTime(), size: fi.Size()}
	f, err := os.Open(path)
	if err != nil {
		return e
	}
	defer f.Close()
	e.info, _ = tags.Read(f)
	if _, err := f.Seek(0, 0); err == nil {
		e.duration, _ = audio.ProbeDuration(f)
	}
	return e
}
