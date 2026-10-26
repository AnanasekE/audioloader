package api

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/mb"
	"audioloader/internal/tracking"
)

// parseArtistMBID reads a MusicBrainz artist ID from text that is either
// the ID itself or a musicbrainz.org/artist/<id> URL; "" if it is neither.
// Other pages' IDs, e.g. a release's, are refused: they'd link the wrong thing.
func parseArtistMBID(text string) string {
	text = strings.TrimSpace(text)
	if id := validMBID(text); id != "" {
		return strings.ToLower(id)
	}
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	u, err := url.Parse(text)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	if h := strings.ToLower(u.Hostname()); h != "musicbrainz.org" && h != "www.musicbrainz.org" && h != "beta.musicbrainz.org" {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "artist" {
		return ""
	}
	return strings.ToLower(validMBID(parts[1]))
}

func getFollowPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok {
		return
	}
	a, err := db.GetArtist(viewer(r), id.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	follow, err := db.GetFollow(id.String())
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		serverError(w, err)
		return
	}

	// without a linked ID, offer MusicBrainz's best guesses
	var candidates []mb.Artist
	searchErr := ""
	if a.MBID == "" {
		candidates, err = mb.Default().SearchArtists(r.Context(), a.Name, 5)
		if err != nil {
			log.Println("search artists:", err)
			searchErr = "Couldn't search MusicBrainz; paste the artist's ID below instead."
		}
	}
	render(w, r, components.FollowPage(a, follow, candidates, searchErr))
}

func followArtist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok {
		return
	}
	if _, err := db.GetArtist(viewer(r), id.String()); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	mbid := r.PostForm.Get("mbid")
	if manual := strings.TrimSpace(r.PostForm.Get("mbid_manual")); manual != "" {
		mbid = parseArtistMBID(manual)
		if mbid == "" {
			http.Error(w, "that isn't a MusicBrainz artist ID or URL", http.StatusBadRequest)
			return
		}
	}
	if validMBID(mbid) == "" {
		http.Error(w, "pick the artist on MusicBrainz", http.StatusBadRequest)
		return
	}

	f := db.FollowFilters{SkipSecondary: []string{}}
	for _, t := range r.PostForm["type"] {
		if audio.ReleaseType(t).Valid() {
			f.Types = append(f.Types, t)
		}
	}
	if len(f.Types) == 0 {
		http.Error(w, "pick at least one release type", http.StatusBadRequest)
		return
	}
	for _, s := range r.PostForm["skip"] {
		if slices.Contains(db.SecondaryTypes, s) {
			f.SkipSecondary = append(f.SkipSecondary, s)
		}
	}

	err := db.SetFollow(id.String(), strings.ToLower(mbid), f)
	switch {
	case errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, db.ErrMBIDTaken):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	if err := tracking.EnqueueCheck(id.String(), time.Time{}); err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/inbox")
}

func unfollowArtist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok {
		return
	}
	if _, err := db.GetArtist(viewer(r), id.String()); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := db.Unfollow(id.String()); err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/artists")
}

func checkArtistNow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok {
		return
	}
	if _, err := db.GetArtist(viewer(r), id.String()); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := tracking.EnqueueCheck(id.String(), time.Time{}); err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/inbox")
}

func getInboxPage(w http.ResponseWriter, r *http.Request) {
	tab := "new"
	if r.URL.Query().Get("tab") == "ignored" {
		tab = "ignored"
	}
	releases, err := db.ListInbox(tab)
	if err != nil {
		serverError(w, err)
		return
	}
	newCount, ignored, err := db.InboxCounts()
	if err != nil {
		serverError(w, err)
		return
	}
	follows, err := db.ListFollows()
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.InboxPage(tab, releases, newCount, ignored, follows))
}

func getInboxDetails(w http.ResponseWriter, r *http.Request) {
	rg, ok := pathUUID(w, r, "release_group")
	if !ok {
		return
	}
	ir, err := db.GetInboxRelease(rg.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.InboxDetails(ir))
}

func getInboxBadge(w http.ResponseWriter, r *http.Request) {
	n, err := db.InboxCount()
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.Badge(n))
}

func confirmInboxRelease(w http.ResponseWriter, r *http.Request) {
	rg, ok := pathUUID(w, r, "release_group")
	if !ok {
		return
	}
	releaseID, err := tracking.Confirm(rg.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/releases/"+releaseID)
}

func confirmAllInbox(w http.ResponseWriter, r *http.Request) {
	releases, err := db.ListInbox("new")
	if err != nil {
		serverError(w, err)
		return
	}
	for _, ir := range releases {
		if _, err := tracking.Confirm(ir.ReleaseGroupMBID); err != nil {
			serverError(w, err)
			return
		}
	}
	hxLocation(w, "/downloads")
}

func ignoreInboxRelease(w http.ResponseWriter, r *http.Request) {
	rg, ok := pathUUID(w, r, "release_group")
	if !ok {
		return
	}
	err := db.IgnoreInboxRelease(rg.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/inbox")
}

func restoreInboxRelease(w http.ResponseWriter, r *http.Request) {
	rg, ok := pathUUID(w, r, "release_group")
	if !ok {
		return
	}
	err := db.RestoreInboxRelease(rg.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/inbox?tab=ignored")
}
