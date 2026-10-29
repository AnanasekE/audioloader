package ytdlp

import (
	"context"
	"strings"
	"time"

	"audioloader/internal/match"
	"audioloader/internal/sources"
)

// MusicName is the source name stored on downloads.
const MusicName = "ytmusic"

// LengthTolerance is how far a song's length may be from the track's.
var LengthTolerance = 2 * time.Second

// minTitleScore is how well a song's title must fit the track's.
const minTitleScore = 0.9

// MusicSource searches YouTube Music and offers its top song only when it
// clearly is the track.
type MusicSource struct{ C *Client }

func (MusicSource) Name() string { return MusicName }

// Find searches for "<first artist> <title>". Without a title or a known
// length there is nothing to be confident about.
func (s MusicSource) Find(ctx context.Context, want sources.Want) ([]sources.Candidate, error) {
	if want.Title == "" || want.Duration <= 0 {
		return nil, nil
	}
	query := want.Title
	if len(want.Artists) > 0 {
		query = want.Artists[0] + " " + query
	}
	found, err := s.C.MusicSearch(ctx, query, 5)
	if err != nil {
		return nil, err
	}
	best, ok := BestSong(found, want)
	if !ok {
		return nil, nil
	}
	return []sources.Candidate{{Source: MusicName, Ref: best.ID, Label: VideoURL(best.ID)}}, nil
}

func (s MusicSource) Fetch(ctx context.Context, c sources.Candidate, progress func(float64)) (sources.File, error) {
	return fetch(ctx, s.C, c.Ref, progress)
}

// BestSong picks the confident match from search results, if any: a length
// within LengthTolerance, a title that fits and an artist the song names.
// The smallest length difference wins, ties going to the better title.
func BestSong(found []Entry, want sources.Want) (Entry, bool) {
	var best Entry
	var bestDiff time.Duration
	var bestScore float64
	ok := false
	for _, e := range found {
		if e.Duration <= 0 || want.Duration <= 0 {
			continue
		}
		diff := time.Duration(e.Duration*float64(time.Second)) - want.Duration
		if diff < 0 {
			diff = -diff
		}
		if diff > LengthTolerance {
			continue
		}
		score := match.TitleScore(e.Title, want.Title, want.Artists)
		if score < minTitleScore || !namesArtist(e, want.Artists) {
			continue
		}
		if !ok || diff < bestDiff || diff == bestDiff && score > bestScore {
			best, bestDiff, bestScore, ok = e, diff, score, true
		}
	}
	return best, ok
}

// namesArtist reports whether the song credits one of artists: in its
// artists, its channel, or the "Artist - " prefix of its title.
func namesArtist(e Entry, artists []string) bool {
	credited := append([]string{ArtistName(e), match.TitlePrefix(e.Title)}, e.Artists...)
	for _, a := range artists {
		na := match.Normalize(a)
		if na == "" {
			continue
		}
		for _, c := range credited {
			if match.Same(a, c) || strings.Contains(match.Normalize(c), na) {
				return true
			}
		}
	}
	return false
}
