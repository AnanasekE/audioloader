package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"audioloader/internal/components"
	"audioloader/internal/db"
)

func createArtist(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	artist, err := db.AddArtist(name)
	if errors.Is(err, db.ErrArtistExists) {
		renderStatus(w, r, http.StatusConflict, components.Notice(name+" is already in the library."))
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}

	// the event carries the new artist, so pages can add it without refetching
	trigger, err := json.Marshal(map[string]any{
		"artistsChanged": map[string]string{"id": artist.ID.String(), "name": artist.Name},
	})
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("HX-Trigger", string(trigger))
	render(w, r, components.Notice("Added "+name+"."))
}

func getArtists(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.ArtistList(artists))
}

// resolveArtists takes {"names": [...]} and answers {"name": "id", ...},
// creating artists that don't exist yet. Used by the library importer.
func resolveArtists(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	names := make([]string, 0, len(req.Names))
	for _, n := range req.Names {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}

	ids, err := db.ResolveArtists(names)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ids)
}
