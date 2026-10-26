package api

import (
	"errors"
	"net/http"

	"audioloader/internal/db"
)

func getCover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "release_id")
	if !ok || !visibleRelease(w, r, id.String()) {
		return
	}
	path, err := db.CoverFilePath(id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// the URL carries a version, so a changed cover gets a new URL
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, path)
}

func uploadCover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "release_id")
	if !ok || !visibleRelease(w, r, id.String()) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, db.MaxCoverSize+(1<<20))
	file, _, err := r.FormFile("cover")
	if err != nil {
		http.Error(w, "missing cover image, or it is over 10 MiB", http.StatusBadRequest)
		return
	}
	defer file.Close()

	err = db.SaveCoverImage(id, file)
	switch {
	case errors.Is(err, db.ErrBadImage):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/releases/"+id.String())
}

func getArtistImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok || !visibleArtist(w, r, id.String()) {
		return
	}
	path, err := db.ArtistImagePath(id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// the URL carries the file's name as its version, so a new picture gets a new URL
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, path)
}

func uploadArtistImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok || !visibleArtist(w, r, id.String()) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, db.MaxCoverSize+(1<<20))
	file, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "missing image, or it is over 10 MiB", http.StatusBadRequest)
		return
	}
	defer file.Close()

	err = db.SaveArtistImage(id, file)
	switch {
	case errors.Is(err, db.ErrBadImage):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/artists/"+id.String())
}

func removeArtistImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok || !visibleArtist(w, r, id.String()) {
		return
	}
	err := db.RemoveArtistImage(id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/artists/"+id.String())
}

// visibleArtist writes a 404 and returns false if the viewer can't see the artist.
func visibleArtist(w http.ResponseWriter, r *http.Request, id string) bool {
	_, err := db.GetArtist(viewer(r), id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return false
	}
	if err != nil {
		serverError(w, err)
		return false
	}
	return true
}
