// Package downloads fills in placeholder tracks: the download job asks the
// sources for a file, fetches the best one and attaches it to the track,
// through the same fingerprint and duration checks as an upload.
package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/jobs"
	"audioloader/internal/sources"
	"audioloader/internal/sources/folder"
	"audioloader/internal/sources/ytdlp"
)

// Downloader holds the configured sources.
type Downloader struct {
	Sources []sources.Source
	// Priority orders sources when candidates are otherwise equal.
	Priority []string
	// YT is nil when yt-dlp isn't available; YTErr then says why.
	YT    *ytdlp.Client
	YTErr error
}

var (
	defaultOnce sync.Once
	defaultD    *Downloader
)

// Default is configured from the environment: AUDIOLOADER_WATCH for the
// watched folder, YTDLP and FFMPEG for yt-dlp, and AUDIOLOADER_SOURCES for
// the priority (default "folder,yt-dlp").
func Default() *Downloader {
	defaultOnce.Do(func() {
		d := &Downloader{Priority: []string{folder.Name, ytdlp.Name}}
		if p := os.Getenv("AUDIOLOADER_SOURCES"); p != "" {
			d.Priority = strings.Split(p, ",")
		}
		if f := folder.FromEnv(); f != nil {
			d.Sources = append(d.Sources, f)
		}
		d.YT, d.YTErr = ytdlp.Default()
		if d.YT != nil {
			d.Sources = append(d.Sources, ytdlp.Source{C: d.YT})
			if v, err := d.YT.Version(context.Background()); err == nil {
				log.Println("yt-dlp", v)
			}
		} else {
			log.Println("downloads:", d.YTErr)
		}
		defaultD = d
	})
	return defaultD
}

// JobYouTubeImport downloads a YouTube video as a new track.
const JobYouTubeImport = "youtube_import"

// Register sets up the job handlers.
func Register() {
	jobs.Register(JobDownload, func(ctx context.Context, payload json.RawMessage) error {
		var p struct{ TrackID, VideoID string }
		if err := json.Unmarshal(payload, &p); err != nil {
			return jobs.Permanent(err)
		}
		return Default().Run(ctx, p.TrackID, p.VideoID)
	})
	jobs.Register(JobYouTubeImport, func(ctx context.Context, payload json.RawMessage) error {
		var imp YouTubeImport
		if err := json.Unmarshal(payload, &imp); err != nil {
			return jobs.Permanent(err)
		}
		return Default().Import(ctx, imp)
	})
}

// EnqueueVideo queues downloading a YouTube video into a placeholder track.
func EnqueueVideo(trackID, videoID string) error {
	if err := db.SetDownload(trackID, db.DownloadQueued, ytdlp.Name, 0, ""); err != nil {
		return err
	}
	return jobs.EnqueueAt(JobDownload, trackID+":"+videoID,
		map[string]string{"TrackID": trackID, "VideoID": videoID}, time.Time{})
}

// Retry queues a failed or not found download again.
func Retry(trackID string) error {
	if err := db.SetDownload(trackID, db.DownloadQueued, "", 0, ""); err != nil {
		return err
	}
	return Enqueue(trackID)
}

// Run downloads a placeholder track's file: from the given YouTube video,
// or else the best candidate any source has.
func (d *Downloader) Run(ctx context.Context, trackID, videoID string) error {
	dl, err := db.GetDownload(db.All, trackID)
	if errors.Is(err, db.ErrNotFound) {
		return nil // merged into another track or deleted
	}
	if err != nil {
		return err
	}
	if dl.Track.HasFile {
		return nil
	}

	var src sources.Source
	var cand sources.Candidate
	sourceURL := ""
	if videoID != "" {
		if d.YT == nil {
			return d.fail(trackID, ytdlp.Name, d.YTErr)
		}
		sourceURL = ytdlp.VideoURL(videoID)
		existing, err := db.TrackBySourceURL(sourceURL)
		if err != nil {
			return err
		}
		if existing != "" {
			return db.MergePlaceholder(trackID, existing)
		}
		src, cand = ytdlp.Source{C: d.YT}, ytdlp.Candidate(videoID, 0)
	} else {
		if err := db.SetDownload(trackID, db.DownloadSearching, "", 0, ""); err != nil {
			return err
		}
		var all []sources.Candidate
		for _, s := range d.Sources {
			found, err := s.Find(ctx, wantFor(dl))
			if err != nil {
				log.Printf("downloads: %s: %v", s.Name(), err)
				continue
			}
			all = append(all, found...)
		}
		if len(all) == 0 {
			return db.SetDownload(trackID, db.DownloadNotFound, "", 0, "")
		}
		sources.Rank(all, d.Priority)
		cand = all[0]
		for _, s := range d.Sources {
			if s.Name() == cand.Source {
				src = s
			}
		}
	}

	if err := db.SetDownload(trackID, db.DownloadDownloading, src.Name(), 0, ""); err != nil {
		return err
	}
	var last time.Time
	progress := func(p float64) {
		// a progress write per yt-dlp line would be many; once a second is plenty
		if time.Since(last) < time.Second && p < 1 {
			return
		}
		last = time.Now()
		if err := db.SetDownloadProgress(trackID, p); err != nil {
			log.Println("downloads: progress:", err)
		}
	}
	f, err := src.Fetch(ctx, cand, progress)
	if err != nil {
		if ctx.Err() != nil {
			return err // shutting down; the job runs again
		}
		return d.fail(trackID, src.Name(), err)
	}
	defer f.Cleanup()

	merged, err := Attach(db.All, trackID, f.Path)
	if err != nil {
		return d.fail(trackID, src.Name(), err)
	}
	if !merged && sourceURL != "" {
		if err := db.SetSourceURL(trackID, sourceURL); err != nil {
			return err
		}
	}
	if merged {
		return nil // the download row went with the placeholder
	}
	return db.SetDownload(trackID, db.DownloadDone, src.Name(), 0, "")
}

// fail records a failed download. It isn't retried automatically: a
// missing video or broken yt-dlp fails the same way again.
func (d *Downloader) fail(trackID, source string, err error) error {
	if serr := db.SetDownload(trackID, db.DownloadFailed, source, 0, err.Error()); serr != nil {
		return serr
	}
	return jobs.Permanent(err)
}

// Attach stores the file at path for a placeholder track. If the library
// already has the same file, the placeholder is merged into that track
// and merged is true; unless that track is secret and v can't see it,
// which gives db.ErrHidden.
func Attach(v db.Viewer, trackID, path string) (merged bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	return AttachReader(v, trackID, f, filepath.Ext(path))
}

// AttachReader is Attach for an open file, e.g. an upload.
func AttachReader(v db.Viewer, trackID string, f io.ReadSeeker, ext string) (merged bool, err error) {
	dur, err := audio.ProbeDuration(f)
	if err != nil {
		return false, fmt.Errorf("could not read audio duration: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	existing, err := db.AttachFile(trackID, f, ext, dur)
	if errors.Is(err, db.ErrExists) {
		if t, gerr := db.GetTrack(v, existing); gerr != nil || t == nil {
			return false, db.ErrHidden
		}
		return true, db.MergePlaceholder(trackID, existing)
	}
	if err == nil {
		err = db.SetDownload(trackID, db.DownloadDone, "", 0, "")
	}
	return false, err
}

func wantFor(dl *db.Download) sources.Want {
	w := sources.Want{Title: dl.Track.Title, MBID: dl.Track.MBID, ISRC: dl.Track.ISRC, Duration: dl.Track.Duration}
	for _, a := range dl.Track.Artists {
		w.Artists = append(w.Artists, a.Name)
	}
	if dl.Release != nil {
		w.Release = dl.Release.Title
	}
	return w
}

// Schedule looks again for "not found" downloads once a day, when a
// watched folder is set up (files may have been added to it since).
func Schedule(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(24 * time.Hour):
		}
		if os.Getenv("AUDIOLOADER_WATCH") == "" {
			continue
		}
		missing, err := db.ListDownloads(db.All, db.DownloadNotFound)
		if err != nil {
			log.Println("downloads:", err)
			continue
		}
		for _, dl := range missing {
			if err := Retry(dl.Track.ID.String()); err != nil {
				log.Println("downloads:", err)
			}
		}
	}
}
