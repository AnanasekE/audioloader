// Package sources defines where downloads come from. Each source is a small
// module (yt-dlp, a watched folder, ...) behind the Source interface; the
// download job asks them in a fixed order and takes the first candidate.
package sources

import (
	"context"
	"sort"
	"time"
)

// Want describes the track a download is for.
type Want struct {
	Title   string
	Artists []string
	Release string
	MBID    string // MusicBrainz recording ID
	ISRC    string
	// SpotifyURL is the track's Spotify link, if MusicBrainz or an album
	// mapping gave one.
	SpotifyURL string
	Duration   time.Duration // 0 if unknown
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

// Order is the fixed order sources are tried in: the user's own files
// first, then YouTube Music, then spotDL, then YouTube. A later source is
// only asked when the earlier ones found nothing.
var Order = []string{"folder", "ytmusic", "spotdl", "yt-dlp"}

// Position is a source's place in Order; unknown sources come last.
func Position(name string) int {
	for i, o := range Order {
		if o == name {
			return i
		}
	}
	return len(Order)
}

// Rank sorts candidates best first: by their source's place in Order, then
// lossless before lossy, then higher bitrate. Quality only breaks ties
// between candidates of the same source.
func Rank(cands []Candidate) {
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if pa, pb := Position(a.Source), Position(b.Source); pa != pb {
			return pa < pb
		}
		if a.Lossless != b.Lossless {
			return a.Lossless
		}
		return a.Bitrate > b.Bitrate
	})
}
