package api

import (
	"net/http"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
)

func getIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/tracks", http.StatusSeeOther)
}

func getTracksPage(w http.ResponseWriter, r *http.Request) {
	tracks, err := db.ListTracks()
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.TracksPage(tracks))
}

func getUploadPage(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists()
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.UploadPage(artists))
}

func getBatchUploadPage(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists()
	if err != nil {
		serverError(w, err)
		return
	}

	releases, err := db.ListReleases()
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
	artists, err := db.ListArtists()
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.ArtistsPage(artists))
}

func getImportPage(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists()
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.ImportPage(artists))
}
