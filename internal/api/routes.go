package api

import (
	"net/http"

	"audioloader/internal/vault"
)

// Register adds the routes. v unlocks secret tracks; nil turns them off.
func Register(mux *http.ServeMux, v *vault.Vault) {
	theVault = v
	Routes = nil
	mux.Handle("GET /assets/", http.StripPrefix(
		"/assets/",
		http.FileServer(http.Dir("assets")),
	))

	handle(mux, "GET /covers/{release_id}", getCover)
	handle(mux, "GET /artist-images/{artist_id}", getArtistImage)
	handle(mux, "GET /{$}", getIndex)
	handle(mux, "GET /vault", getVaultPage)
	handle(mux, "POST /vault", unlockVault)
	handle(mux, "POST /vault/lock", lockVault)
	handle(mux, "GET /tracks", getTracksPage)
	handle(mux, "GET /tracks/{track_id}/edit", getTrackEditPage)
	handle(mux, "GET /upload", getUploadPage)
	handle(mux, "GET /upload/batch", getBatchUploadPage)
	handle(mux, "GET /upload/import", getImportPage)
	handle(mux, "GET /artists", getArtistsPage)
	handle(mux, "GET /releases", getReleasesPage)
	handle(mux, "GET /releases/{release_id}", getReleasePage)
	handle(mux, "GET /releases/{release_id}/edit", getReleaseEditPage)
	handle(mux, "GET /tagging", getTaggingPage)
	handle(mux, "GET /inbox", getInboxPage)
	handle(mux, "GET /search", getSearchPage)
	handle(mux, "GET /shows", getShowsPage)
	handle(mux, "GET /shows/{show_id}", getShowPage)
	handle(mux, "GET /playlists", getPlaylistsPage)
	handle(mux, "GET /playlists/{playlist_id}", getPlaylistPage)
	handle(mux, "GET /playlists/{playlist_id}/edit", getPlaylistPage)
	handle(mux, "GET /downloads", getDownloadsPage)
	handle(mux, "GET /upload/youtube", getYouTubeImportPage)
	handle(mux, "GET /artists/{artist_id}", getArtistPage)
	handle(mux, "GET /artists/{artist_id}/follow", getFollowPage)

	handle(mux, "POST /ui/artists", createArtist)
	handle(mux, "GET /ui/artists", getArtists)
	handle(mux, "GET /ui/musicbrainz/artists/{mbid}/releases", getCandidateReleases)
	handle(mux, "GET /ui/tracks", getTracks)
	handle(mux, "GET /ui/tracks/pick", pickCoverTracks)
	handle(mux, "GET /ui/tracks/{track_id}", getTrack)
	handle(mux, "POST /ui/tracks", addTrack)
	handle(mux, "POST /ui/tracks/{track_id}", updateTrack)
	handle(mux, "POST /ui/tracks/{track_id}/delete", deleteTrack)
	handle(mux, "POST /ui/releases", createRelease)
	handle(mux, "POST /ui/releases/{release_id}", updateRelease)
	handle(mux, "POST /ui/releases/{release_id}/delete", deleteRelease)
	handle(mux, "POST /ui/releases/{release_id}/cover", uploadCover)
	handle(mux, "POST /ui/releases/{release_id}/tracks", saveReleaseTracks)

	handle(mux, "GET /ui/search", getSearchSuggest)
	handle(mux, "POST /ui/episodes", addEpisode)
	handle(mux, "POST /ui/tracks/{track_id}/secret", setTrackSecret)
	handle(mux, "POST /ui/tracks/{track_id}/played", markPlayed)
	handle(mux, "POST /ui/tracks/{track_id}/unplayed", markPlayed)
	handle(mux, "GET /ui/playlists/picker", getPlaylistPicker)
	handle(mux, "POST /ui/playlists", createPlaylist)
	handle(mux, "POST /ui/playlists/{playlist_id}/rename", renamePlaylist)
	handle(mux, "POST /ui/playlists/{playlist_id}/delete", deletePlaylist)
	handle(mux, "POST /ui/playlists/{playlist_id}/tracks", savePlaylistTracks)
	handle(mux, "POST /ui/playlists/{playlist_id}/tracks/add", addToPlaylist)
	handle(mux, "GET /ui/badges/tagging", getTaggingBadge)
	handle(mux, "GET /ui/badges/inbox", getInboxBadge)
	handle(mux, "GET /ui/badges/downloads", getDownloadsBadge)
	handle(mux, "POST /ui/artists/{artist_id}/rename", renameArtist)
	handle(mux, "POST /ui/artists/{artist_id}/merge", mergeArtist)
	handle(mux, "GET /ui/artists/{artist_id}/link", getArtistLink)
	handle(mux, "POST /ui/artists/{artist_id}/link", linkArtist)
	handle(mux, "POST /ui/artists/{artist_id}/image", uploadArtistImage)
	handle(mux, "POST /ui/artists/{artist_id}/image/remove", removeArtistImage)
	handle(mux, "POST /ui/artists/{artist_id}/twitch", setArtistTwitch)
	handle(mux, "POST /ui/artists/{artist_id}/twitch/picture", updateArtistTwitchPicture)
	handle(mux, "POST /ui/artists/{artist_id}/follow", followArtist)
	handle(mux, "POST /ui/artists/{artist_id}/unfollow", unfollowArtist)
	handle(mux, "POST /ui/artists/{artist_id}/check", checkArtistNow)
	handle(mux, "GET /ui/downloads/active", getActiveDownloads)
	handle(mux, "POST /ui/downloads/{track_id}/retry", retryDownload)
	handle(mux, "POST /ui/downloads/{track_id}/youtube", downloadFromYouTube)
	handle(mux, "GET /ui/downloads/{track_id}/youtube/search", searchYouTubeForTrack)
	handle(mux, "POST /ui/downloads/{track_id}/youtube/none", noYouTubeMatch)
	handle(mux, "POST /ui/tracks/{track_id}/file", uploadTrackFile)
	handle(mux, "POST /ui/releases/{release_id}/youtube", previewReleasePlaylist)
	handle(mux, "POST /ui/releases/{release_id}/youtube/confirm", confirmReleasePlaylist)
	handle(mux, "POST /ui/youtube/preview", previewYouTubeImport)
	handle(mux, "POST /ui/youtube/import", importFromYouTube)
	handle(mux, "POST /ui/inbox/confirm-all", confirmAllInbox)
	handle(mux, "POST /ui/inbox/{release_group}/confirm", confirmInboxRelease)
	handle(mux, "POST /ui/inbox/{release_group}/ignore", ignoreInboxRelease)
	handle(mux, "POST /ui/tagging/run", runTagging)
	handle(mux, "POST /ui/tagging/retry", retryTagging)
	handle(mux, "POST /ui/tagging/apply-all", applyAllTagMatches)
	handle(mux, "POST /ui/tagging/{track_id}/cover-of/apply", applyCoverSuggestion)
	handle(mux, "POST /ui/tagging/{track_id}/cover-of/reject", rejectCoverSuggestion)
	handle(mux, "POST /ui/tagging/{track_id}/{recording_id}/apply", applyTagMatch)
	handle(mux, "POST /ui/tagging/{track_id}/{recording_id}/reject", rejectTagMatch)

	handle(mux, "GET /api/tracks/{track_id}/stream", streamTrack)
	handle(mux, "GET /api/tracks/{track_id}/chapters", getChapters)
	handle(mux, "POST /api/tracks/{track_id}/position", savePosition)
	handle(mux, "POST /api/tracks/exists", checkTracksExist)
	handle(mux, "POST /api/artists/resolve", resolveArtists)
	handle(mux, "POST /api/releases", createReleaseJSON)
}

// Routes lists every registered pattern, e.g. "GET /tracks", for tests
// that check all of them.
var Routes []string

func handle(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	Routes = append(Routes, pattern)
	mux.HandleFunc(pattern, h)
}
