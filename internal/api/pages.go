package api

import (
	"net/http"
	"strings"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
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
	render(w, r, components.TracksPage(tracks, cont))
}

func getUploadPage(w http.ResponseWriter, r *http.Request) {
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
	render(w, r, components.UploadPage(artists, shows))
}

func getBatchUploadPage(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}

	releases, err := db.ListReleases(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	nextPositions, err := db.NextPositions()
	if err != nil {
		serverError(w, err)
		return
	}

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
