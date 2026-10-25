package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
)

func getShowsPage(w http.ResponseWriter, r *http.Request) {
	shows, err := db.ListShows()
	if err != nil {
		serverError(w, err)
		return
	}
	cont, err := db.ContinueListening(10)
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.ShowsPage(shows, cont))
}

func getShowPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "show_id")
	if !ok {
		return
	}
	s, err := db.GetShow(id.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.ShowPage(s))
}

func addEpisode(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("track")
	if err != nil {
		http.Error(w, "missing or invalid audio file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	duration, err := audio.ProbeDuration(file)
	if err != nil {
		http.Error(w, "could not read audio duration: "+err.Error(), http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	published := strings.TrimSpace(r.FormValue("published"))
	if published != "" && !releaseDate.MatchString(published) {
		http.Error(w, "date must be YYYY, YYYY-MM or YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	showID := r.FormValue("show_id")
	if showID == "" {
		show, err := db.ResolveShow(r.FormValue("show_name"), "")
		if err != nil {
			http.Error(w, "pick a show or name a new one", http.StatusBadRequest)
			return
		}
		showID = show.ID.String()
	} else if validMBID(showID) == "" { // checks for a UUID
		http.Error(w, "invalid show id", http.StatusBadRequest)
		return
	}

	t := &audio.Track{Title: title, Duration: duration.Round(time.Second), Published: published}
	id, err := db.AddEpisode(showID, t, file, filepath.Ext(header.Filename))
	switch {
	case errors.Is(err, db.ErrExists):
		http.Error(w, "already exists", http.StatusConflict)
		return
	case errors.Is(err, db.ErrBadReference):
		http.Error(w, "unknown show", http.StatusBadRequest)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	// chapters are a bonus: a file we can't read them from still uploads
	if chapters, err := audio.ReadChapters(file); err != nil {
		log.Println("read chapters:", err)
	} else if len(chapters) > 0 {
		if err := db.SetChapters(id, chapters); err != nil {
			log.Println("save chapters:", err)
		}
	}
	render(w, r, components.Notice("Uploaded "+title+"."))
}

// savePosition takes {"seconds": n, "finished": bool} from the player. It
// is also sent with navigator.sendBeacon, which can't set a JSON content
// type, so the body is parsed whatever its type.
func savePosition(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}
	var req struct {
		Seconds  float64 `json:"seconds"`
		Finished bool    `json:"finished"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Seconds < 0 {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := db.SavePosition(id.String(), req.Seconds, req.Finished); errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func markPlayed(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}
	var err error
	if strings.HasSuffix(r.URL.Path, "/unplayed") {
		err = db.ClearPosition(id.String())
	} else {
		err = db.SavePosition(id.String(), 0, true)
	}
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, backTo(r, "/shows"))
}

// backTo is the path of the page that sent an htmx request, or fallback.
func backTo(r *http.Request, fallback string) string {
	if u, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil && strings.HasPrefix(u.Path, "/") {
		return u.Path
	}
	return fallback
}

func getChapters(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}
	chapters, err := db.GetChapters(id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	type chapter struct {
		Start float64 `json:"start"` // seconds
		Title string  `json:"title"`
	}
	out := make([]chapter, len(chapters))
	for i, c := range chapters {
		out[i] = chapter{Start: c.Start.Seconds(), Title: c.Title}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
