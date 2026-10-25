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
}

type Artist struct {
	ID     uuid.UUID
	Name   string
	ImgURL string
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
	Tracks []*Track
	ImgURL string
}
