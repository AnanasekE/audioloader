package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"uuid"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
)

// releaseDate accepts "YYYY", "YYYY-MM" and "YYYY-MM-DD".
var releaseDate = regexp.MustCompile(`^\d{4}(-(0[1-9]|1[0-2])(-(0[1-9]|[12]\d|3[01]))?)?$`)

func getReleasesPage(w http.ResponseWriter, r *http.Request) {
	releases, err := db.ListReleases(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.ReleasesPage(releases, artists))
}

func getReleasePage(w http.ResponseWriter, r *http.Request) {
	release, ok := loadRelease(w, r)
	if !ok {
		return
	}
	render(w, r, components.ReleasePage(release))
}

func getReleaseEditPage(w http.ResponseWriter, r *http.Request) {
	release, ok := loadRelease(w, r)
	if !ok {
		return
	}
	tracks, err := db.ListTracks(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.ReleaseEditPage(release, tracks, artists))
}

func createRelease(w http.ResponseWriter, r *http.Request) {
	title := strings.TrimSpace(r.FormValue("title"))
	typ := audio.ReleaseType(r.FormValue("type"))
	date := strings.TrimSpace(r.FormValue("date"))
	// FormValue has parsed the form; artist_id repeats once per selected artist
	artistIDs := r.Form["artist_id"]

	if msg := validateRelease(title, typ, date, artistIDs); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	release, err := db.CreateRelease(title, typ, date, artistIDs)
	if errors.Is(err, db.ErrBadReference) {
		http.Error(w, "unknown artist", http.StatusBadRequest)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	// the upload page creates albums first, then continues to batch upload their tracks
	if r.FormValue("next") == "batch" {
		hxLocation(w, "/upload/batch?release="+release.ID.String())
		return
	}
	hxLocation(w, "/releases/"+release.ID.String())
}

// createReleaseJSON is createRelease for the library importer: it takes and
// returns JSON instead of a form and an HX-Location.
func createReleaseJSON(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title     string            `json:"title"`
		Type      audio.ReleaseType `json:"type"`
		Date      string            `json:"date"`
		ArtistIDs []string          `json:"artist_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	req.Title, req.Date = strings.TrimSpace(req.Title), strings.TrimSpace(req.Date)
	if msg := validateRelease(req.Title, req.Type, req.Date, req.ArtistIDs); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	release, err := db.CreateRelease(req.Title, req.Type, req.Date, req.ArtistIDs)
	if errors.Is(err, db.ErrBadReference) {
		http.Error(w, "unknown artist", http.StatusBadRequest)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": release.ID.String()})
}

// validateRelease returns a message for the first invalid field, or "".
func validateRelease(title string, typ audio.ReleaseType, date string, artistIDs []string) string {
	switch {
	case title == "":
		return "title is required"
	case !typ.Valid():
		return "invalid release type"
	case date != "" && !releaseDate.MatchString(date):
		return "date must be YYYY, YYYY-MM or YYYY-MM-DD"
	case len(artistIDs) == 0:
		return "pick at least one artist"
	}
	for _, id := range artistIDs {
		if _, err := uuid.Parse(id); err != nil {
			return "invalid artist id"
		}
	}
	return ""
}

func updateRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "release_id")
	if !ok || !visibleRelease(w, r, id.String()) {
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	typ := audio.ReleaseType(r.FormValue("type"))
	date := strings.TrimSpace(r.FormValue("date"))
	artistIDs := r.Form["artist_id"]
	if msg := validateRelease(title, typ, date, artistIDs); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	err := db.UpdateRelease(id.String(), title, typ, date, artistIDs)
	switch {
	case errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, db.ErrBadReference):
		http.Error(w, "unknown artist", http.StatusBadRequest)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/releases/"+id.String())
}

func deleteRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "release_id")
	if !ok || !visibleRelease(w, r, id.String()) {
		return
	}
	err := db.DeleteRelease(id.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/releases")
}

func saveReleaseTracks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "release_id")
	if !ok || !visibleRelease(w, r, id.String()) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	// track_id and disc repeat once per row, in tracklist order; every row
	// sends both, so the two lists line up
	trackIDs, discs := r.PostForm["track_id"], r.PostForm["disc"]
	if len(trackIDs) != len(discs) {
		http.Error(w, "every track needs a disc", http.StatusBadRequest)
		return
	}
	entries := make([]db.ReleaseTrackInput, len(trackIDs))
	for i := range trackIDs {
		if _, err := uuid.Parse(trackIDs[i]); err != nil {
			http.Error(w, "invalid track id", http.StatusBadRequest)
			return
		}
		disc, err := strconv.Atoi(discs[i])
		if err != nil || disc < 1 {
			http.Error(w, "disc must be a number from 1", http.StatusBadRequest)
			return
		}
		entries[i] = db.ReleaseTrackInput{TrackID: trackIDs[i], Disc: disc}
	}

	err := db.SetReleaseTracks(viewer(r), id.String(), entries)
	switch {
	case errors.Is(err, db.ErrAlreadyOnRelease):
		http.Error(w, "a track is listed twice", http.StatusBadRequest)
		return
	case errors.Is(err, db.ErrBadReference):
		http.Error(w, "unknown release or track", http.StatusBadRequest)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/releases/"+id.String())
}

// loadRelease reads {release_id} and loads the release, writing 400/404/500 on failure.
func loadRelease(w http.ResponseWriter, r *http.Request) (*audio.Release, bool) {
	id, ok := pathUUID(w, r, "release_id")
	if !ok {
		return nil, false
	}
	release, err := db.GetRelease(viewer(r), id.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return nil, false
	}
	if err != nil {
		serverError(w, err)
		return nil, false
	}
	return release, true
}

// hxLocation navigates the browser to path through htmx, without a full
// page load (an HX-Redirect would reload and stop the player).
func hxLocation(w http.ResponseWriter, path string) {
	loc, _ := json.Marshal(map[string]string{"path": path, "target": "body"})
	w.Header().Set("HX-Location", string(loc))
	w.WriteHeader(http.StatusNoContent)
}

// visibleRelease writes a 404 and returns false if the viewer can't see the release.
func visibleRelease(w http.ResponseWriter, r *http.Request, id string) bool {
	_, err := db.GetRelease(viewer(r), id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return false
	}
	if err != nil {
		serverError(w, err)
		return false
	}
	return true
}
