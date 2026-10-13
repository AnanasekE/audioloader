package tmp

type Song struct {
	Title   string
	Album   string
	Artists []*Artist
}

type Artist struct {
	Name string
}
