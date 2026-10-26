package audio

import (
	"time"
	"uuid"
)

type Identifiable interface{}

type Track struct {
	ID       uuid.UUID
	Title    string
	Duration time.Duration
	Artists  []*Artist
	ImgURL   string
	Releases []*Release // without their Tracks
	MBID     string     // MusicBrainz recording ID, "" if unknown
	ISRC     string
	// Kind is KindMusic or KindEpisode. An episode belongs to a show
	// (ShowID) instead of releases, and was published on Published.
	Kind      string
	ShowID    string
	Published string
	// Position is where playback of an episode stopped; Finished marks it played
	Position time.Duration
	Finished bool
	// Secret tracks are only shown once the vault is unlocked
	Secret bool
	// CoverOf is the original song this track covers, nil if none or if it
	// is a secret track the viewer can't see
	CoverOf *CoverRef
	// false for a placeholder: the track is known (e.g. from a release's
	// tracklist) but its file hasn't been added yet
	HasFile bool
}

// CoverRef points a cover to its original, as a reference only: the
// original's artists are text and never become credits of the cover.
type CoverRef struct {
	Title   string
	Artists string // display text, e.g. "A & B"; may be empty
	// TrackID is the original in the library; ReleaseID is the release to
	// link to for it ("" if it is on none). Both are empty for a text
	// reference, which may carry the MusicBrainz recording ID instead.
	TrackID   string
	ReleaseID string
	MBID      string
}

// String is "Title by Artists", for places that only show text.
func (c *CoverRef) String() string {
	if c.Artists == "" {
		return c.Title
	}
	return c.Title + " by " + c.Artists
}

// Track kinds.
const (
	KindMusic   = "music"
	KindEpisode = "episode"
)

func (t *Track) IsEpisode() bool { return t.Kind == KindEpisode }

// Show is a podcast; its episodes are tracks of kind KindEpisode.
type Show struct {
	ID       uuid.UUID
	Title    string
	Author   string
	ImgURL   string
	Episodes []*Track // newest first
	// set by listings, which leave Episodes empty
	Count    int
	Unplayed int
}

type Artist struct {
	ID     uuid.UUID
	Name   string
	ImgURL string
	MBID   string // MusicBrainz artist ID, "" if unknown
	// TwitchLogin is the lowercase login of the artist's Twitch channel, "" if none
	TwitchLogin string
	// Followed is set by artist listings: new releases are tracked
	Followed bool
}

type ReleaseType string

const (
	ReleaseAlbum       ReleaseType = "album"
	ReleaseEP          ReleaseType = "ep"
	ReleaseSingle      ReleaseType = "single"
	ReleaseCompilation ReleaseType = "compilation"
)

// ReleaseTypes is the display order, e.g. for the sections of the releases page.
var ReleaseTypes = []ReleaseType{ReleaseAlbum, ReleaseEP, ReleaseSingle, ReleaseCompilation}

func (t ReleaseType) Valid() bool {
	for _, v := range ReleaseTypes {
		if t == v {
			return true
		}
	}
	return false
}

func (t ReleaseType) Label() string {
	switch t {
	case ReleaseAlbum:
		return "Album"
	case ReleaseEP:
		return "EP"
	case ReleaseSingle:
		return "Single"
	case ReleaseCompilation:
		return "Compilation"
	}
	return string(t)
}

// Release is an album, EP, single or compilation. The type is always set
// explicitly, never derived from the track count: a single can have B-sides
// and an album can be one long track.
type Release struct {
	ID    uuid.UUID
	Title string
	Type  ReleaseType
	// "YYYY", "YYYY-MM" or "YYYY-MM-DD": often only the year is known,
	// and a full timestamp would invent a month and day
	Date    string
	ImgURL  string
	Artists []*Artist
	Tracks  []*ReleaseTrack // ordered by disc, then position
	// MusicBrainz release and release group IDs, "" if unknown
	MBID             string
	ReleaseGroupMBID string
}

type ReleaseTrack struct {
	Disc     int
	Position int
	Track    *Track
}

func (r *Release) IsSingle() bool { return r.Type == ReleaseSingle }

func (r *Release) Year() string {
	if len(r.Date) >= 4 {
		return r.Date[:4]
	}
	return ""
}

func (r *Release) Duration() time.Duration {
	var d time.Duration
	for _, rt := range r.Tracks {
		d += rt.Track.Duration
	}
	return d
}

// Discs groups Tracks by disc, in order.
func (r *Release) Discs() [][]*ReleaseTrack {
	var discs [][]*ReleaseTrack
	for i, rt := range r.Tracks {
		if i == 0 || rt.Disc != r.Tracks[i-1].Disc {
			discs = append(discs, nil)
		}
		discs[len(discs)-1] = append(discs[len(discs)-1], rt)
	}
	return discs
}

type Playlist struct {
	ID     uuid.UUID
	Name   string
	Tracks []*Track // in order; may repeat a track
	ImgURL string   // the first track's image
	// set by listings, which leave Tracks empty
	Count    int
	Duration time.Duration
}
