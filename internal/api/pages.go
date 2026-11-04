package api

import (
	"net/http"
	"net/url"
	"strings"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
)

func getIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/tracks", http.StatusSeeOther)
}

func getTracksPage(w http.ResponseWriter, r *http.Request) {
	tracks, err := db.ListTracks(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	cont, err := db.ContinueListening(viewer(r), 5)
	if err != nil {
		serverError(w, err)
		return
	}
	sortBy := trackSort(r)
	if !viewer(r).IsSecret() {
		// secret mode groups its list in the page; here the list is flat
		switch sortBy {
		case components.SortTitle:
			sortTracksBy(tracks, func(t *audio.Track) string { return t.Title })
		case components.SortCreator:
			sortTracksBy(tracks, func(t *audio.Track) string {
				if len(t.Artists) == 0 {
					return ""
				}
				return t.Artists[0].Name
			})
		}
	}
	render(w, r, components.TracksPage(tracks, cont, sortBy))
}

// trackSort is the sort order the track list asks for. "Added" is the
// default: the order the library lists the tracks in.
func trackSort(r *http.Request) string {
	switch s := r.URL.Query().Get("sort"); s {
	case components.SortCreator, components.SortTag, components.SortTitle:
		return s
	}
	return components.SortAdded
}

// getAddPage is the one Add page. A q fills the box and looks it up on load.
func getAddPage(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	shows, err := db.ListShows(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	var recent []jobs.Job
	var apple []*db.AppleImport
	if !viewer(r).IsSecret() {
		if recent, err = jobs.Recent(downloads.JobYouTubeImport, 20); err != nil {
			serverError(w, err)
			return
		}
		if apple, err = db.ListAppleImports(recentAppleImports); err != nil {
			serverError(w, err)
			return
		}
	}
	render(w, r, components.AddPage(strings.TrimSpace(r.URL.Query().Get("q")), artists, shows, recent, apple))
}

// redirectUpload sends the old pages to /add, with the query string of
// /upload as it was, or with the address of the old download pages as q.
func redirectUpload(w http.ResponseWriter, r *http.Request) {
	target := "/add"
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// redirectUploadURL is redirectUpload for /upload/youtube and
// /upload/secret-source, whose address was in "url".
func redirectUploadURL(w http.ResponseWriter, r *http.Request) {
	target := "/add"
	if u := strings.TrimSpace(r.URL.Query().Get("url")); u != "" {
		target += "?q=" + url.QueryEscape(u)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func getBatchUploadPage(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}

	releases, err := uploadTargets(r)
	if err != nil {
		serverError(w, err)
		return
	}
	nextPositions, err := db.NextPositions()
	if err != nil {
		serverError(w, err)
		return
	}
	// only the releases offered: the others' positions are none of the page's business
	offered := map[string]map[int]int{}
	for _, rel := range releases {
		if p, ok := nextPositions[rel.ID.String()]; ok {
			offered[rel.ID.String()] = p
		}
	}
	nextPositions = offered

	// ?release=<id> preselects that release for every row; its first artist
	// becomes the default artist
	var release *audio.Release
	if id := r.URL.Query().Get("release"); id != "" {
		for _, rel := range releases {
			if rel.ID.String() == id {
				release = rel
			}
		}
		if release == nil {
			http.NotFound(w, r)
			return
		}
	}

	wanted := r.URL.Query().Get("artist")
	if wanted == "" && release != nil && len(release.Artists) > 0 {
		wanted = release.Artists[0].ID.String()
	}
	defaultArtist := ""
	for _, a := range artists {
		if a.ID.String() == wanted {
			defaultArtist = a.ID.String()
		}
	}
	render(w, r, components.BatchUploadPage(artists, defaultArtist, releases, release, nextPositions))
}

func getArtistsPage(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.ArtistsPage(artists))
}

func getImportPage(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.ImportPage(artists))
}

func getSearchPage(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	res, err := db.Search(viewer(r), q, 50)
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.SearchPage(q, res))
}

func getSearchSuggest(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	res, err := db.Search(viewer(r), q, 5)
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.SearchSuggest(q, res))
}

// uploadTargets are the releases tracks can be uploaded onto. For a locked
// viewer that leaves out releases with secret tracks: their next free
// position would give the hidden tracks away.
func uploadTargets(r *http.Request) ([]*audio.Release, error) {
	releases, err := db.ListReleases(viewer(r))
	if err != nil || viewer(r).IsSecret() {
		return releases, err
	}
	withSecrets, err := db.ReleasesWithSecrets()
	if err != nil {
		return nil, err
	}
	out := releases[:0]
	for _, rel := range releases {
		if !withSecrets[rel.ID.String()] {
			out = append(out, rel)
		}
	}
	return out, nil
}
