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
}

type Artist struct {
	ID     uuid.UUID
	Name   string
	ImgURL string
}

type Album struct {
	ID     uuid.UUID
	Name   string
	Songs  []*Track
	ImgURL string
}

type Release struct {
	ID    uuid.UUID
	Date  time.Time // Should a release contain the date or the album + track independently?
	Track *Track    // non-nil for singles
	Album *Album    // non-nil for albums
}

func (r Release) IsSingle() bool { return r.Track != nil }

type Playlist struct {
	ID     uuid.UUID
	Name   string
	Tracks []*Track
	ImgURL string
}
