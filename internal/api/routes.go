package api

import (
	"fmt"
	"net/http"
	"uuid"

	"audioloader/internal/db"
)

func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/hello", hello)
	mux.HandleFunc("POST /api/hello", hello)
	// mux.HandleFunc("POST /api/track/new", addTrack)
	mux.HandleFunc("POST /api/artists", createArtist)
	mux.HandleFunc("GET /api/artists", getArtists)
}

func hello(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<p>Hello from the API! <strong>HTMX works.</strong></p>`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"message":"Hello from the API!"}`)
}

func createArtist(w http.ResponseWriter, r *http.Request) {
	artist_name := r.FormValue("name")
	id := uuid.New().String()

	query1 := `
	INSERT INTO ARTISTS VALUES(%s, %s, null)
	`
	query2 := fmt.Sprintf(query1, id, artist_name)

	tx, _ := db.DB.Begin()
	_, err := tx.Exec(query2)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	tx.Commit()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<p>Success!</p>")
}

func getArtists(w http.ResponseWriter, r *http.Request) {
}

// func addTrack(w http.ResponseWriter, r *http.Request) {
// 	fmt.Fprint(w, "AYY WORKS")
// 	f, fh, _ := r.FormFile("track")
// 	fmt.Println(f)
// 	fmt.Println(fh.Filename)
// 	fo, err := os.Create(fmt.Sprintf("./data/%s", fh.Filename))
// 	if err != nil {
// 		panic(err)
// 	}
// 	io.Copy(fo, f)
// }
//
// func SaveTrackFile(f multipart.File) (*uuid.UUID, error) {
// 	dir := filepath.Dir("./data/")
// 	id := uuid.New()
// 	created_file, err := os.Create(dir + "/" + id.String())
// 	if err != nil {
// 		return nil, errors.New("Unable to save file")
// 	}
// 	io.Copy(created_file, f)
// 	return &id, nil
// }
