package api

import (
	"errors"
	"net/http"

	"audioloader/internal/db"
)

// The browse endpoints of the JSON API: each answers with what the HTML page
// of the same name reads, through the request's viewer.

// found writes a 404 for db.ErrNotFound and a 500 for any other error, and
// reports whether there was one.
func found(w http.ResponseWriter, r *http.Request, err error) bool {
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

func getArtistsJSON(w http.ResponseWriter, r *http.Request) {
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	limit, offset := pageParams(r)
	writeJSON(w, http.StatusOK, listOf(artistsJSON(viewer(r), artists), limit, offset))
}

func getArtistJSON(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok {
		return
	}
	v := viewer(r)
	artist, err := db.GetArtist(v, id.String())
	if !found(w, r, err) {
		return
	}
	d := jsonArtistDetail{jsonArtist: artistJSON(v, artist), Releases: []jsonRelease{}}
	// secret tracks are on no release, so only normal mode has releases
	if !v.IsSecret() {
		releases, err := db.ListArtistReleases(v, id.String())
		if err != nil {
			serverError(w, err)
			return
		}
		d.Releases = releasesJSON(v, releases)
	}
	tracks, err := db.ListArtistTracks(v, id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	d.Tracks = tracksJSON(v, tracks)
	writeJSON(w, http.StatusOK, d)
}

func getReleasesJSON(w http.ResponseWriter, r *http.Request) {
	releases, err := db.ListReleases(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	limit, offset := pageParams(r)
	writeJSON(w, http.StatusOK, listOf(releasesJSON(viewer(r), releases), limit, offset))
}

func getReleaseJSON(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "release_id")
	if !ok {
		return
	}
	v := viewer(r)
	release, err := db.GetRelease(v, id.String())
	if !found(w, r, err) {
		return
	}
	d := jsonReleaseDetail{
		jsonRelease: releaseJSON(v, release),
		Duration:    release.Duration().Seconds(),
		Tracks:      make([]jsonReleaseTrack, len(release.Tracks)),
	}
	for i, rt := range release.Tracks {
		d.Tracks[i] = jsonReleaseTrack{Disc: rt.Disc, Number: rt.Position, Track: trackJSON(v, rt.Track)}
	}
	writeJSON(w, http.StatusOK, d)
}

func getPlaylistsJSON(w http.ResponseWriter, r *http.Request) {
	playlists, err := db.ListPlaylists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	limit, offset := pageParams(r)
	writeJSON(w, http.StatusOK, listOf(playlistsJSON(viewer(r), playlists), limit, offset))
}

func getPlaylistJSON(w http.ResponseWriter, r *http.Request) {
	id, ok := playlistID(w, r)
	if !ok {
		return
	}
	v := viewer(r)
	p, err := db.GetPlaylist(v, id)
	if !found(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, jsonPlaylistDetail{jsonPlaylist: playlistJSON(v, p), Tracks: tracksJSON(v, p.Tracks)})
}

func getShowsJSON(w http.ResponseWriter, r *http.Request) {
	shows, err := db.ListShows(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	limit, offset := pageParams(r)
	writeJSON(w, http.StatusOK, listOf(showsJSON(viewer(r), shows), limit, offset))
}

func getShowJSON(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "show_id")
	if !ok {
		return
	}
	v := viewer(r)
	s, err := db.GetShow(v, id.String())
	if !found(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, jsonShowDetail{jsonShow: showJSON(v, s), Episodes: tracksJSON(v, s.Episodes)})
}
