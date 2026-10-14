package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"uuid"
)

func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/hello", hello)
	mux.HandleFunc("POST /api/hello", hello)
	mux.HandleFunc("POST /api/track/new", addTrack)
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

func addTrack(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "AYY WORKS")
	f, fh, _ := r.FormFile("track")
	fmt.Println(f)
	fmt.Println(fh.Filename)
	fo, err := os.Create(fmt.Sprintf("./data/%s", fh.Filename))
	if err != nil {
		panic(err)
	}
	io.Copy(fo, f)
}

func SaveTrackFile(f multipart.File) (*uuid.UUID, error) {
	dir := filepath.Dir("./data/")
	id := uuid.New()
	created_file, err := os.Create(dir + "/" + id.String())
	if err != nil {
		return nil, errors.New("Unable to save file")
	}
	io.Copy(created_file, f)
	return &id, nil
}
