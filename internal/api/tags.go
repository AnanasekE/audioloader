package api

import (
	"net/http"

	"audioloader/internal/components"
	"audioloader/internal/db"
)

// tagQuery reads the tag lens state from the URL:
// ?in=ambient&in=rain&ex=lofi&match=any&sort=creator
func tagQuery(r *http.Request) db.TagQuery {
	p := r.URL.Query()
	q := db.TagQuery{
		Include: db.NormalizeTags(p["in"]),
		Exclude: db.NormalizeTags(p["ex"]),
		Any:     p.Get("match") == "any",
	}
	switch s := p.Get("sort"); s {
	case "creator", "tag":
		q.Sort = s
	}
	return q
}

// getTagsPage is the tag lens. A swap of the lens region (an htmx request
// that isn't a boosted page visit or a history restore) gets just that
// region.
func getTagsPage(w http.ResponseWriter, r *http.Request) {
	q := tagQuery(r)
	lens := components.TagLens{Query: q}
	var err error
	if lens.Counts, err = db.TrackTags(viewer(r), q); err != nil {
		serverError(w, err)
		return
	}
	if lens.All, err = db.TrackTags(viewer(r), db.TagQuery{}); err != nil {
		serverError(w, err)
		return
	}
	if lens.Tracks, err = db.TracksByTags(viewer(r), q); err != nil {
		serverError(w, err)
		return
	}
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") == "" && r.Header.Get("HX-History-Restore-Request") == "" {
		render(w, r, components.TagLensRegion(lens))
		return
	}
	render(w, r, components.TagsPage(lens))
}
