package db

import (
	"math/rand/v2"
	"slices"
	"strconv"

	"audioloader/internal/audio"
)

// Autoplay suggestions: when the queue runs out, tracks from the library that
// are like what just played. Only owned music tracks the viewer may see are
// ever suggested.

const (
	maxAutoplaySeeds = 5
	maxAutoplayN     = 50
	// release years this close count as "the same time"
	autoplayYears = 5
)

// releaseFacts is what a release says about a track: its type and year (0 if unknown).
type releaseFacts struct {
	typ  audio.ReleaseType
	year int
}

// profile is what the scoring looks at for one track.
type profile struct {
	secret   bool
	artists  map[string]bool
	releases []releaseFacts
	tags     map[string]bool
}

// AutoplaySuggestions returns up to n tracks like the seeds (the tracks just
// played, the current one first). A candidate scores +3 for an artist of a
// seed, +2 for an artist who shares a release with a seed artist, +1 for a
// release of the same type within 5 years of a seed's release, and for secret
// tracks +2 per shared tag. Seeds and exclude are never suggested. Ties and
// the tracks that score nothing come in random order, so fewer than n
// matches are filled up with random tracks. An episode as the first seed
// means no suggestions at all.
func AutoplaySuggestions(v Viewer, seeds, exclude []string, n int) ([]*audio.Track, error) {
	if n <= 0 {
		return nil, nil
	}
	n = min(n, maxAutoplayN)
	if len(seeds) > maxAutoplaySeeds {
		seeds = seeds[:maxAutoplaySeeds]
	}

	// seeds the viewer can see, in the order given; an episode first ends it here
	kinds, err := trackKinds(v, seeds)
	if err != nil {
		return nil, err
	}
	if len(seeds) > 0 && kinds[seeds[0]] == "episode" {
		return nil, nil
	}
	skip := map[string]bool{}
	for _, id := range append(append([]string{}, seeds...), exclude...) {
		skip[id] = true
	}

	profiles, err := autoplayProfiles(v)
	if err != nil {
		return nil, err
	}
	seed := seedProfile{artists: map[string]bool{}, tags: map[string]bool{}}
	for _, id := range seeds {
		if kinds[id] != "music" {
			continue
		}
		p := profiles[id]
		if p == nil {
			continue
		}
		for a := range p.artists {
			seed.artists[a] = true
		}
		seed.releases = append(seed.releases, p.releases...)
		if p.secret {
			for tag := range p.tags {
				seed.tags[tag] = true
			}
		}
	}
	if seed.related, err = relatedArtists(seed.artists); err != nil {
		return nil, err
	}

	type scored struct {
		id    string
		score int
	}
	var cands []scored
	for id, p := range profiles {
		if !skip[id] {
			cands = append(cands, scored{id, seed.score(p)})
		}
	}
	// map order is random already, but be explicit: ties must not depend on it
	rand.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
	sortStableDesc(cands, func(c scored) int { return c.score })

	ids := make([]string, 0, n)
	for _, c := range cands {
		if len(ids) == n {
			break
		}
		ids = append(ids, c.id)
	}
	return tracksByIDs(v, ids)
}

// seedProfile is the sum of the seeds' profiles, plus the artists related to theirs.
type seedProfile struct {
	artists, related, tags map[string]bool
	releases               []releaseFacts
}

func (s *seedProfile) score(p *profile) int {
	score := 0
	for a := range p.artists {
		if s.artists[a] {
			score += 3
			break
		}
	}
	for a := range p.artists {
		if s.related[a] {
			score += 2
			break
		}
	}
	if s.nearRelease(p) {
		score++
	}
	if p.secret {
		for tag := range p.tags {
			if s.tags[tag] {
				score += 2
			}
		}
	}
	return score
}

// nearRelease: some release of the track has the type of a seed's release,
// and a year within autoplayYears of it.
func (s *seedProfile) nearRelease(p *profile) bool {
	for _, c := range p.releases {
		for _, sr := range s.releases {
			if c.typ == sr.typ && c.year > 0 && sr.year > 0 && abs(c.year-sr.year) <= autoplayYears {
				return true
			}
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// sortStableDesc orders s by key, highest first, keeping the order of equals.
func sortStableDesc[T any](s []T, key func(T) int) {
	slices.SortStableFunc(s, func(a, b T) int { return key(b) - key(a) })
}

// trackKinds returns the kind ("music" or "episode") of those of ids the
// viewer can see.
func trackKinds(v Viewer, ids []string) (map[string]string, error) {
	kinds := map[string]string{}
	if len(ids) == 0 {
		return kinds, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := DB.Query(`SELECT t.uuid, t.kind FROM tracks t WHERE t.uuid IN (`+placeholders(len(ids))+`) AND `+v.trackCond("t")+`;`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, kind string
		if err := rows.Scan(&id, &kind); err != nil {
			return nil, err
		}
		kinds[id] = kind
	}
	return kinds, rows.Err()
}

// autoplayProfiles loads the profile of every playable music track the
// viewer can see, by track ID.
func autoplayProfiles(v Viewer) (map[string]*profile, error) {
	profiles := map[string]*profile{}
	rows, err := DB.Query(`SELECT t.uuid, t.secret FROM tracks t WHERE ` + libraryTracks + ` AND ` + v.trackCond("t") + `;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		p := &profile{artists: map[string]bool{}, tags: map[string]bool{}}
		if err := rows.Scan(&id, &p.secret); err != nil {
			return nil, err
		}
		profiles[id] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	each := func(query string, f func(p *profile, a, b string)) error {
		rs, err := DB.Query(query)
		if err != nil {
			return err
		}
		defer rs.Close()
		for rs.Next() {
			var id, a, b string
			if err := rs.Scan(&id, &a, &b); err != nil {
				return err
			}
			if p := profiles[id]; p != nil {
				f(p, a, b)
			}
		}
		return rs.Err()
	}
	if err := each(`SELECT track_id, artist_id, '' FROM track_artist_map;`, func(p *profile, a, _ string) { p.artists[a] = true }); err != nil {
		return nil, err
	}
	if err := each(`SELECT rt.track_id, r.type, COALESCE(substr(r.release_date, 1, 4), '') FROM release_tracks rt JOIN releases r ON r.uuid = rt.release_id;`, func(p *profile, typ, year string) {
		y, _ := strconv.Atoi(year)
		p.releases = append(p.releases, releaseFacts{audio.ReleaseType(typ), y})
	}); err != nil {
		return nil, err
	}
	if err := each(`SELECT track_id, lower(tag), '' FROM track_tags;`, func(p *profile, tag, _ string) { p.tags[tag] = true }); err != nil {
		return nil, err
	}
	return profiles, nil
}

// relatedArtists returns the artists, not in artists themselves, who share a
// release with one of them, either as release artists or through the tracks
// on it.
func relatedArtists(artists map[string]bool) (map[string]bool, error) {
	related := map[string]bool{}
	if len(artists) == 0 {
		return related, nil
	}
	rows, err := DB.Query(`
		WITH ra(release_id, artist_id) AS (
			SELECT release_id, artist_id FROM release_artist_map
			UNION
			SELECT rt.release_id, m.artist_id FROM release_tracks rt JOIN track_artist_map m ON m.track_id = rt.track_id
		)
		SELECT DISTINCT a.artist_id, b.artist_id FROM ra a JOIN ra b ON a.release_id = b.release_id WHERE a.artist_id != b.artist_id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var seedSide, other string
		if err := rows.Scan(&seedSide, &other); err != nil {
			return nil, err
		}
		if artists[seedSide] && !artists[other] {
			related[other] = true
		}
	}
	return related, rows.Err()
}

// tracksByIDs loads the tracks with their artists, releases, covers and tags,
// in the order of ids.
func tracksByIDs(v Viewer, ids []string) ([]*audio.Track, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := DB.Query(selectTrack+` WHERE t.uuid IN (`+placeholders(len(ids))+`) AND `+v.trackCond("t")+`;`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]*audio.Track{}
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		byID[t.ID.String()] = t
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	tracks := make([]*audio.Track, 0, len(ids))
	for _, id := range ids {
		if t := byID[id]; t != nil {
			tracks = append(tracks, t)
		}
	}
	if err := attachArtists(tracks); err != nil {
		return nil, err
	}
	if err := attachReleases(tracks); err != nil {
		return nil, err
	}
	if err := attachCovers(v, tracks); err != nil {
		return nil, err
	}
	if err := attachTags(tracks); err != nil {
		return nil, err
	}
	return tracks, nil
}
