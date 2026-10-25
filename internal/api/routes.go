package api

import (
	"net/http"
)

func Register(mux *http.ServeMux) {
	mux.Handle("GET /assets/", http.StripPrefix(
		"/assets/",
		http.FileServer(http.Dir("assets")),
	))

	mux.HandleFunc("GET /covers/{release_id}", getCover)
	mux.HandleFunc("GET /{$}", getIndex)
	mux.HandleFunc("GET /tracks", getTracksPage)
	mux.HandleFunc("GET /upload", getUploadPage)
	mux.HandleFunc("GET /upload/batch", getBatchUploadPage)
	mux.HandleFunc("GET /upload/import", getImportPage)
	mux.HandleFunc("GET /artists", getArtistsPage)
	mux.HandleFunc("GET /releases", getReleasesPage)
	mux.HandleFunc("GET /releases/{release_id}", getReleasePage)
	mux.HandleFunc("GET /releases/{release_id}/edit", getReleaseEditPage)
	mux.HandleFunc("GET /tagging", getTaggingPage)
	mux.HandleFunc("GET /inbox", getInboxPage)
	mux.HandleFunc("GET /downloads", getDownloadsPage)
	mux.HandleFunc("GET /upload/youtube", getYouTubeImportPage)
	mux.HandleFunc("GET /artists/{artist_id}/follow", getFollowPage)

	mux.HandleFunc("POST /ui/artists", createArtist)
	mux.HandleFunc("GET /ui/artists", getArtists)
	mux.HandleFunc("GET /ui/tracks", getTracks)
	mux.HandleFunc("GET /ui/tracks/{track_id}", getTrack)
	mux.HandleFunc("POST /ui/tracks", addTrack)
	mux.HandleFunc("POST /ui/releases", createRelease)
	mux.HandleFunc("POST /ui/releases/{release_id}", updateRelease)
	mux.HandleFunc("POST /ui/releases/{release_id}/delete", deleteRelease)
	mux.HandleFunc("POST /ui/releases/{release_id}/cover", uploadCover)
	mux.HandleFunc("POST /ui/releases/{release_id}/tracks", saveReleaseTracks)

	mux.HandleFunc("GET /ui/badges/tagging", getTaggingBadge)
	mux.HandleFunc("GET /ui/badges/inbox", getInboxBadge)
	mux.HandleFunc("POST /ui/artists/{artist_id}/follow", followArtist)
	mux.HandleFunc("POST /ui/artists/{artist_id}/unfollow", unfollowArtist)
	mux.HandleFunc("POST /ui/artists/{artist_id}/check", checkArtistNow)
	mux.HandleFunc("GET /ui/downloads/active", getActiveDownloads)
	mux.HandleFunc("POST /ui/downloads/{track_id}/retry", retryDownload)
	mux.HandleFunc("POST /ui/downloads/{track_id}/youtube", downloadFromYouTube)
	mux.HandleFunc("POST /ui/tracks/{track_id}/file", uploadTrackFile)
	mux.HandleFunc("POST /ui/releases/{release_id}/youtube", previewReleasePlaylist)
	mux.HandleFunc("POST /ui/releases/{release_id}/youtube/confirm", confirmReleasePlaylist)
	mux.HandleFunc("POST /ui/youtube/preview", previewYouTubeImport)
	mux.HandleFunc("POST /ui/youtube/import", importFromYouTube)
	mux.HandleFunc("POST /ui/inbox/confirm-all", confirmAllInbox)
	mux.HandleFunc("POST /ui/inbox/{release_group}/confirm", confirmInboxRelease)
	mux.HandleFunc("POST /ui/inbox/{release_group}/ignore", ignoreInboxRelease)
	mux.HandleFunc("POST /ui/tagging/run", runTagging)
	mux.HandleFunc("POST /ui/tagging/retry", retryTagging)
	mux.HandleFunc("POST /ui/tagging/apply-all", applyAllTagMatches)
	mux.HandleFunc("POST /ui/tagging/{track_id}/{recording_id}/apply", applyTagMatch)
	mux.HandleFunc("POST /ui/tagging/{track_id}/{recording_id}/reject", rejectTagMatch)

	mux.HandleFunc("GET /api/tracks/{track_id}/stream", streamTrack)
	mux.HandleFunc("POST /api/tracks/exists", checkTracksExist)
	mux.HandleFunc("POST /api/artists/resolve", resolveArtists)
	mux.HandleFunc("POST /api/releases", createReleaseJSON)
}
