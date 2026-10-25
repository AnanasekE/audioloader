package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/sources/ytdlp"
)

var activeDownloads = []string{db.DownloadQueued, db.DownloadSearching, db.DownloadDownloading}

func getDownloadsPage(w http.ResponseWriter, r *http.Request) {
	active, err := db.ListDownloads(activeDownloads...)
	if err != nil {
		serverError(w, err)
		return
	}
	failed, err := db.ListDownloads(db.DownloadFailed)
	if err != nil {
		serverError(w, err)
		return
	}
	notFound, err := db.ListDownloads(db.DownloadNotFound)
	if err != nil {
		serverError(w, err)
		return
	}
	ytErr := ""
	if d := downloads.Default(); d.YTErr != nil {
		ytErr = d.YTErr.Error()
	}
	render(w, r, components.DownloadsPage(active, failed, notFound, ytErr))
}

func getActiveDownloads(w http.ResponseWriter, r *http.Request) {
	active, err := db.ListDownloads(activeDownloads...)
	if err != nil {
		serverError(w, err)
		return
	}
	if len(active) == 0 {
		// the finished ones moved to other sections; reload to show them
		hxLocation(w, "/downloads")
		return
	}
	render(w, r, components.DownloadsActive(active))
}

func retryDownload(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}
	if err := downloads.Retry(id.String()); err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/downloads")
}

// downloadFromYouTube downloads a pasted video into a placeholder track.
func downloadFromYouTube(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}
	ref, err := ytdlp.Parse(r.FormValue("url"))
	if err != nil || ref.Video == "" {
		http.Error(w, "paste the URL of one YouTube video", http.StatusBadRequest)
		return
	}
	if _, err := db.GetDownload(id.String()); errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	if err := downloads.EnqueueVideo(id.String(), ref.Video); err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/downloads")
}

// uploadTrackFile uploads the file of a placeholder track by hand.
func uploadTrackFile(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}
	file, header, err := r.FormFile("track")
	if err != nil {
		http.Error(w, "missing or invalid track file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	_, err = downloads.AttachReader(id.String(), file, filepath.Ext(header.Filename))
	switch {
	case errors.Is(err, db.ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, db.ErrHasFile):
		http.Error(w, "this track already has a file", http.StatusConflict)
		return
	case err != nil && strings.HasPrefix(err.Error(), "could not read audio duration"):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	hxLocation(w, "/downloads")
}

// previewReleasePlaylist reads a YouTube playlist and proposes which video
// fills which of the release's missing tracks.
func previewReleasePlaylist(w http.ResponseWriter, r *http.Request) {
	release, ok := loadRelease(w, r)
	if !ok {
		return
	}
	ref, err := ytdlp.Parse(r.FormValue("url"))
	if err != nil || ref.Playlist == "" {
		http.Error(w, "paste the URL of a YouTube playlist", http.StatusBadRequest)
		return
	}
	d := downloads.Default()
	if d.YT == nil {
		http.Error(w, d.YTErr.Error(), http.StatusServiceUnavailable)
		return
	}
	info, err := d.YT.Info(r.Context(), ref.PlaylistURL())
	if err != nil {
		http.Error(w, "couldn't read the playlist: "+err.Error(), http.StatusBadGateway)
		return
	}
	videos := info.Videos()
	render(w, r, components.PlaylistMapping(release, downloads.MapPlaylist(release, videos), videos))
}

func confirmReleasePlaylist(w http.ResponseWriter, r *http.Request) {
	release, ok := loadRelease(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	trackIDs, videoIDs := r.PostForm["track_id"], r.PostForm["video_id"]
	if len(trackIDs) != len(videoIDs) {
		http.Error(w, "every track needs a video choice", http.StatusBadRequest)
		return
	}
	missing := map[string]bool{}
	for _, rt := range release.Tracks {
		if !rt.Track.HasFile {
			missing[rt.Track.ID.String()] = true
		}
	}
	for i, tid := range trackIDs {
		if videoIDs[i] == "" || !missing[tid] {
			continue
		}
		ref, err := ytdlp.Parse(ytdlp.VideoURL(videoIDs[i]))
		if err != nil {
			http.Error(w, "invalid video id", http.StatusBadRequest)
			return
		}
		if err := downloads.EnqueueVideo(tid, ref.Video); err != nil {
			serverError(w, err)
			return
		}
	}
	hxLocation(w, "/downloads")
}

func getYouTubeImportPage(w http.ResponseWriter, r *http.Request) {
	recent, err := jobs.Recent(downloads.JobYouTubeImport, 20)
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.YouTubeImportPage(recent))
}

func previewYouTubeImport(w http.ResponseWriter, r *http.Request) {
	ref, err := ytdlp.Parse(r.FormValue("url"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	d := downloads.Default()
	if d.YT == nil {
		http.Error(w, d.YTErr.Error(), http.StatusServiceUnavailable)
		return
	}
	// a URL with both reads the playlist: that's what "list=" asks for
	u := ref.VideoURL()
	if ref.Playlist != "" {
		u = ref.PlaylistURL()
	}
	info, err := d.YT.Info(r.Context(), u)
	if err != nil {
		http.Error(w, "couldn't read the URL: "+err.Error(), http.StatusBadGateway)
		return
	}
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
	var names []string
	for _, a := range artists {
		names = append(names, a.Name)
	}
	videos := info.Videos()
	titles := make([]string, len(videos))
	for i, v := range videos {
		titles[i] = downloads.CleanTitle(v.Title, names)
	}
	render(w, r, components.YouTubePreview(info, titles, artists, releases))
}

func importFromYouTube(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	videoIDs := r.PostForm["video_id"]
	artistIDs := r.PostForm["artist_id"]
	releaseID := r.PostForm.Get("release_id")
	if len(videoIDs) == 0 {
		http.Error(w, "pick at least one video", http.StatusBadRequest)
		return
	}
	if len(artistIDs) == 0 {
		http.Error(w, "pick at least one artist", http.StatusBadRequest)
		return
	}
	for _, id := range artistIDs {
		if validMBID(id) == "" { // validMBID checks for a UUID, which artist IDs are too
			http.Error(w, "invalid artist id", http.StatusBadRequest)
			return
		}
	}

	next := 1
	if releaseID != "" {
		if validMBID(releaseID) == "" {
			http.Error(w, "invalid release id", http.StatusBadRequest)
			return
		}
		positions, err := db.NextPositions()
		if err != nil {
			serverError(w, err)
			return
		}
		if p := positions[releaseID][1]; p > 0 {
			next = p
		}
	}
	for _, vid := range videoIDs {
		ref, err := ytdlp.Parse(ytdlp.VideoURL(vid))
		if err != nil {
			http.Error(w, "invalid video id", http.StatusBadRequest)
			return
		}
		title := strings.TrimSpace(r.PostForm.Get("title_" + vid))
		if title == "" {
			http.Error(w, "every video needs a title", http.StatusBadRequest)
			return
		}
		imp := downloads.YouTubeImport{VideoID: ref.Video, Title: title, ArtistIDs: artistIDs}
		if releaseID != "" {
			imp.ReleaseID, imp.Disc, imp.Position = releaseID, 1, next
			next++
		}
		if err := downloads.EnqueueImport(imp); err != nil {
			serverError(w, err)
			return
		}
	}
	hxLocation(w, "/upload/youtube")
}
