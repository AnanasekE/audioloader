package api

import (
	"errors"
	"net/http"
	"strings"

	"audioloader/internal/components"
	"audioloader/internal/db"
)

func getPlaylistsPage(w http.ResponseWriter, r *http.Request) {
	playlists, err := db.ListPlaylists(viewer(r))
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
	p, err := db.GetPlaylist(viewer(r), id)
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
	p, err := db.CreatePlaylist(viewer(r), name)
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
	if err := db.RenamePlaylist(viewer(r), id, name); errors.Is(err, db.ErrNotFound) {
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
	if err := db.DeletePlaylist(viewer(r), id); errors.Is(err, db.ErrNotFound) {
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
	err := db.SetPlaylistTracks(viewer(r), id, trackIDs)
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
	if !visibleTrack(w, r, trackID) {
		return
	}
	playlists, err := db.ListPlaylists(viewer(r))
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
	if !visibleTrack(w, r, trackID) {
		return
	}
	if err := db.AppendToPlaylist(viewer(r), id, trackID); errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if errors.Is(err, db.ErrBadReference) {
		http.Error(w, "that track can't go on this playlist", http.StatusBadRequest)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	p, err := db.GetPlaylist(viewer(r), id)
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.Notice("Added to "+p.Name+"."))
}
