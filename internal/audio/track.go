package audio

import (
	"net/url"
	"time"
	"uuid"
)

type Identifiable interface{}

type Track struct {
	ID       uuid.UUID
	Title    string
	Duration time.Duration
	Artists  []*Artist
	ImgURL   *url.URL
}

type Artist struct {
	ID     uuid.UUID
	Name   string
	ImgURL *url.URL
}

type Album struct {
	ID     uuid.UUID
	Name   string
	Songs  []*Track
	ImgURL *url.URL
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
	ImgURL *url.URL
}
