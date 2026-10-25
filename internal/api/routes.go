package api

import (
	"net/http"
)

func Register(mux *http.ServeMux) {
	mux.Handle("GET /assets/", http.StripPrefix(
		"/assets/",
		http.FileServer(http.Dir("assets")),
	))

	mux.HandleFunc("GET /{$}", getIndex)
	mux.HandleFunc("GET /tracks", getTracksPage)
	mux.HandleFunc("GET /upload", getUploadPage)
	mux.HandleFunc("GET /upload/batch", getBatchUploadPage)
	mux.HandleFunc("GET /upload/import", getImportPage)
	mux.HandleFunc("GET /artists", getArtistsPage)
	mux.HandleFunc("GET /releases", getReleasesPage)
	mux.HandleFunc("GET /releases/{release_id}", getReleasePage)
	mux.HandleFunc("GET /releases/{release_id}/edit", getReleaseEditPage)

	mux.HandleFunc("POST /ui/artists", createArtist)
	mux.HandleFunc("GET /ui/artists", getArtists)
	mux.HandleFunc("GET /ui/tracks", getTracks)
	mux.HandleFunc("GET /ui/tracks/{track_id}", getTrack)
	mux.HandleFunc("POST /ui/tracks", addTrack)
	mux.HandleFunc("POST /ui/releases", createRelease)
	mux.HandleFunc("POST /ui/releases/{release_id}/tracks", saveReleaseTracks)

	mux.HandleFunc("GET /api/tracks/{track_id}/stream", streamTrack)
	mux.HandleFunc("POST /api/tracks/exists", checkTracksExist)
	mux.HandleFunc("POST /api/artists/resolve", resolveArtists)
	mux.HandleFunc("POST /api/releases", createReleaseJSON)
}
