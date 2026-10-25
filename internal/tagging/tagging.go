// Package tagging matches library tracks to MusicBrainz in the background:
// first by MusicBrainz IDs in the file's tags, then by AcoustID fingerprint,
// then by text search. Good matches are applied, uncertain ones wait on the
// review page. Tags are never written into files: that would change their
// fingerprint and break duplicate detection.
package tagging

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/jobs"
	"audioloader/internal/match"
	"audioloader/internal/mb"
	"audioloader/internal/tags"
)

// Job types.
const (
	JobTag   = "tag_track"
	JobCover = "fetch_cover"
)

// Score thresholds: at or above AutoApply a match is applied without asking;
// at or above Review it waits for a decision; below it counts as no match.
const (
	AutoApply = 0.9
	Review    = 0.5
)

// Skip lists checks for tracks that are never auto-tagged: placeholders
// have no file to identify, podcast episodes aren't on MusicBrainz, and
// secret tracks aren't sent to anyone.
var Skip = []func(t *audio.Track) bool{
	func(t *audio.Track) bool { return !t.HasFile },
	func(t *audio.Track) bool { return t.IsEpisode() },
	func(t *audio.Track) bool { return t.Secret },
}

// Tagger holds the services the pipeline uses; tests swap them.
type Tagger struct {
	MB *mb.Client
	// FPCalc is the Chromaprint fpcalc binary; "" disables AcoustID.
	FPCalc string
}

var (
	defaultOnce sync.Once
	defaultT    *Tagger
)

// Default is the tagger the jobs use, configured from the environment
// (FPCALC, ACOUSTID_KEY, AUDIOLOADER_CONTACT).
func Default() *Tagger {
	defaultOnce.Do(func() {
		defaultT = &Tagger{MB: mb.Default(), FPCalc: findFPCalc()}
	})
	return defaultT
}

func findFPCalc() string {
	name := os.Getenv("FPCALC")
	if name == "" {
		name = "fpcalc"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		log.Printf("tagging: %s not found, AcoustID lookups are off", name)
		return ""
	}
	return path
}

// Register sets up the job handlers.
func Register() {
	jobs.Register(JobTag, func(ctx context.Context, payload json.RawMessage) error {
		var p struct{ TrackID string }
		if err := json.Unmarshal(payload, &p); err != nil {
			return jobs.Permanent(err)
		}
		return Default().Tag(ctx, p.TrackID)
	})
	jobs.Register(JobCover, func(ctx context.Context, payload json.RawMessage) error {
		var p struct{ ReleaseID string }
		if err := json.Unmarshal(payload, &p); err != nil {
			return jobs.Permanent(err)
		}
		return Default().FetchCover(ctx, p.ReleaseID)
	})
}

// Enqueue queues a track for auto-tagging, unless it is already queued.
func Enqueue(trackID string) error {
	return jobs.EnqueueAt(JobTag, trackID, map[string]string{"TrackID": trackID}, time.Time{})
}

// EnqueueCover queues fetching a release's cover from the Cover Art Archive.
func EnqueueCover(releaseID string) error {
	return jobs.EnqueueAt(JobCover, releaseID, map[string]string{"ReleaseID": releaseID}, time.Time{})
}

// Tag finds MusicBrainz candidates for a track, stores them and applies the
// best one if it is good enough.
func (tg *Tagger) Tag(ctx context.Context, trackID string) error {
	t, err := db.GetTrack(db.All, trackID)
	if errors.Is(err, db.ErrNotFound) {
		return nil // deleted since it was queued
	}
	if err != nil {
		return err
	}
	for _, skip := range Skip {
		if skip(t) {
			return nil
		}
	}
	rejected, err := db.RejectedRecordings(trackID)
	if err != nil {
		return err
	}
	path, err := db.TrackFilePath(t.ID)
	if err != nil {
		return err
	}

	cands, err := tg.candidates(ctx, t, path)
	if err != nil {
		return err
	}
	cands = keep(cands, func(m db.Match) bool { return !rejected[m.RecordingID] && m.Score >= Review })
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
	if len(cands) > 5 {
		cands = cands[:5]
	}

	switch {
	case len(cands) == 0:
		return db.SaveMatches(trackID, nil, db.TagNone)
	case cands[0].Score >= AutoApply:
		if err := db.SaveMatches(trackID, cands[:1], db.TagReview); err != nil {
			return err
		}
		return tg.apply(trackID, cands[0])
	default:
		return db.SaveMatches(trackID, cands, db.TagReview)
	}
}

// Apply applies a stored candidate, e.g. from the review page.
func Apply(trackID, recordingID string) error {
	m, err := db.GetMatch(trackID, recordingID)
	if err != nil {
		return err
	}
	return Default().apply(trackID, *m)
}

func (tg *Tagger) apply(trackID string, m db.Match) error {
	releaseIDs, err := db.ApplyMatch(trackID, m)
	if err != nil {
		return err
	}
	for _, id := range releaseIDs {
		if err := EnqueueCover(id); err != nil {
			return err
		}
	}
	return nil
}

// candidates runs the pipeline, stopping at the first step that finds a
// match good enough to apply.
func (tg *Tagger) candidates(ctx context.Context, t *audio.Track, path string) ([]db.Match, error) {
	var info tags.Info
	if f, err := os.Open(path); err == nil {
		info, _ = tags.Read(f) // unreadable tags just mean no hints
		f.Close()
	}

	// 1: MusicBrainz IDs from the upload or the file's tags
	if id := firstNonEmpty(t.MBID, info.RecordingID); id != "" {
		rec, err := tg.MB.GetRecording(ctx, id)
		if err != nil && !errors.Is(err, mb.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			m := fromRecording(rec, t, info.ReleaseID, "tags")
			m.Score = 1
			return []db.Match{m}, nil
		}
	}

	var cands []db.Match
	// 2: AcoustID fingerprint
	if tg.FPCalc != "" && tg.MB.AcoustIDKey != "" {
		found, err := tg.acoustID(ctx, t, path)
		if err != nil {
			return nil, err
		}
		cands = append(cands, found...)
		if best(cands) >= AutoApply {
			return cands, nil
		}
	}

	// 3: text search on title and artists
	found, err := tg.search(ctx, t)
	if err != nil {
		return nil, err
	}
	return merge(cands, found), nil
}

func (tg *Tagger) acoustID(ctx context.Context, t *audio.Track, path string) ([]db.Match, error) {
	out, err := exec.CommandContext(ctx, tg.FPCalc, "-json", path).Output()
	if err != nil {
		// a file fpcalc can't decode isn't worth retrying
		log.Printf("tagging: fpcalc %s: %v", t.ID, err)
		return nil, nil
	}
	var fp struct {
		Duration    float64 `json:"duration"`
		Fingerprint string  `json:"fingerprint"`
	}
	if err := json.Unmarshal(out, &fp); err != nil {
		return nil, nil
	}
	results, err := tg.MB.AcoustIDLookup(ctx, fp.Fingerprint, int(fp.Duration))
	if err != nil {
		return nil, err
	}

	var cands []db.Match
	seen := map[string]bool{}
	for _, res := range results {
		if res.Score < Review {
			continue
		}
		for _, r := range res.Recordings {
			// a few lookups at most: each costs a MusicBrainz request
			if seen[r.ID] || len(seen) >= 3 {
				continue
			}
			seen[r.ID] = true
			rec, err := tg.MB.GetRecording(ctx, r.ID)
			if errors.Is(err, mb.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			m := fromRecording(rec, t, "", "acoustid")
			// the fingerprint is strong evidence even when our title is just a file name
			m.Score = 0.6*res.Score + 0.4*metaScore(t, rec, m.ReleaseTitle)
			cands = append(cands, m)
		}
	}
	return cands, nil
}

func (tg *Tagger) search(ctx context.Context, t *audio.Track) ([]db.Match, error) {
	q := "recording:" + mb.Phrase(t.Title)
	if len(t.Artists) > 0 {
		q += " AND artist:" + mb.Phrase(t.Artists[0].Name)
	}
	recs, err := tg.MB.SearchRecordings(ctx, q, 5)
	if err != nil {
		return nil, err
	}
	cands := make([]db.Match, 0, len(recs))
	for i := range recs {
		m := fromRecording(&recs[i], t, "", "search")
		m.Score = metaScore(t, &recs[i], m.ReleaseTitle)
		cands = append(cands, m)
	}
	return cands, nil
}

// metaScore rates how well a recording's metadata fits the track.
func metaScore(t *audio.Track, rec *mb.Recording, releaseTitle string) float64 {
	title := match.Similarity(t.Title, rec.Title)

	artist := 0.0
	for _, a := range t.Artists {
		for _, c := range rec.ArtistCredit {
			artist = max(artist, match.Similarity(a.Name, c.Name), match.Similarity(a.Name, c.Artist.Name))
		}
	}

	duration := 0.5 // unknown length
	if rec.Length > 0 && t.Duration > 0 {
		diff := t.Duration - time.Duration(rec.Length)*time.Millisecond
		if diff < 0 {
			diff = -diff
		}
		switch {
		case diff <= 3*time.Second:
			duration = 1
		case diff <= 10*time.Second:
			duration = 0.5
		default:
			duration = 0
		}
	}

	release := 0.3
	for _, r := range t.Releases {
		if match.Same(r.Title, releaseTitle) {
			release = 1
		}
	}
	return 0.4*title + 0.3*artist + 0.2*duration + 0.1*release
}

// fromRecording builds a candidate from rec on the release that fits the
// track best: hintReleaseID (from tags) if rec is on it, else one with the
// title of a release the track is on, else the earliest official album.
func fromRecording(rec *mb.Recording, t *audio.Track, hintReleaseID, source string) db.Match {
	m := db.Match{
		Source:      source,
		RecordingID: rec.ID,
		Title:       rec.Title,
		LengthMs:    rec.Length,
		Artists:     matchArtists(rec.ArtistCredit),
	}
	if len(rec.ISRCs) > 0 {
		m.ISRC = rec.ISRCs[0]
	}
	if r := pickRelease(rec.Releases, t, hintReleaseID); r != nil {
		m.ReleaseID = r.ID
		m.ReleaseGroupID = r.ReleaseGroup.ID
		m.ReleaseTitle = r.Title
		m.ReleaseType = ReleaseType(r.ReleaseGroup)
		m.ReleaseDate = r.Date
		m.ReleaseArtists = matchArtists(r.ArtistCredit)
	}
	return m
}

func pickRelease(releases []mb.Release, t *audio.Track, hintID string) *mb.Release {
	if len(releases) == 0 {
		return nil
	}
	rank := func(r *mb.Release) int {
		score := 0
		if r.ID == hintID {
			score += 1000
		}
		for _, own := range t.Releases {
			if match.Same(own.Title, r.Title) {
				score += 100
				if string(own.Type) == ReleaseType(r.ReleaseGroup) {
					score += 50
				}
			}
		}
		if r.Status == "Official" {
			score += 10
		}
		if ReleaseType(r.ReleaseGroup) == "album" {
			score += 5
		}
		return score
	}
	best := &releases[0]
	for i := range releases {
		r := &releases[i]
		rb, rr := rank(best), rank(r)
		// ties go to the earliest date; an unknown date sorts last
		if rr > rb || rr == rb && earlier(r.Date, best.Date) {
			best = r
		}
	}
	return best
}

func earlier(a, b string) bool {
	if a == "" {
		return false
	}
	return b == "" || a < b
}

// ReleaseType maps a MusicBrainz release group to our release type, or ""
// for types we don't model (broadcasts, "other").
func ReleaseType(rg mb.ReleaseGroup) string {
	for _, s := range rg.SecondaryTypes {
		if s == "Compilation" {
			return "compilation"
		}
	}
	switch rg.PrimaryType {
	case "Album":
		return "album"
	case "EP":
		return "ep"
	case "Single":
		return "single"
	}
	return ""
}

func matchArtists(credits mb.Credits) []db.MatchArtist {
	out := make([]db.MatchArtist, 0, len(credits))
	for _, c := range credits {
		name := c.Artist.Name
		if name == "" {
			name = c.Name
		}
		out = append(out, db.MatchArtist{ID: c.Artist.ID, Name: name})
	}
	return out
}

// FetchCover downloads a release's front cover from the Cover Art Archive,
// unless it already has one.
func (tg *Tagger) FetchCover(ctx context.Context, releaseID string) error {
	r, err := db.GetRelease(db.All, releaseID)
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if r.ImgURL != db.DefaultCover || r.MBID == "" {
		return nil
	}
	body, _, err := tg.MB.FrontCover(ctx, "release", r.MBID)
	if errors.Is(err, mb.ErrNotFound) && r.ReleaseGroupMBID != "" {
		body, _, err = tg.MB.FrontCover(ctx, "release-group", r.ReleaseGroupMBID)
	}
	if errors.Is(err, mb.ErrNotFound) {
		return nil // no cover; nothing to retry
	}
	if err != nil {
		return err
	}
	defer body.Close()
	return db.SaveCoverImage(r.ID, body)
}

func best(ms []db.Match) float64 {
	b := 0.0
	for _, m := range ms {
		b = max(b, m.Score)
	}
	return b
}

// merge adds b's candidates to a, keeping the higher score per recording.
func merge(a, b []db.Match) []db.Match {
	idx := map[string]int{}
	for i, m := range a {
		idx[m.RecordingID] = i
	}
	for _, m := range b {
		if i, ok := idx[m.RecordingID]; ok {
			if m.Score > a[i].Score {
				a[i] = m
			}
			continue
		}
		idx[m.RecordingID] = len(a)
		a = append(a, m)
	}
	return a
}

func keep(ms []db.Match, f func(db.Match) bool) []db.Match {
	out := ms[:0]
	for _, m := range ms {
		if f(m) {
			out = append(out, m)
		}
	}
	return out
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}
