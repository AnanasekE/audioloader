package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/trim"
)

// trimTimeout covers detecting and cutting a long episode.
const trimTimeout = 5 * time.Minute

// previewTrim says how much silence the end of a track's file has. It is
// swapped into the track menu in place of the "Trim silence…" button.
func previewTrim(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok || !visibleTrack(w, r, id.String()) {
		return
	}
	t, err := db.GetTrack(viewer(r), id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	c, err := trim.Default()
	if err != nil {
		render(w, r, components.TrimPreview(t, trim.Plan{}, err.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), trimTimeout)
	defer cancel()
	plan, err := c.Preview(ctx, id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		render(w, r, components.TrimPreview(t, trim.Plan{}, "couldn't read the file: "+err.Error()))
		return
	}
	render(w, r, components.TrimPreview(t, plan, ""))
}

// applyTrim cuts the silence off the end of a track's file.
func applyTrim(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok || !visibleTrack(w, r, id.String()) {
		return
	}
	t, err := db.GetTrack(viewer(r), id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	c, err := trim.Default()
	if err != nil {
		render(w, r, components.TrimPreview(t, trim.Plan{}, err.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), trimTimeout)
	defer cancel()
	if _, err := c.Apply(ctx, id); errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "couldn't trim the file: "+err.Error(), http.StatusBadGateway)
		return
	}
	hxLocation(w, backTo(r, "/tracks"))
}
