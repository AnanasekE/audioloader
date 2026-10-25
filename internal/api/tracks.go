package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"uuid"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/tagging"
)

func getTracks(w http.ResponseWriter, r *http.Request) {
	tracks, err := db.ListTracks(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.TrackList(tracks))
}

func getTrack(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}

	track, err := db.GetTrack(viewer(r), id.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.Track(track))
}

func addTrack(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("track")
	if err != nil {
		http.Error(w, "missing or invalid track file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	duration, err := audio.ProbeDuration(file)
	if err != nil {
		http.Error(w, "could not read audio duration: "+err.Error(), http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}

	// artist_id can repeat: one value per selected artist
	artistIDs := r.MultipartForm.Value["artist_id"]
	if len(artistIDs) == 0 {
		http.Error(w, "pick at least one artist", http.StatusBadRequest)
		return
	}
	for _, id := range artistIDs {
		if _, err := uuid.Parse(id); err != nil {
			http.Error(w, "invalid artist id", http.StatusBadRequest)
			return
		}
	}

	link, ok := releaseLinkFromForm(w, r)
	if !ok {
		return
	}
	// without a release the track becomes its own single, on this date
	singleDate := strings.TrimSpace(r.FormValue("date"))
	if link == nil && singleDate != "" && !releaseDate.MatchString(singleDate) {
		http.Error(w, "date must be YYYY, YYYY-MM or YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	track := &audio.Track{
		Title:    title,
		Duration: duration.Round(time.Second),
		// optional, from the file's tags; invalid values are dropped, not rejected
		MBID: validMBID(r.FormValue("mbid")),
		ISRC: validISRC(r.FormValue("isrc")),
	}
	ext := filepath.Ext(header.Filename)
	res, err := db.AddTrack(viewer(r), artistIDs, track, file, ext, link, singleDate)
	switch {
	case errors.Is(err, db.ErrExists):
		http.Error(w, "already exists", http.StatusConflict)
		return
	case errors.Is(err, db.ErrAlreadyOnRelease):
		http.Error(w, "already on this release", http.StatusConflict)
		return
	case errors.Is(err, db.ErrSlotTaken):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, db.ErrBadReference):
		http.Error(w, "unknown artist or release", http.StatusBadRequest)
		return
	case err != nil:
		serverError(w, err)
		return
	}

	if res.Linked {
		render(w, r, components.Notice("Already in the library, added to the release."))
		return
	}
	// a failed enqueue only means the track isn't auto-tagged; the upload worked
	if err := tagging.Enqueue(res.TrackID); err != nil {
		log.Println("enqueue tagging:", err)
	}
	if link == nil {
		render(w, r, components.Notice("Uploaded "+track.Title+" as a single."))
		return
	}
	render(w, r, components.Notice("Uploaded "+track.Title+"."))
}

// releaseLinkFromForm reads the optional release_id, disc and position fields.
// It returns nil when no release_id was sent, and writes a 400 for bad values.
func releaseLinkFromForm(w http.ResponseWriter, r *http.Request) (*db.ReleaseLink, bool) {
	releaseID := r.FormValue("release_id")
	if releaseID == "" {
		return nil, true
	}
	if _, err := uuid.Parse(releaseID); err != nil {
		http.Error(w, "invalid release id", http.StatusBadRequest)
		return nil, false
	}
	disc, err1 := strconv.Atoi(r.FormValue("disc"))
	position, err2 := strconv.Atoi(r.FormValue("position"))
	if err1 != nil || err2 != nil || disc < 1 || position < 1 {
		http.Error(w, "disc and position must be numbers from 1", http.StatusBadRequest)
		return nil, false
	}
	return &db.ReleaseLink{ReleaseID: releaseID, Disc: disc, Position: position}, true
}

// checkTracksExist takes a JSON array of fingerprints and answers with a
// JSON array of booleans, one per fingerprint, true if it's already stored.
func checkTracksExist(w http.ResponseWriter, r *http.Request) {
	var fps []db.Fingerprint
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&fps); err != nil {
		http.Error(w, "invalid fingerprints", http.StatusBadRequest)
		return
	}

	exists := make([]bool, len(fps))
	for i, fp := range fps {
		ok, err := db.TrackExists(viewer(r), fp)
		if err != nil {
			serverError(w, err)
			return
		}
		exists[i] = ok
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exists)
}

func streamTrack(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}

	path, err := db.TrackFilePath(id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		serverError(w, err)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		serverError(w, err)
		return
	}

	// ServeContent handles Range requests (seeking in <audio>) and sets
	// Content-Type from the name's extension.
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}

// trackID parses the {track_id} path value, writing a 400 if it isn't a uuid.
func trackID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	return pathUUID(w, r, "track_id")
}

// pathUUID parses the named path value, writing a 400 if it isn't a uuid.
func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		http.Error(w, "invalid "+strings.ReplaceAll(name, "_", " "), http.StatusBadRequest)
		return uuid.UUID{}, false
	}
	return id, true
}

// isrcPattern is an ISRC without hyphens: country, registrant, year, number.
var isrcPattern = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{3}[0-9]{7}$`)

// validMBID returns s normalized if it is a MusicBrainz ID (a UUID), else "".
func validMBID(s string) string {
	id, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	return id.String()
}

// validISRC returns s normalized if it is an ISRC, else "".
func validISRC(s string) string {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "-", ""))
	if !isrcPattern.MatchString(s) {
		return ""
	}
	return s
}
