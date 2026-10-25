// Package sources defines where downloads come from. Each source is a small
// module (yt-dlp, a watched folder, ...) behind the Source interface; the
// download job asks all of them and takes the best candidate.
package sources

import (
	"context"
	"sort"
	"time"
)

// Want describes the track a download is for.
type Want struct {
	Title    string
	Artists  []string
	Release  string
	MBID     string // MusicBrainz recording ID
	ISRC     string
	Duration time.Duration // 0 if unknown
}

// Candidate is a file a source could provide.
type Candidate struct {
	Source   string // Source.Name()
	Ref      string // source-specific: a path, a URL
	Label    string // shown to the user
	Lossless bool
	Bitrate  int // kbit/s, 0 if unknown
}

// File is a fetched candidate on local disk. Cleanup removes it if the
// source made a temporary copy.
type File struct {
	Path    string
	Cleanup func()
}

type Source interface {
	Name() string
	// Find returns the source's candidates for want, best first.
	Find(ctx context.Context, want Want) ([]Candidate, error)
	// Fetch makes the candidate a local file, reporting progress (0..1).
	Fetch(ctx context.Context, c Candidate, progress func(float64)) (File, error)
}

// Rank sorts candidates best first: lossless before lossy, then higher
// bitrate, then by the position of their source in priority.
func Rank(cands []Candidate, priority []string) {
	pos := func(name string) int {
		for i, p := range priority {
			if p == name {
				return i
			}
		}
		return len(priority)
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.Lossless != b.Lossless {
			return a.Lossless
		}
		if a.Bitrate != b.Bitrate {
			return a.Bitrate > b.Bitrate
		}
		return pos(a.Source) < pos(b.Source)
	})
}
