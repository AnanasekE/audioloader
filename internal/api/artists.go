package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"uuid"

	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/mb"
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

func getArtistPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok {
		return
	}
	artist, err := db.GetArtist(viewer(r), id.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	releases, err := db.ListArtistReleases(viewer(r), id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	tracks, err := db.ListArtistTracks(viewer(r), id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	others, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.ArtistPage(artist, releases, tracks, others))
}

func renameArtist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok || !visibleArtist(w, r, id.String()) {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	err := db.RenameArtist(id.String(), name)
	switch {
	case errors.Is(err, db.ErrArtistExists):
		http.Error(w, name+" is already in the library.", http.StatusConflict)
		return
	case errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/artists/"+id.String())
}

// mergeArtist folds the artist into the one named by "into".
func mergeArtist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok || !visibleArtist(w, r, id.String()) {
		return
	}
	into, err := uuid.Parse(r.FormValue("into"))
	if err != nil {
		http.Error(w, "pick the artist to merge into", http.StatusBadRequest)
		return
	}
	if !visibleArtist(w, r, into.String()) {
		return
	}
	err = db.MergeArtists(id.String(), into.String())
	switch {
	case errors.Is(err, db.ErrBadMerge):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/artists/"+into.String())
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

// getCandidateReleases answers a candidate's Expand button with that
// MusicBrainz artist's newest release groups. It reads nothing from the
// library, so any viewer may ask.
func getCandidateReleases(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "mbid")
	if !ok {
		return
	}
	groups, err := mb.Default().RecentReleaseGroups(r.Context(), id.String(), 5)
	if err != nil {
		log.Println("browse release groups:", err)
	}
	render(w, r, components.CandidateReleases(groups, err != nil))
}

// getArtistLink searches MusicBrainz for candidates for an artist without a MusicBrainz ID.
func getArtistLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok || !visibleArtist(w, r, id.String()) {
		return
	}
	a, err := db.GetArtist(viewer(r), id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	if a.MBID != "" {
		http.Error(w, a.Name+" is already linked.", http.StatusConflict)
		return
	}
	candidates, err := mb.Default().SearchArtists(r.Context(), a.Name, 5)
	searchErr := ""
	if err != nil {
		log.Println("search artists:", err)
		searchErr = "Couldn't search MusicBrainz; paste the artist's ID or URL below instead."
	}
	render(w, r, components.ArtistLinkCandidates(a, candidates, searchErr))
}

// linkArtist sets the artist's MusicBrainz ID from a pick ("mbid") or a
// pasted ID or URL ("mbid_manual"). If another artist has it, it offers a
// merge instead.
func linkArtist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok || !visibleArtist(w, r, id.String()) {
		return
	}
	a, err := db.GetArtist(viewer(r), id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	if a.MBID != "" {
		http.Error(w, a.Name+" is already linked.", http.StatusConflict)
		return
	}
	mbid := strings.ToLower(validMBID(r.FormValue("mbid")))
	if manual := r.FormValue("mbid_manual"); strings.TrimSpace(manual) != "" {
		if mbid = parseArtistMBID(manual); mbid == "" {
			http.Error(w, "that isn't a MusicBrainz artist ID or URL", http.StatusBadRequest)
			return
		}
	}
	if mbid == "" {
		http.Error(w, "pick the artist on MusicBrainz", http.StatusBadRequest)
		return
	}

	err = db.SetArtistMBID(id.String(), mbid)
	if errors.Is(err, db.ErrMBIDTaken) {
		other, err := db.ArtistByMBID(viewer(r), mbid)
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			serverError(w, err)
			return
		}
		render(w, r, components.ArtistLinkTaken(a, other))
		return
	}
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/artists/"+id.String())
}
