package api

import (
	"net/http"
)

func Register(mux *http.ServeMux) {
	mux.Handle("GET /assets/", http.StripPrefix(
		"/assets/",
		http.FileServer(http.Dir("assets")),
	))

	mux.HandleFunc("GET /", getHome)

	mux.HandleFunc("POST /ui/artists", createArtist)
	mux.HandleFunc("GET /ui/artists", getArtists)
	mux.HandleFunc("GET /ui/artists/options", getArtistsOptions)
	mux.HandleFunc("GET /ui/tracks", getTracks)
	mux.HandleFunc("GET /ui/tracks/{track_id}", getTrack)
	mux.HandleFunc("POST /ui/tracks", addTrack)

	mux.HandleFunc("GET /api/tracks/{track_id}/stream", streamTrack)
}
