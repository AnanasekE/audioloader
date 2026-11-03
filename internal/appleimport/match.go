// Package appleimport matches Apple Music songs to MusicBrainz recordings.
// Apple's public pages carry no ISRCs, so matching is by text: the title,
// the artists, the album and the length.
package appleimport

import (
	"context"
	"sort"
	"strings"
	"time"

	"audioloader/internal/applemusic"
	"audioloader/internal/match"
	"audioloader/internal/mb"
	"audioloader/internal/tagging"
)

const (
	// confidentTitle is how well the titles must fit for a sure match; it is
	// the YouTube Music source's bar too.
	confidentTitle = 0.9
	// confidentLength is how far apart the lengths may be for a sure match.
	confidentLength = 3 * time.Second
	// searchLimit is how many recordings one search asks MusicBrainz for.
	searchLimit = 10
)

// Candidate is a MusicBrainz recording that may be an Apple Music song.
type Candidate struct {
	Recording mb.Recording
	// Release is the release the song would be added on: one titled like
	// the Apple album if the recording is on one, else tagging.PickRelease's
	// choice. nil if the recording is on none.
	Release   *mb.Release
	Score     float64 // 0..1
	Confident bool    // title, artist and length all fit
}

// Find searches MusicBrainz for song and returns its recordings, best
// first: the confident ones, then by score. When the full artist credit
// finds nothing sure ("Drake & Don Toliver" is often "Drake feat. Don
// Toliver" there), it searches once more with the first artist alone.
func Find(ctx context.Context, c *mb.Client, song applemusic.Song) ([]Candidate, error) {
	cands, err := search(ctx, c, song, song.Artist)
	if err != nil {
		return nil, err
	}
	if first := firstArtist(song.Artist); !anyConfident(cands) && first != song.Artist {
		more, err := search(ctx, c, song, first)
		if err != nil {
			return nil, err
		}
		cands = merge(cands, more)
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.Confident != b.Confident {
			return a.Confident
		}
		return a.Score > b.Score
	})
	return cands, nil
}

// Best is the candidate to add without asking, or nil when none is sure.
func Best(cands []Candidate) *Candidate {
	if len(cands) > 0 && cands[0].Confident {
		return &cands[0]
	}
	return nil
}

func anyConfident(cands []Candidate) bool { return Best(cands) != nil }

func search(ctx context.Context, c *mb.Client, song applemusic.Song, artist string) ([]Candidate, error) {
	recs, err := c.SearchSongs(ctx, artist+" - "+song.Title, searchLimit)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(recs))
	for _, rec := range recs {
		out = append(out, score(song, rec))
	}
	return out, nil
}

// merge adds the candidates of b that a doesn't have.
func merge(a, b []Candidate) []Candidate {
	seen := map[string]bool{}
	for _, c := range a {
		seen[c.Recording.ID] = true
	}
	for _, c := range b {
		if !seen[c.Recording.ID] {
			a = append(a, c)
		}
	}
	return a
}

// score rates how well rec fits song.
func score(song applemusic.Song, rec mb.Recording) Candidate {
	title := match.TitleScore(rec.Title, song.Title, artistNames(song.Artist))
	artist := artistCredited(song.Artist, rec.ArtistCredit)
	length, near := lengthScore(song.Duration, time.Duration(rec.Length)*time.Millisecond)

	cand := Candidate{Recording: rec, Release: pickRelease(rec.Releases, song.Album)}
	album := 0.0
	if cand.Release != nil && match.Same(cand.Release.Title, song.Album) {
		album = 1
	}
	artistScore := 0.0
	if artist {
		artistScore = 1
	}
	cand.Score = 0.45*title + 0.25*artistScore + 0.2*length + 0.1*album
	cand.Confident = title >= confidentTitle && artist && near
	return cand
}

// pickRelease prefers a release titled like the Apple album; the
// recording's other releases are then only a fallback.
func pickRelease(releases []mb.Release, album string) *mb.Release {
	var same []mb.Release
	for _, r := range releases {
		if album != "" && match.Same(r.Title, album) {
			same = append(same, r)
		}
	}
	if len(same) > 0 {
		return tagging.PickRelease(same)
	}
	return tagging.PickRelease(releases)
}

// lengthScore rates two lengths: 1 within confidentLength, 0.5 within 10 s,
// 0.25 when either is unknown, else 0. near says the lengths are known and
// within confidentLength.
func lengthScore(a, b time.Duration) (score float64, near bool) {
	if a <= 0 || b <= 0 {
		return 0.25, false
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	switch {
	case diff <= confidentLength:
		return 1, true
	case diff <= 10*time.Second:
		return 0.5, false
	}
	return 0, false
}

// artistSeps split an Apple artist line into names.
var artistSeps = strings.NewReplacer(" & ", "\x00", ", ", "\x00", " feat. ", "\x00", " ft. ", "\x00", " featuring ", "\x00", " x ", "\x00", " × ", "\x00")

// artistNames splits "Drake, Don Toliver & Yebba" into its names.
func artistNames(line string) []string {
	var out []string
	for _, n := range strings.Split(artistSeps.Replace(line), "\x00") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func firstArtist(line string) string {
	if names := artistNames(line); len(names) > 0 {
		return names[0]
	}
	return line
}

// artistCredited reports whether the credits name the Apple artist line:
// as a whole, or one of its names (an artist of a collaboration is enough).
func artistCredited(line string, credits mb.Credits) bool {
	if match.Close(line, credits.String()) {
		return true
	}
	for _, name := range artistNames(line) {
		for _, c := range credits {
			if match.Close(name, c.Name) || match.Close(name, c.Artist.Name) {
				return true
			}
		}
	}
	return false
}
