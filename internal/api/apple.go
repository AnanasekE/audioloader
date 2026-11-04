package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"audioloader/internal/addsong"
	"audioloader/internal/appleimport"
	"audioloader/internal/applemusic"
	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/mb"
)

const msgAppleLink = "Paste a song or playlist link."

// maxAppleCandidates is how many MusicBrainz recordings a song link lists.
const maxAppleCandidates = 5

// appleSongMatches reads the song an Apple Music link points to, and the
// MusicBrainz recordings that may be it, best first.
func appleSongMatches(r *http.Request, link string) (applemusic.Song, []appleimport.Candidate, *songError) {
	song, err := applemusic.Default().GetSong(r.Context(), link)
	if serr := appleError(err); serr != nil {
		return song, nil, serr
	}
	cands, err := appleimport.Find(r.Context(), mb.Default(), song)
	if err != nil {
		return song, nil, &songError{http.StatusBadGateway, "couldn't search MusicBrainz: " + err.Error()}
	}
	return song, cands, nil
}

// appleError is err from the applemusic package as the answer to give.
func appleError(err error) *songError {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, applemusic.ErrNotFound):
		return &songError{http.StatusNotFound, "that page isn't on Apple Music"}
	case errors.Is(err, applemusic.ErrUnreadable):
		return &songError{http.StatusBadGateway, err.Error()}
	}
	return &songError{http.StatusBadGateway, "couldn't read Apple Music: " + err.Error()}
}

// listAppleMusic is the Add box for an Apple Music link: a song is shown as
// the MusicBrainz recordings it may be, the sure one first.
func listAppleMusic(w http.ResponseWriter, r *http.Request, link string) {
	kind, _, _, ok := applemusic.ParseLink(link)
	if ok && kind == "playlist" {
		startAppleImport(w, link)
		return
	}
	if !ok || kind != "song" {
		http.Error(w, msgAppleLink, http.StatusBadRequest)
		return
	}
	song, cands, serr := appleSongMatches(r, link)
	if serr != nil {
		http.Error(w, serr.msg, serr.status)
		return
	}
	library, err := db.ListTracks(db.Normal)
	if err != nil {
		serverError(w, err)
		return
	}
	if len(cands) > maxAppleCandidates {
		cands = cands[:maxAppleCandidates]
	}
	c := mb.Default()
	found := make([]components.SongCandidate, 0, len(cands))
	for _, cand := range cands {
		sc, err := candidateOn(c, library, cand.Recording, cand.Release)
		if err != nil {
			serverError(w, err)
			return
		}
		sc.Best = cand.Confident && len(found) == 0
		found = append(found, sc)
	}
	render(w, r, components.SongCandidates(song.Artist+" - "+song.Title, nil, found))
}

// shareAppleSong is a share of an Apple Music link: a song with a sure
// MusicBrainz match is added right away, as the Add page's "Add" would. In
// any other case the Add page is where to go on.
func shareAppleSong(w http.ResponseWriter, r *http.Request, link string) {
	if !allowShare() {
		http.Error(w, "Too many shares; try again in a minute.", http.StatusTooManyRequests)
		return
	}
	kind, _, _, _ := applemusic.ParseLink(link)
	switch kind {
	case "song":
	case "playlist":
		shareOpenAddPage(w, r, link, "Open the Add page to import a playlist.")
		return
	default:
		http.Error(w, msgAppleLink, http.StatusBadRequest)
		return
	}
	_, cands, serr := appleSongMatches(r, link)
	if serr != nil {
		http.Error(w, serr.msg, serr.status)
		return
	}
	best := appleimport.Best(cands)
	if best == nil {
		shareOpenAddPage(w, r, link, "Open the Add page to pick.")
		return
	}
	releaseID := ""
	if best.Release != nil {
		releaseID = best.Release.ID
	}
	song, it, serr := songFor(r.Context(), best.Recording.ID, releaseID)
	if serr != nil {
		http.Error(w, serr.msg, serr.status)
		return
	}
	library, err := db.ListTracks(db.Normal)
	if err != nil {
		serverError(w, err)
		return
	}
	if _, err := addsong.Queue(library, song, it); errors.Is(err, addsong.ErrAlreadyOwned) {
		http.Error(w, "Already in your library: "+song.Title, http.StatusConflict)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	artists := make([]string, len(song.Artists))
	for i, a := range song.Artists {
		artists[i] = a.Name
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, "Queued: %s by %s\n", song.Title, strings.Join(artists, ", "))
}

// shareOpenAddPage answers a share that needs a person: msg, then the Add
// page with the link filled in on the next line.
func shareOpenAddPage(w http.ResponseWriter, r *http.Request, link, msg string) {
	http.Error(w, msg+"\n"+publicBase(r)+"/add?q="+url.QueryEscape(link), http.StatusUnprocessableEntity)
}
