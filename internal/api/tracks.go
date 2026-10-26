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
	"audioloader/internal/mb"
	"audioloader/internal/tagging"
)

// maxDescription caps the description of a secret track, in bytes.
const maxDescription = 10000

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

func getTrackEditPage(w http.ResponseWriter, r *http.Request) {
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
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.TrackEditPage(track, artists))
}

func updateTrack(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok || !visibleTrack(w, r, id.String()) {
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	kind := r.FormValue("kind")
	if kind != audio.KindMusic && kind != audio.KindEpisode {
		http.Error(w, "invalid kind", http.StatusBadRequest)
		return
	}
	// artist_id repeats once per selected artist; a song needs one
	artistIDs := r.Form["artist_id"]
	if kind == audio.KindMusic && len(artistIDs) == 0 {
		http.Error(w, "pick at least one artist", http.StatusBadRequest)
		return
	}
	for _, aid := range artistIDs {
		if _, err := uuid.Parse(aid); err != nil {
			http.Error(w, "invalid artist id", http.StatusBadRequest)
			return
		}
	}
	edit := db.TrackEdit{Title: title, Kind: kind, ArtistIDs: artistIDs}
	cover, ok := coverFromForm(w, r)
	if !ok {
		return
	}
	edit.Cover = cover
	// a description and tags are for secret tracks only
	if viewer(r).IsSecret() {
		if r.Form.Has("description") {
			d := strings.TrimSpace(r.FormValue("description"))
			if len(d) > maxDescription {
				http.Error(w, "the description is too long", http.StatusBadRequest)
				return
			}
			edit.Description = &d
		}
		if r.Form.Has("tags") {
			tags := db.NormalizeTags(strings.Split(r.FormValue("tags"), ","))
			edit.Tags = &tags
		}
	}

	err := db.UpdateTrack(viewer(r), id.String(), edit)
	switch {
	case errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, db.ErrBadEdit), errors.Is(err, db.ErrBadReference):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/tracks")
}

// coverFromForm reads the "Cover of" part of the edit form. cover_mode says
// which fields count: nothing (leave as is), the library track
// cover_track_id, the text fields cover_title and cover_artists, a
// MusicBrainz recording URL that is looked up here, or none to remove the
// link. It returns nil to leave the cover alone, and writes a 400 for bad values.
func coverFromForm(w http.ResponseWriter, r *http.Request) (*db.CoverEdit, bool) {
	switch r.FormValue("cover_mode") {
	case "", "keep":
		return nil, true
	case "none":
		return &db.CoverEdit{}, true
	case "library":
		id := strings.TrimSpace(r.FormValue("cover_track_id"))
		if _, err := uuid.Parse(id); err != nil {
			http.Error(w, "pick the original track from the list", http.StatusBadRequest)
			return nil, false
		}
		return &db.CoverEdit{TrackID: id}, true
	case "text":
		title := strings.TrimSpace(r.FormValue("cover_title"))
		if title == "" {
			http.Error(w, "the original needs a title", http.StatusBadRequest)
			return nil, false
		}
		return &db.CoverEdit{Title: title, Artists: strings.TrimSpace(r.FormValue("cover_artists"))}, true
	case "mbid":
		id, ok := mb.RecordingID(r.FormValue("cover_mb_url"))
		if !ok {
			http.Error(w, "not a MusicBrainz recording URL", http.StatusBadRequest)
			return nil, false
		}
		rec, err := mb.Default().GetRecording(r.Context(), id)
		if errors.Is(err, mb.ErrNotFound) {
			http.Error(w, "MusicBrainz doesn't know that recording", http.StatusBadRequest)
			return nil, false
		}
		if err != nil {
			log.Println("cover of: musicbrainz:", err)
			http.Error(w, "could not reach MusicBrainz, try again later", http.StatusBadGateway)
			return nil, false
		}
		return &db.CoverEdit{Title: rec.Title, Artists: rec.ArtistCredit.String(), MBID: rec.ID}, true
	}
	http.Error(w, "invalid cover mode", http.StatusBadRequest)
	return nil, false
}

// pickCoverTracks answers the "Cover of" search: library songs matching q,
// leaving out the track being edited (exclude).
func pickCoverTracks(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	res, err := db.Search(viewer(r), q, 8)
	if err != nil {
		serverError(w, err)
		return
	}
	exclude := r.URL.Query().Get("exclude")
	tracks := make([]*audio.Track, 0, len(res.Tracks))
	for _, t := range res.Tracks {
		if t.ID.String() != exclude && !t.IsEpisode() {
			tracks = append(tracks, t)
		}
	}
	render(w, r, components.CoverPickResults(tracks))
}

func deleteTrack(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}
	err := db.DeleteTrack(viewer(r), id.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, backTo(r, "/tracks"))
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
	case errors.Is(err, db.ErrHidden):
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	if !ok || !visibleTrack(w, r, id.String()) {
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
