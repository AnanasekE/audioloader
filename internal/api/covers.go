package api

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"audioloader/internal/avatar"
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

// getArtistImage serves the artist's own picture, else (normal mode) the
// cover of their newest release, else a pixel initials tile. Its URL never
// reveals a release ID.
func getArtistImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok {
		return
	}
	artist, err := db.GetArtist(viewer(r), id.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// fallbacks change without a new URL, so keep the cache short
	w.Header().Set("Cache-Control", "private, max-age=3600")

	path, err := db.ArtistImagePath(id)
	if err == nil {
		serveFileETag(w, r, path, "p")
		return
	}
	if !errors.Is(err, db.ErrNotFound) {
		serverError(w, err)
		return
	}
	releaseID, err := db.NewestReleaseCover(viewer(r), id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	if releaseID != "" {
		if rid, perr := uuid.Parse(releaseID); perr == nil {
			if path, err := db.CoverFilePath(rid); err == nil {
				serveFileETag(w, r, path, "c")
				return
			}
		}
	}
	tile, err := avatar.Tile(artist.Name)
	if err != nil {
		serverError(w, err)
		return
	}
	// hex, since an ETag may not hold spaces
	name := strings.ToLower(strings.TrimSpace(artist.Name))
	w.Header().Set("ETag", fmt.Sprintf(`"t-%x-%d"`, name, avatar.ColorIndex(name)))
	http.ServeContent(w, r, "tile.png", time.Time{}, bytes.NewReader(tile))
}

// serveFileETag serves path with an ETag made of kind, the file's name and
// its modification time, so a changed file never matches.
func serveFileETag(w http.ResponseWriter, r *http.Request, path, kind string) {
	if fi, err := os.Stat(path); err == nil {
		w.Header().Set("ETag", fmt.Sprintf(`"%s-%s-%d"`, kind, filepath.Base(path), fi.ModTime().UnixNano()))
	}
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

func getTrackImage(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}
	path, err := db.TrackImagePath(viewer(r), id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// a secret picture must not stay in the browser's cache after locking
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeFile(w, r, path)
}

// uploadTrackImage sets the picture of a secret track.
func uploadTrackImage(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok || !visibleTrack(w, r, id.String()) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, db.MaxCoverSize+(1<<20))
	file, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "missing image, or it is over 10 MiB", http.StatusBadRequest)
		return
	}
	defer file.Close()

	err = db.SaveTrackImage(viewer(r), id, file)
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
	hxLocation(w, "/tracks/"+id.String()+"/edit")
}

func removeTrackImage(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok || !visibleTrack(w, r, id.String()) {
		return
	}
	if err := db.RemoveTrackImage(viewer(r), id); errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/tracks/"+id.String()+"/edit")
}
