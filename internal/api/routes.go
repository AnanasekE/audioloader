package api

import (
	"mime"
	"net/http"
	"time"

	"audioloader/internal/sources/secret"
	"audioloader/internal/vault"
)

// Register adds the routes. v unlocks secret tracks; nil turns them off.
func Register(mux *http.ServeMux, v *vault.Vault) {
	theVault = v
	Routes = nil
	NormalOnly = nil
	SecretOnly = nil
	SecretModeOnly = nil
	TokenOnly = nil
	// the file server's type table doesn't know the web app manifest
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
	mux.Handle("GET /assets/", http.StripPrefix(
		"/assets/",
		http.FileServer(http.Dir("assets")),
	))

	handleNormal(mux, "GET /covers/{release_id}", getCover)
	handle(mux, "GET /artist-images/{artist_id}", getArtistImage)
	handle(mux, "GET /track-images/{track_id}", getTrackImage)
	handle(mux, "GET /{$}", getIndex)
	handle(mux, "GET /vault", getVaultPage)
	handle(mux, "POST /vault", unlockVault)
	handle(mux, "POST /vault/lock", lockVault)
	handle(mux, "GET /tracks", getTracksPage)
	handle(mux, "GET /tracks/{track_id}/edit", getTrackEditPage)
	handle(mux, "GET /upload", redirectUpload)
	handle(mux, "GET /add", getAddPage)
	handle(mux, "GET /upload/batch", getBatchUploadPage)
	handle(mux, "GET /upload/import", getImportPage)
	handle(mux, "GET /artists", getArtistsPage)
	handleSecretMode(mux, "GET /tags", getTagsPage)
	handleNormal(mux, "GET /releases", getReleasesPage)
	handleNormal(mux, "GET /releases/{release_id}", getReleasePage)
	handleNormal(mux, "GET /releases/{release_id}/edit", getReleaseEditPage)
	handleNormal(mux, "GET /tagging", getTaggingPage)
	handleNormal(mux, "GET /inbox", getInboxPage)
	handle(mux, "GET /search", getSearchPage)
	handle(mux, "GET /shows", getShowsPage)
	handle(mux, "GET /shows/{show_id}", getShowPage)
	handle(mux, "GET /playlists", getPlaylistsPage)
	handle(mux, "GET /playlists/{playlist_id}", getPlaylistPage)
	handle(mux, "GET /playlists/{playlist_id}/edit", getPlaylistPage)
	handleNormal(mux, "GET /downloads", getDownloadsPage)
	handleNormal(mux, "GET /upload/youtube", redirectUploadURL)
	handleSecret(mux, "GET /upload/secret-source", redirectUploadURL)
	handle(mux, "GET /artists/{artist_id}", getArtistPage)
	handleNormal(mux, "GET /artists/{artist_id}/follow", getFollowPage)

	handle(mux, "POST /ui/artists", createArtist)
	handle(mux, "GET /ui/artists", getArtists)
	handleNormal(mux, "GET /ui/artists/musicbrainz", searchArtistsMusicBrainz)
	handleNormal(mux, "POST /ui/artists/musicbrainz", addArtistFromMusicBrainz)
	handleNormal(mux, "GET /ui/musicbrainz/artists/{mbid}/releases", getCandidateReleases)
	handle(mux, "GET /ui/tracks", getTracks)
	handleNormal(mux, "GET /ui/tracks/pick", pickCoverTracks)
	handle(mux, "GET /ui/tracks/{track_id}", getTrack)
	handle(mux, "POST /ui/tracks", addTrack)
	handle(mux, "POST /ui/tracks/{track_id}", updateTrack)
	handle(mux, "POST /ui/tracks/{track_id}/delete", deleteTrack)
	handle(mux, "GET /ui/tracks/{track_id}/trim", previewTrim)
	handle(mux, "POST /ui/tracks/{track_id}/trim", applyTrim)
	handle(mux, "POST /ui/tracks/{track_id}/image", uploadTrackImage)
	handle(mux, "POST /ui/tracks/{track_id}/image/remove", removeTrackImage)
	handleNormal(mux, "POST /ui/releases", createRelease)
	handleNormal(mux, "POST /ui/releases/{release_id}", updateRelease)
	handleNormal(mux, "POST /ui/releases/{release_id}/delete", deleteRelease)
	handleNormal(mux, "POST /ui/releases/{release_id}/remove", removeRelease)
	handleNormal(mux, "POST /ui/releases/{release_id}/cover", uploadCover)
	handleNormal(mux, "POST /ui/releases/{release_id}/tracks", saveReleaseTracks)

	handle(mux, "GET /ui/search", getSearchSuggest)
	handle(mux, "POST /ui/episodes", addEpisode)
	handle(mux, "POST /ui/tracks/{track_id}/played", markPlayed)
	handle(mux, "POST /ui/tracks/{track_id}/unplayed", markPlayed)
	handle(mux, "GET /ui/playlists/picker", getPlaylistPicker)
	handle(mux, "POST /ui/playlists", createPlaylist)
	handle(mux, "POST /ui/playlists/{playlist_id}/rename", renamePlaylist)
	handle(mux, "POST /ui/playlists/{playlist_id}/delete", deletePlaylist)
	handle(mux, "POST /ui/playlists/{playlist_id}/tracks", savePlaylistTracks)
	handle(mux, "POST /ui/playlists/{playlist_id}/tracks/add", addToPlaylist)
	handleNormal(mux, "GET /ui/badges/tagging", getTaggingBadge)
	handleNormal(mux, "GET /ui/badges/inbox", getInboxBadge)
	handleNormal(mux, "GET /ui/badges/more", getMoreBadge)
	handleNormal(mux, "GET /ui/inbox/{release_group}", getInboxDetails)
	handleNormal(mux, "GET /ui/badges/downloads", getDownloadsBadge)
	handle(mux, "POST /ui/artists/{artist_id}/rename", renameArtist)
	handle(mux, "POST /ui/artists/{artist_id}/merge", mergeArtist)
	handleNormal(mux, "GET /ui/artists/{artist_id}/link", getArtistLink)
	handleNormal(mux, "POST /ui/artists/{artist_id}/link", linkArtist)
	handle(mux, "POST /ui/artists/{artist_id}/image", uploadArtistImage)
	handle(mux, "POST /ui/artists/{artist_id}/image/remove", removeArtistImage)
	handle(mux, "POST /ui/artists/{artist_id}/twitch", setArtistTwitch)
	handle(mux, "POST /ui/artists/{artist_id}/twitch/picture", updateArtistTwitchPicture)
	handleNormal(mux, "POST /ui/artists/{artist_id}/follow", followArtist)
	handleNormal(mux, "POST /ui/artists/{artist_id}/unfollow", unfollowArtist)
	handleNormal(mux, "POST /ui/artists/{artist_id}/check", checkArtistNow)
	handleNormal(mux, "POST /ui/follows/check", checkFollowsNow)
	handleNormal(mux, "GET /ui/downloads/active", getActiveDownloads)
	handleNormal(mux, "POST /ui/downloads/{track_id}/retry", retryDownload)
	handleNormal(mux, "POST /ui/downloads/{track_id}/youtube", downloadFromYouTube)
	handleNormal(mux, "GET /ui/downloads/{track_id}/youtube/search", searchYouTubeForTrack)
	handleNormal(mux, "POST /ui/downloads/{track_id}/youtube/none", noYouTubeMatch)
	handleNormal(mux, "POST /ui/tracks/{track_id}/file", uploadTrackFile)
	handleNormal(mux, "POST /ui/releases/{release_id}/youtube", previewReleasePlaylist)
	handleNormal(mux, "POST /ui/releases/{release_id}/youtube/confirm", confirmReleasePlaylist)
	handleSecret(mux, "POST /ui/secret-source/check", checkSecretSource)
	handleSecret(mux, "POST /ui/secret-source/download", downloadFromSecretSource)
	handleSecret(mux, "GET /ui/secret-source/status", getSecretSourceStatus)
	handle(mux, "POST /ui/add/lookup", lookupAdd)
	handleNormal(mux, "GET /add/find", getFindPage)
	handleNormal(mux, "GET /ui/add/find/results", getFindResults)
	handleNormal(mux, "GET /add/find/artist/{mbid}", getFindArtist)
	handleNormal(mux, "GET /ui/add/find/artist/{mbid}/groups", getFindArtistGroups)
	handleNormal(mux, "GET /add/find/release-group/{mbid}", getFindReleaseGroup)
	handleNormal(mux, "POST /ui/add/find/song", addFindSong)
	handleNormal(mux, "POST /ui/add/find/release", addFindRelease)
	handleNormal(mux, "GET /ui/add/musicbrainz", searchSongsMusicBrainz)
	handleNormal(mux, "POST /ui/add/musicbrainz", addSongFromMusicBrainz)
	handleNormal(mux, "POST /ui/add/musicbrainz/release", addReleaseFromMusicBrainz)
	handleNormal(mux, "GET /add/apple/{id}", getAppleImportPage)
	handleNormal(mux, "GET /ui/add/apple/{id}", getAppleImportBody)
	handleNormal(mux, "POST /ui/add/apple/{id}/add", addAppleImport)
	handleNormal(mux, "POST /ui/add/apple/{id}/follow", followAppleImport)
	handleNormal(mux, "GET /ui/add/apple-follows", getAppleFollows)
	handleNormal(mux, "POST /ui/add/apple-follows/{id}/check", checkAppleFollow)
	handleNormal(mux, "POST /ui/add/apple-follows/{id}/delete", deleteAppleFollow)
	handleNormal(mux, "GET /ui/add/apple/{id}/{position}/candidates", getAppleCandidates)
	handleNormal(mux, "POST /ui/add/apple/{id}/{position}/pick", pickAppleCandidate)
	handleNormal(mux, "GET /ui/add/youtube", searchSongsYouTube)
	handleNormal(mux, "POST /ui/add/youtube", addSongFromYouTube)
	handleNormal(mux, "POST /ui/youtube/preview", previewYouTubeImport)
	handleNormal(mux, "POST /ui/youtube/import", importFromYouTube)
	handleNormal(mux, "POST /ui/inbox/confirm-all", confirmAllInbox)
	handleNormal(mux, "POST /ui/inbox/{release_group}/confirm", confirmInboxRelease)
	handleNormal(mux, "POST /ui/inbox/{release_group}/ignore", ignoreInboxRelease)
	handleNormal(mux, "POST /ui/inbox/{release_group}/restore", restoreInboxRelease)
	handleNormal(mux, "POST /ui/tagging/run", runTagging)
	handleNormal(mux, "POST /ui/tagging/retry", retryTagging)
	handleNormal(mux, "POST /ui/tagging/apply-all", applyAllTagMatches)
	handleNormal(mux, "POST /ui/tagging/{track_id}/move", moveTrack)
	handleNormal(mux, "POST /ui/tagging/{track_id}/move/dismiss", dismissMove)
	handleNormal(mux, "POST /ui/tagging/{track_id}/cover-of/apply", applyCoverSuggestion)
	handleNormal(mux, "POST /ui/tagging/{track_id}/cover-of/reject", rejectCoverSuggestion)
	handleNormal(mux, "POST /ui/tagging/{track_id}/{recording_id}/apply", applyTagMatch)
	handleNormal(mux, "POST /ui/tagging/{track_id}/{recording_id}/reject", rejectTagMatch)

	handle(mux, "GET /api/tracks/{track_id}/stream", streamTrack)
	handle(mux, "GET /api/tracks/{track_id}/chapters", getChapters)
	handle(mux, "POST /api/tracks/{track_id}/position", savePosition)
	handle(mux, "POST /api/tracks/exists", checkTracksExist)
	handle(mux, "POST /api/artists/resolve", resolveArtists)
	handleNormal(mux, "POST /api/releases", createReleaseJSON)
	handleToken(mux, "POST /api/share", share)

	registerV1(mux)
}

// Routes lists every registered pattern, e.g. "GET /tracks", for tests
// that check all of them.
var Routes []string

// NormalOnly lists the patterns that exist in normal mode only: releases,
// the inbox, tagging and downloads work on things secret tracks don't have.
// In secret mode they answer 404.
var NormalOnly []string

// SecretOnly lists the patterns that exist in secret mode only, and only
// when a secret source is configured. Anywhere else they answer 404.
var SecretOnly []string

// SecretModeOnly lists the patterns that exist in secret mode only, source or not.
var SecretModeOnly []string

// TokenOnly lists the patterns that the share token opens instead of a
// session: no cookie is read, and the handler runs as the secret viewer.
var TokenOnly []string

// handleToken registers a route for the share token. It answers 404 while no
// token is set, and 401 (after a second, like a wrong passphrase) without
// the right token. It needs no cookie, and ignores one; h picks the viewer.
func handleToken(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	TokenOnly = append(TokenOnly, pattern)
	handle(mux, pattern, func(w http.ResponseWriter, r *http.Request) {
		if theShareToken == "" {
			http.NotFound(w, r)
			return
		}
		if !shareToken(r) {
			time.Sleep(shareFailDelay)
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		h(w, r)
	})
}

// handleSecretMode registers a route that exists in secret mode only; unlike
// handleSecret it doesn't need a secret source to be configured.
func handleSecretMode(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	SecretModeOnly = append(SecretModeOnly, pattern)
	handle(mux, pattern, func(w http.ResponseWriter, r *http.Request) {
		if !viewer(r).IsSecret() {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	})
}

// handleSecret registers a route that needs secret mode and a secret source.
func handleSecret(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	SecretOnly = append(SecretOnly, pattern)
	handle(mux, pattern, func(w http.ResponseWriter, r *http.Request) {
		if !viewer(r).IsSecret() || secret.Default() == nil {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	})
}

// handleNormal registers a route that exists in normal mode only.
func handleNormal(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	NormalOnly = append(NormalOnly, pattern)
	handle(mux, pattern, func(w http.ResponseWriter, r *http.Request) {
		if viewer(r).IsSecret() {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	})
}

func handle(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	Routes = append(Routes, pattern)
	mux.HandleFunc(pattern, h)
}
