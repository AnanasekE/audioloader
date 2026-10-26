package mb

import (
	"regexp"
	"strings"
	"uuid"
)

// The types mirror the parts of MusicBrainz's JSON that we use.

type Artist struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	SortName       string   `json:"sort-name"`
	Disambiguation string   `json:"disambiguation"`
	Country        string   `json:"country"`
	Type           string   `json:"type"`
	Score          int      `json:"score"` // search results only
	Area           Area     `json:"area"`
	LifeSpan       LifeSpan `json:"life-span"`
}

// Area is where an artist is from, e.g. a country or a city.
type Area struct {
	Name string `json:"name"`
}

// LifeSpan is when an artist was active; the dates are "YYYY", "YYYY-MM" or
// "YYYY-MM-DD", and may be empty when unknown.
type LifeSpan struct {
	Begin string `json:"begin"`
	End   string `json:"end"`
	Ended bool   `json:"ended"`
}

type ArtistCredit struct {
	Name       string `json:"name"`
	JoinPhrase string `json:"joinphrase"`
	Artist     Artist `json:"artist"`
}

// Credits is an artist credit list, e.g. "A feat. B".
type Credits []ArtistCredit

func (cs Credits) String() string {
	var b strings.Builder
	for _, c := range cs {
		b.WriteString(c.Name)
		b.WriteString(c.JoinPhrase)
	}
	return b.String()
}

type ReleaseGroup struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	PrimaryType      string   `json:"primary-type"`
	SecondaryTypes   []string `json:"secondary-types"`
	FirstReleaseDate string   `json:"first-release-date"`
	ArtistCredit     Credits  `json:"artist-credit"`
}

type Release struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Status       string       `json:"status"`
	Date         string       `json:"date"`
	Country      string       `json:"country"`
	ArtistCredit Credits      `json:"artist-credit"`
	ReleaseGroup ReleaseGroup `json:"release-group"`
	Media        []Medium     `json:"media"`
	TrackCount   int          `json:"track-count"`
	Relations    []Relation   `json:"relations"`
}

// Relation is a link from an entity to something else: a URL, or (for
// covers) a work or a recording.
type Relation struct {
	Type       string   `json:"type"`        // e.g. "free streaming", "purchase for download", "performance"
	TargetType string   `json:"target-type"` // "url", "work", "recording", ...
	Direction  string   `json:"direction"`   // "forward" or "backward", relative to the entity it hangs on
	Attributes []string `json:"attributes"`  // e.g. "cover", "live"
	URL        struct {
		Resource string `json:"resource"`
	} `json:"url"`
	Work      *Work      `json:"work"`      // set when TargetType is "work"
	Recording *Recording `json:"recording"` // set when TargetType is "recording"
}

// HasAttribute reports whether the relation carries the attribute, e.g. "cover".
func (r Relation) HasAttribute(name string) bool {
	for _, a := range r.Attributes {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}

// Work is a composition. Recordings that perform it hang on it as relations.
type Work struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Relations []Relation `json:"relations"`
}

type Medium struct {
	Position int     `json:"position"`
	Format   string  `json:"format"`
	Tracks   []Track `json:"tracks"`
}

type Track struct {
	ID        string    `json:"id"`
	Position  int       `json:"position"`
	Number    string    `json:"number"`
	Title     string    `json:"title"`
	Length    int       `json:"length"` // milliseconds
	Recording Recording `json:"recording"`
}

type Recording struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Length           int        `json:"length"` // milliseconds
	ArtistCredit     Credits    `json:"artist-credit"`
	Releases         []Release  `json:"releases"`
	ISRCs            []string   `json:"isrcs"`
	FirstReleaseDate string     `json:"first-release-date"`
	Relations        []Relation `json:"relations"`
	Score            int        `json:"score"` // search results only
}

var recordingURL = regexp.MustCompile(`(?i)^(?:https?://)?(?:[a-z]+\.)?musicbrainz\.org/recording/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:[/?#].*)?$`)

// RecordingID extracts the recording ID from a MusicBrainz recording URL,
// e.g. https://musicbrainz.org/recording/<id>. A bare ID is accepted too.
// ok is false for anything else.
func RecordingID(s string) (id string, ok bool) {
	s = strings.TrimSpace(s)
	if m := recordingURL.FindStringSubmatch(s); m != nil {
		return strings.ToLower(m[1]), true
	}
	if _, err := uuid.Parse(s); err == nil {
		return strings.ToLower(s), true
	}
	return "", false
}

// CoverWork returns the work this recording is a cover of: the target of a
// "performance" relation with the "cover" attribute. nil if it isn't a cover
// (or the lookup didn't include work-rels).
func (r *Recording) CoverWork() *Work {
	for _, rel := range r.Relations {
		if rel.Type == "performance" && rel.TargetType == "work" && rel.Work != nil && rel.HasAttribute("cover") {
			return rel.Work
		}
	}
	return nil
}

// notOriginal are performance attributes of recordings that can't stand for the
// original song: other covers and partial, live, medley or karaoke versions.
var notOriginal = []string{"cover", "live", "partial", "medley", "karaoke"}

// Original picks the original among the recordings that perform the work: the
// one released first that isn't one of notOriginal, and isn't excludeID (the
// cover being looked up). Recordings without a date come last. nil if none.
func (w *Work) Original(excludeID string) *Recording {
	var best *Recording
	for _, rel := range w.Relations {
		if rel.Type != "performance" || rel.TargetType != "recording" || rel.Recording == nil || rel.Recording.ID == excludeID {
			continue
		}
		skip := false
		for _, a := range notOriginal {
			skip = skip || rel.HasAttribute(a)
		}
		if skip {
			continue
		}
		rec := rel.Recording
		if best == nil || rec.FirstReleaseDate != "" && (best.FirstReleaseDate == "" || rec.FirstReleaseDate < best.FirstReleaseDate) {
			best = rec
		}
	}
	return best
}
