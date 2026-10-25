package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"uuid"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
)

func getHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	artists, err := db.ListArtists()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tracks, err := db.ListTracks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c := components.Home(artists, tracks)
	err = c.Render(r.Context(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func createArtist(w http.ResponseWriter, r *http.Request) {
	artist_name := r.FormValue("name")
	err := db.AddArtist(artist_name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// tells listeners like the track form's artist select to refresh
	w.Header().Set("HX-Trigger", "artistsChanged")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<p>Success!</p>")
}

func getArtists(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c := components.ArtistList(artists)
	err = c.Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func getTracks(w http.ResponseWriter, r *http.Request) {
	tracks, err := db.ListTracks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c := components.TrackList(tracks)
	err = c.Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func addTrack(w http.ResponseWriter, r *http.Request) {
	artistUUID := r.FormValue("artist_id")
	title := r.FormValue("title")
	imgURL := r.FormValue("img_url")

	file, _, err := r.FormFile("track")
	if err != nil {
		http.Error(w, "missing or invalid track file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	duration, err := audio.ProbeDuration(file)
	if err != nil {
		http.Error(w, "could not read audio duration: "+err.Error(), http.StatusBadRequest)
		return
	}

	track := &audio.Track{
		Title:    title,
		Duration: duration.Round(time.Second),
		ImgURL:   imgURL,
	}

	ext := filepath.Ext(r.MultipartForm.File["track"][0].Filename)
	err = db.AddTrack(artistUUID, track, file, ext)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<p>Success!</p>")
}

func getArtistsOptions(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c := components.ArtistOptions(artists)
	err = c.Render(r.Context(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func getTrack(w http.ResponseWriter, r *http.Request) {
	trackID := r.PathValue("track_id")
	if trackID == "" {
		http.Error(w, "missing track id", http.StatusBadRequest)
		return
	}

	track, err := db.GetTrack(trackID)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c := components.Track(track)
	err = c.Render(r.Context(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func streamTrack(w http.ResponseWriter, r *http.Request) {
	trackID := r.PathValue("track_id")
	if trackID == "" {
		http.Error(w, "missing track id", http.StatusBadRequest)
		return
	}

	// trackID comes from the URL; a valid uuid can't contain "/" or "..",
	// so it can't escape TracksDir or inject glob metacharacters.
	id, err := uuid.Parse(trackID)
	if err != nil {
		http.Error(w, "invalid track id", http.StatusBadRequest)
		return
	}

	// Files are saved as TracksDir/<uuid><ext>; the ext isn't in the DB, so glob for it.
	matches, err := filepath.Glob(filepath.Join(db.TracksDir, id.String()+".*"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(matches) == 0 {
		http.NotFound(w, r)
		return
	}
	path := matches[0]

	f, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// ServeContent handles Range requests (seeking in <audio>) and sets
	// Content-Type from the name's extension.
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}
