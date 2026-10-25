package api

import (
	"errors"
	"io"
	"net/http"

	"audioloader/internal/db"
)

// maxCoverSize caps cover uploads.
const maxCoverSize = 10 << 20

// coverTypes maps the sniffed content types we accept to their extension.
var coverTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

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
	r.Body = http.MaxBytesReader(w, r.Body, maxCoverSize+(1<<20))
	file, _, err := r.FormFile("cover")
	if err != nil {
		http.Error(w, "missing cover image, or it is over 10 MiB", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// trust the bytes, not the file name or the browser's content type
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		http.Error(w, "could not read the image", http.StatusBadRequest)
		return
	}
	ext, ok := coverTypes[http.DetectContentType(head[:n])]
	if !ok {
		http.Error(w, "cover must be a JPEG, PNG or WebP image", http.StatusBadRequest)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		serverError(w, err)
		return
	}

	err = db.SetReleaseCover(id, file, ext)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/releases/"+id.String())
}

