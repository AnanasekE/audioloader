package mb

import "strings"

// The types mirror the parts of MusicBrainz's JSON that we use.

type Artist struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	SortName       string `json:"sort-name"`
	Disambiguation string `json:"disambiguation"`
	Country        string `json:"country"`
	Type           string `json:"type"`
	Score          int    `json:"score"` // search results only
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
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Length           int       `json:"length"` // milliseconds
	ArtistCredit     Credits   `json:"artist-credit"`
	Releases         []Release `json:"releases"`
	ISRCs            []string  `json:"isrcs"`
	FirstReleaseDate string    `json:"first-release-date"`
	Score            int       `json:"score"` // search results only
}
