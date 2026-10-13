package tmp

import "time"

type Track struct {
	Title    string
	Duration time.Duration
	Artists  []*Artist
}

type Artist struct {
	Name string
}

type Album struct {
	Songs []*Track
}

type Release struct {
	Date  time.Time // Should a release contain the date or the album + track independently?
	Track *Track    // non-nil for singles
	Album *Album    // non-nil for albums
}

func (r Release) IsSingle() bool { return r.Track != nil }

type Playlist struct {
	Tracks []*Track
}
