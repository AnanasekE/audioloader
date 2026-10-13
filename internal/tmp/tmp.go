package tmp

type Track struct {
	Title         string
	LengthSeconds int
	Artists       []*Artist
}

type Artist struct {
	Name string
}

type Album struct {
	Songs []*Track
}

type Release struct {
	Track *Track // non-nil for singles
	Album *Album // non-nil for albums
}

func (r Release) IsSingle() bool { return r.Track != nil }
