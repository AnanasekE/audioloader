package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"audioloader/internal/appleimport"
	"audioloader/internal/applemusic"
	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
)

// recentAppleImports is how many imports the Add page lists.
const recentAppleImports = 5

// startAppleImport answers an Apple Music playlist link in the Add box: it
// starts an import and sends the browser to its page. A link whose import
// is still running reopens that one.
func startAppleImport(w http.ResponseWriter, link string) {
	recent, err := db.ListAppleImports(recentAppleImports)
	if err != nil {
		serverError(w, err)
		return
	}
	for _, i := range recent {
		if i.URL == link && (i.Status == db.ImportQueued || i.Status == db.ImportRunning) {
			hxLocation(w, fmt.Sprintf("/add/apple/%d", i.ID))
			return
		}
	}
	id, err := appleimport.Start(link)
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, fmt.Sprintf("/add/apple/%d", id))
}

// appleImportFor reads the import named by the request's {id}, answering
// 404 when there is none.
func appleImportFor(w http.ResponseWriter, r *http.Request) (*db.AppleImport, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	imp, err := db.GetAppleImport(id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return nil, false
	}
	if err != nil {
		serverError(w, err)
		return nil, false
	}
	return imp, true
}

// appleTrackFor is appleImportFor for a song of it, by the request's {position}.
func appleTrackFor(w http.ResponseWriter, r *http.Request, imp *db.AppleImport) (*db.AppleImportTrack, bool) {
	pos, err := strconv.Atoi(r.PathValue("position"))
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	tracks, err := db.AppleImportTracks(imp.ID)
	if err != nil {
		serverError(w, err)
		return nil, false
	}
	for _, t := range tracks {
		if t.Position == pos {
			return t, true
		}
	}
	http.NotFound(w, r)
	return nil, false
}

// appleImportView is what the page shows of imp. An import whose job gave
// up shows its error and stops polling.
func appleImportView(imp *db.AppleImport, notice string) (components.AppleImportView, error) {
	tracks, err := db.AppleImportTracks(imp.ID)
	if err != nil {
		return components.AppleImportView{}, err
	}
	v := components.AppleImportView{
		ID: imp.ID, Name: imp.Name, Listed: len(tracks), TrackCount: imp.TrackCount,
		PlaylistID: imp.PlaylistID, Notice: notice, Error: imp.Error,
	}
	active := imp.Status == db.ImportQueued || imp.Status == db.ImportRunning
	if active {
		j, err := jobs.Latest(appleimport.JobImport, appleimport.Key(imp.ID))
		if err != nil {
			return v, err
		}
		if j != nil && j.Status == "failed" {
			v.Error, active = "The import stopped: "+j.LastError, false
		}
	}
	if imp.Status == db.ImportFailed && v.Error == "" {
		v.Error = "The import failed."
	}
	v.Running = active
	if f, err := db.AppleFollowOfImport(imp.ID); err == nil {
		v.FollowID = f.ID
	} else if !errors.Is(err, db.ErrNotFound) {
		return v, err
	}
	for _, t := range tracks {
		if t.State != db.AppleSongPending {
			v.Searched++
		}
		v.Songs = append(v.Songs, components.AppleSongRow{
			Position: t.Position, Title: t.Title, Artist: t.Artist,
			Length: time.Duration(t.DurationMs) * time.Millisecond, State: t.State,
			MatchTitle: t.MatchTitle, MatchArtist: t.MatchArtist, ReleaseTitle: t.ReleaseTitle,
			Score: int(t.Score*100 + 0.5),
		})
	}
	return v, nil
}

// getAppleImportPage is the page of one import.
func getAppleImportPage(w http.ResponseWriter, r *http.Request) {
	imp, ok := appleImportFor(w, r)
	if !ok {
		return
	}
	v, err := appleImportView(imp, "")
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.AppleImportPage(v))
}

// getAppleImportBody is what the page polls while the import runs.
func getAppleImportBody(w http.ResponseWriter, r *http.Request) {
	imp, ok := appleImportFor(w, r)
	if !ok {
		return
	}
	renderAppleImport(w, r, imp, "")
}

func renderAppleImport(w http.ResponseWriter, r *http.Request, imp *db.AppleImport, notice string) {
	v, err := appleImportView(imp, notice)
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.AppleImportBody(v))
}

// appleSong is a song of an import for appleimport's matcher.
func appleSong(t *db.AppleImportTrack) applemusic.Song {
	return applemusic.Song{Title: t.Title, Artist: t.Artist, Album: t.Album, Duration: time.Duration(t.DurationMs) * time.Millisecond}
}

// maxAppleChange is how many candidates "Change" lists.
const maxAppleChange = 5

// getAppleCandidates lists the MusicBrainz recordings a song may be, for
// "Change".
func getAppleCandidates(w http.ResponseWriter, r *http.Request) {
	imp, ok := appleImportFor(w, r)
	if !ok {
		return
	}
	t, ok := appleTrackFor(w, r, imp)
	if !ok {
		return
	}
	cands, err := appleimport.Find(r.Context(), mb.Default(), appleSong(t))
	if err != nil {
		http.Error(w, "couldn't search MusicBrainz: "+err.Error(), http.StatusBadGateway)
		return
	}
	if len(cands) > maxAppleChange {
		cands = cands[:maxAppleChange]
	}
	found := make([]components.AppleCandidate, 0, len(cands))
	for _, c := range cands {
		ac := components.AppleCandidate{
			RecordingID: c.Recording.ID, Title: c.Recording.Title, Artists: c.Recording.ArtistCredit.String(),
			Length: time.Duration(c.Recording.Length) * time.Millisecond,
		}
		if c.Release != nil {
			ac.ReleaseID, ac.ReleaseTitle = c.Release.ID, c.Release.Title
		}
		found = append(found, ac)
	}
	render(w, r, components.AppleCandidates(imp.ID, t.Position, found))
}

// pickAppleCandidate makes the recording a person picked the match of a
// song, ready to be added.
func pickAppleCandidate(w http.ResponseWriter, r *http.Request) {
	imp, ok := appleImportFor(w, r)
	if !ok {
		return
	}
	t, ok := appleTrackFor(w, r, imp)
	if !ok {
		return
	}
	if t.State == db.AppleSongAdded || t.State == db.AppleSongOwned {
		http.Error(w, "this song is already in your library", http.StatusConflict)
		return
	}
	recordingID := validMBID(r.FormValue("recording_id"))
	if recordingID == "" {
		http.Error(w, "invalid recording id", http.StatusBadRequest)
		return
	}
	releaseID := ""
	if r.FormValue("release_id") != "" {
		if releaseID = validMBID(r.FormValue("release_id")); releaseID == "" {
			http.Error(w, "invalid release id", http.StatusBadRequest)
			return
		}
	}
	rec, err := mb.Default().GetRecording(r.Context(), recordingID)
	if errors.Is(err, mb.ErrNotFound) {
		http.Error(w, "that recording isn't on MusicBrainz", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "couldn't read MusicBrainz: "+err.Error(), http.StatusBadGateway)
		return
	}
	picked := *t
	picked.RecordingID, picked.ReleaseID, picked.ReleaseTitle = rec.ID, "", ""
	picked.MatchTitle, picked.MatchArtist = rec.Title, rec.ArtistCredit.String()
	if releaseID != "" {
		i := slices.IndexFunc(rec.Releases, func(rel mb.Release) bool { return rel.ID == releaseID })
		if i < 0 {
			http.Error(w, "that recording isn't on this release", http.StatusBadRequest)
			return
		}
		picked.ReleaseID, picked.ReleaseTitle = releaseID, rec.Releases[i].Title
	}
	picked.State = db.AppleSongMatched
	if err := db.SetAppleTrackMatch(picked); err != nil {
		serverError(w, err)
		return
	}
	renderAppleImport(w, r, imp, "")
}

// addAppleImport adds the ticked songs of an import, each as a placeholder
// on its MusicBrainz release with a queued download, and with "playlist"
// makes or updates the audioloader playlist from the songs the library has.
func addAppleImport(w http.ResponseWriter, r *http.Request) {
	imp, ok := appleImportFor(w, r)
	if !ok {
		return
	}
	if imp.Status != db.ImportDone {
		http.Error(w, "the import is still running", http.StatusConflict)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	tracks, err := db.AppleImportTracks(imp.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	library, err := db.ListTracks(db.Normal)
	if err != nil {
		serverError(w, err)
		return
	}
	ticked := map[string]bool{}
	for _, p := range r.Form["pos"] {
		ticked[p] = true
	}
	// several songs of one release read it once
	releases := map[string]*mb.Release{}
	added, had := 0, 0
	for _, t := range tracks {
		if !ticked[strconv.Itoa(t.Position)] || t.RecordingID == "" || (t.State != db.AppleSongMatched && t.State != db.AppleSongUnsure) {
			continue
		}
		err := appleimport.AddTrack(r.Context(), mb.Default(), releases, library, t)
		switch {
		case errors.Is(err, mb.ErrNotFound):
			http.Error(w, "that recording isn't on MusicBrainz", http.StatusBadRequest)
			return
		case errors.Is(err, appleimport.ErrMusicBrainz):
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		case err != nil:
			serverError(w, err)
			return
		case t.State == db.AppleSongOwned:
			had++
		default:
			added++
		}
	}
	notice := fmt.Sprintf("Added %d songs.", added)
	if added == 1 {
		notice = "Added 1 song."
	}
	if had > 0 {
		notice += fmt.Sprintf(" %d were in your library already.", had)
	}
	if r.FormValue("playlist") != "" {
		if err := appleimport.SyncPlaylist(imp, tracks); err != nil {
			serverError(w, err)
			return
		}
		notice += " The playlist is up to date."
		if imp, err = db.GetAppleImport(imp.ID); err != nil {
			serverError(w, err)
			return
		}
	}
	renderAppleImport(w, r, imp, notice)
}

// followAppleImport keeps a finished import in sync with its playlist.
func followAppleImport(w http.ResponseWriter, r *http.Request) {
	imp, ok := appleImportFor(w, r)
	if !ok {
		return
	}
	if imp.Status != db.ImportDone {
		http.Error(w, "the import is still running", http.StatusConflict)
		return
	}
	if _, err := db.FollowApplePlaylist(imp.ID); err != nil {
		serverError(w, err)
		return
	}
	renderAppleImport(w, r, imp, "Now kept in sync: new songs are added when Apple Music has them.")
}

// appleFollowFor reads the follow named by the request's {id}, answering 404
// when there is none.
func appleFollowFor(w http.ResponseWriter, r *http.Request) (*db.AppleFollow, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	f, err := db.GetAppleFollow(id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return nil, false
	}
	if err != nil {
		serverError(w, err)
		return nil, false
	}
	return f, true
}

// appleFollowRows lists the followed playlists for the Add page.
func appleFollowRows() ([]components.AppleFollowRow, error) {
	follows, err := db.ListAppleFollows()
	if err != nil {
		return nil, err
	}
	rows := make([]components.AppleFollowRow, 0, len(follows))
	for _, f := range follows {
		row := components.AppleFollowRow{
			ID: f.ID, ImportID: f.ImportID, Name: f.Name, Error: f.LastError, ToPick: f.ToPick,
		}
		if row.Name == "" {
			row.Name = f.URL
		}
		if t, err := time.Parse("2006-01-02 15:04:05", f.LastCheckedAt); err == nil {
			row.Checked = timeAgo(time.Since(t))
		}
		j, err := jobs.Latest(appleimport.JobSync, appleimport.SyncKey(f.ID))
		if err != nil {
			return nil, err
		}
		row.Checking = j != nil && (j.Status == "queued" || j.Status == "running")
		rows = append(rows, row)
	}
	return rows, nil
}

// timeAgo is d as "just now", "5 min ago", "3 h ago" or "2 days ago".
func timeAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d/time.Hour))
	}
	return fmt.Sprintf("%d days ago", int(d/(24*time.Hour)))
}

func renderAppleFollows(w http.ResponseWriter, r *http.Request) {
	rows, err := appleFollowRows()
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.AppleFollowsList(rows))
}

// getAppleFollows is what the followed list reloads while a check runs.
func getAppleFollows(w http.ResponseWriter, r *http.Request) {
	renderAppleFollows(w, r)
}

// checkAppleFollow queues a check of a followed playlist right now.
func checkAppleFollow(w http.ResponseWriter, r *http.Request) {
	f, ok := appleFollowFor(w, r)
	if !ok {
		return
	}
	if err := appleimport.EnqueueSync(f.ID); err != nil {
		serverError(w, err)
		return
	}
	renderAppleFollows(w, r)
}

// deleteAppleFollow stops syncing a playlist. Its import and what it added
// stay. Asked from an import page it answers that page, else the list.
func deleteAppleFollow(w http.ResponseWriter, r *http.Request) {
	f, ok := appleFollowFor(w, r)
	if !ok {
		return
	}
	if err := db.DeleteAppleFollow(f.ID); err != nil {
		serverError(w, err)
		return
	}
	if r.Header.Get("HX-Target") == "apple-import" {
		imp, err := db.GetAppleImport(f.ImportID)
		if err != nil {
			serverError(w, err)
			return
		}
		renderAppleImport(w, r, imp, "No longer kept in sync.")
		return
	}
	renderAppleFollows(w, r)
}
