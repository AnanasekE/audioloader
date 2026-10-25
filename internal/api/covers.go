package api

import (
	"errors"
	"net/http"

	"audioloader/internal/db"
)

func getCover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "release_id")
	if !ok {
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
	if !ok {
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
