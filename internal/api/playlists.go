package api

import (
	"errors"
	"net/http"
	"strings"

	"audioloader/internal/components"
	"audioloader/internal/db"
)

func getPlaylistsPage(w http.ResponseWriter, r *http.Request) {
	playlists, err := db.ListPlaylists()
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.PlaylistsPage(playlists))
}

// playlistID reads {playlist_id}, writing a 400 if it isn't a uuid.
func playlistID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := pathUUID(w, r, "playlist_id")
	if !ok {
		return "", false
	}
	return id.String(), true
}

func getPlaylistPage(w http.ResponseWriter, r *http.Request) {
	id, ok := playlistID(w, r)
	if !ok {
		return
	}
	p, err := db.GetPlaylist(id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/edit") {
		render(w, r, components.PlaylistEditPage(p))
		return
	}
	render(w, r, components.PlaylistPage(p))
}

func createPlaylist(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	p, err := db.CreatePlaylist(name)
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/playlists/"+p.ID.String())
}

func renamePlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := playlistID(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if err := db.RenamePlaylist(id, name); errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/playlists/"+id)
}

func deletePlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := playlistID(w, r)
	if !ok {
		return
	}
	if err := db.DeletePlaylist(id); errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/playlists")
}

func savePlaylistTracks(w http.ResponseWriter, r *http.Request) {
	id, ok := playlistID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	trackIDs := r.PostForm["track_id"]
	for _, t := range trackIDs {
		if validMBID(t) == "" { // checks for a UUID
			http.Error(w, "invalid track id", http.StatusBadRequest)
			return
		}
	}
	err := db.SetPlaylistTracks(id, trackIDs)
	switch {
	case errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, db.ErrBadReference):
		http.Error(w, "unknown track", http.StatusBadRequest)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/playlists/"+id)
}

func getPlaylistPicker(w http.ResponseWriter, r *http.Request) {
	trackID := validMBID(r.URL.Query().Get("track_id"))
	if trackID == "" {
		http.Error(w, "invalid track id", http.StatusBadRequest)
		return
	}
	playlists, err := db.ListPlaylists()
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.PlaylistPicker(trackID, playlists))
}

func addToPlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := playlistID(w, r)
	if !ok {
		return
	}
	trackID := validMBID(r.FormValue("track_id"))
	if trackID == "" {
		http.Error(w, "invalid track id", http.StatusBadRequest)
		return
	}
	if err := db.AppendToPlaylist(id, trackID); errors.Is(err, db.ErrBadReference) {
		http.Error(w, "unknown playlist or track", http.StatusBadRequest)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	p, err := db.GetPlaylist(id)
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.Notice("Added to "+p.Name+"."))
}
