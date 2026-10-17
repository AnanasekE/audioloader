package api

import (
	"context"
	"net/http"

	"audioloader/internal/components"
)

func Register(mux *http.ServeMux) {
	mux.Handle("GET /assets/", http.StripPrefix(
		"/assets/",
		http.FileServer(http.Dir("assets")),
	))

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		c := components.Home()
		err := c.Render(context.Background(), w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	mux.HandleFunc("POST /api/artists", createArtist)
	mux.HandleFunc("GET /api/artists", getArtists)
	mux.HandleFunc("GET /api/tracks", getTracks)
}
