package api

import (
	"mime/multipart"
	"net/http"

	"audioloader/internal/db"
)

// maxTrackUpload caps the request body of a track or episode upload. Parts
// over 32 MiB are spooled to a temp file by the multipart reader, not held
// in memory.
const maxTrackUpload = 4 << 30

// trackUpload reads the audio file in form field "track", capped at
// maxTrackUpload, and its lowercased extension. On failure it has written
// the error.
func trackUpload(w http.ResponseWriter, r *http.Request) (multipart.File, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTrackUpload)
	file, header, err := r.FormFile("track")
	if err != nil {
		http.Error(w, "missing or invalid audio file, or it is over 4 GiB", http.StatusBadRequest)
		return nil, "", false
	}
	ext, ok := db.AudioExt(header.Filename)
	if !ok {
		file.Close()
		http.Error(w, db.ErrBadFileType.Error(), http.StatusBadRequest)
		return nil, "", false
	}
	return file, ext, true
}

// WithSecurityHeaders stops browsers from guessing a response's type from
// its content, so a file is only ever what its Content-Type says.
func WithSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
