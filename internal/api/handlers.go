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

func createArtist(w http.ResponseWriter, r *http.Request) {
	artist_name := r.FormValue("name")
	id := uuid.New().String()

	query := `
	INSERT INTO ARTISTS(uuid, name) VALUES(?, ?);
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

func getTracks(w http.ResponseWriter, r *http.Request) {
	query1 := `SELECT uuid, title, duration_seconds, img_url FROM tracks;`
	rows, err := db.DB.Query(query1)
	defer rows.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tracks := make([]*audio.Track, 0)

	for rows.Next() {
		var track audio.Track
		if err := rows.Scan(&track.ID, &track.Title, &track.ImgURL); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tracks = append(tracks, &track)
	}

	c := components.TrackList(tracks)
	err = c.Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
