package api

import (
	"context"
	"fmt"
	"net/http"
	"uuid"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
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
}

func createArtist(w http.ResponseWriter, r *http.Request) {
	artist_name := r.FormValue("name")
	id := uuid.New().String()

	query := `
	INSERT INTO ARTISTS VALUES(?, ?, null);
	`

	tx, _ := db.DB.Begin()
	_, err := tx.Exec(query, id, artist_name)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err2 := tx.Commit()
	if err2 != nil {
		http.Error(w, err2.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<p>Success!</p>")
}

func getArtists(w http.ResponseWriter, r *http.Request) {
	query1 := `SELECT uuid, name, img_url FROM artists;`
	rows, err := db.DB.Query(query1)
	defer rows.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	artists := make([]*audio.Artist, 0)

	for rows.Next() {
		var artist audio.Artist
		if err := rows.Scan(&artist.ID, &artist.Name, &artist.ImgURL); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		artists = append(artists, &artist)
	}

	c := components.ArtistList(artists)
	err = c.Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
