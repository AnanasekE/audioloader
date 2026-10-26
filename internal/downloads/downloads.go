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
	"slices"
	"strings"
	"sync"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/jobs"
	"audioloader/internal/sources"
	"audioloader/internal/sources/folder"
	"audioloader/internal/sources/spotdl"
	"audioloader/internal/sources/ytdlp"
)

// Downloader holds the configured sources.
type Downloader struct {
	Sources []sources.Source
	// YT is nil when yt-dlp isn't available; YTErr then says why.
	YT    *ytdlp.Client
	YTErr error
	// SP is nil when spotDL isn't available; SPErr then says why.
	SP    *spotdl.Client
	SPErr error

	albumMu sync.Mutex
	albums  map[string]albumSongs
}

var (
	defaultOnce sync.Once
	defaultD    *Downloader
)

// Default is configured from the environment: AUDIOLOADER_WATCH for the
// watched folder, YTDLP, SPOTDL and FFMPEG for yt-dlp and spotDL. Sources are tried in the
// fixed order of sources.Order.
func Default() *Downloader {
	defaultOnce.Do(func() {
		d := &Downloader{}
		if os.Getenv("AUDIOLOADER_SOURCES") != "" {
			log.Println("downloads: AUDIOLOADER_SOURCES is ignored; sources are tried in a fixed order:", strings.Join(sources.Order, ", "))
		}
		if f := folder.FromEnv(); f != nil {
			d.Sources = append(d.Sources, f)
		}
		d.SP, d.SPErr = spotdl.Default()
		if d.SP != nil {
			d.Sources = append(d.Sources, spotdl.Source{C: d.SP})
			if v, err := d.SP.Version(context.Background()); err == nil {
				log.Println("spotdl", v)
			}
		} else {
			log.Println("downloads:", d.SPErr)
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

// SetDefault replaces the shared downloader; for tests.
func SetDefault(d *Downloader) {
	defaultOnce.Do(func() {})
	defaultD = d
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
// or else the best candidate of the first source that has one.
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
		// the first source in order that has anything wins; later (slower)
		// ones aren't asked
		ordered := slices.Clone(d.Sources)
		slices.SortStableFunc(ordered, func(a, b sources.Source) int {
			return sources.Position(a.Name()) - sources.Position(b.Name())
		})
		want := wantFor(dl)
		for _, s := range ordered {
			if s.Name() == spotdl.Name && want.SpotifyURL == "" && dl.ReleaseSpotify != "" {
				// the release has an album link: map its songs onto the
				// placeholders, so this track needs no search
				want.SpotifyURL = d.mapAlbum(ctx, dl)
			}
			found, err := s.Find(ctx, want)
			if err != nil {
				log.Printf("downloads: %s: %v", s.Name(), err)
				continue
			}
			if len(found) == 0 {
				continue
			}
			sources.Rank(found)
			src, cand = s, found[0]
			break
		}
		if src == nil {
			// YouTube is never used on a guess: with yt-dlp around, the user
			// gets to pick from its search results
			if d.YT != nil {
				return db.SetDownload(trackID, db.DownloadNeedsPick, "", 0, "")
			}
			return db.SetDownload(trackID, db.DownloadNotFound, "", 0, "")
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
	w := sources.Want{Title: dl.Track.Title, MBID: dl.Track.MBID, ISRC: dl.Track.ISRC, Duration: dl.Track.Duration, SpotifyURL: dl.SpotifyURL}
	for _, a := range dl.Track.Artists {
		w.Artists = append(w.Artists, a.Name)
	}
	if dl.Release != nil {
		w.Release = dl.Release.Title
	}
	return w
}

// Schedule looks again for "not found" and "needs a pick" downloads once a
// day, when a watched folder is set up (files may have been added to it
// since).
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
		missing, err := db.ListDownloads(db.All, db.DownloadNotFound, db.DownloadNeedsPick)
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

// albumSongs is a looked-up Spotify album, kept for a while: every
// placeholder of a release asks for the same one.
type albumSongs struct {
	songs []spotdl.Song
	at    time.Time
}

const albumTTL = 10 * time.Minute

// mapAlbum maps the songs of dl's release album onto the release's
// placeholder tracks, with the same rule as a YouTube playlist, and stores
// each mapped track's Spotify link. It returns dl's own link, or "" if
// nothing fits or the lookup failed (which is logged, not fatal: the
// search still gets its turn).
func (d *Downloader) mapAlbum(ctx context.Context, dl *db.Download) string {
	if d.SP == nil || dl.Release == nil {
		return ""
	}
	songs, err := d.albumSongs(ctx, dl.ReleaseSpotify)
	if err != nil {
		log.Println("downloads: spotdl album:", err)
		return ""
	}
	release, err := db.GetRelease(db.All, dl.Release.ID.String())
	if err != nil {
		log.Println("downloads:", err)
		return ""
	}
	var entries []Entry
	for _, s := range songs {
		if _, u, err := spotdl.ParseURL(s.URL); err == nil {
			entries = append(entries, Entry{ID: u, Title: s.Name, Duration: s.Length()})
		}
	}
	own := ""
	for _, m := range MapTracks(release, entries) {
		if m.ID == "" {
			continue
		}
		if err := db.SetSpotifyURL(m.Track.Track.ID.String(), m.ID); err != nil {
			log.Println("downloads:", err)
			continue
		}
		if m.Track.Track.ID == dl.Track.ID {
			own = m.ID
		}
	}
	return own
}

func (d *Downloader) albumSongs(ctx context.Context, albumURL string) ([]spotdl.Song, error) {
	kind, u, err := spotdl.ParseURL(albumURL)
	if err != nil || kind != "album" {
		return nil, spotdl.ErrBadURL
	}
	d.albumMu.Lock()
	defer d.albumMu.Unlock()
	if c, ok := d.albums[u]; ok && time.Since(c.at) < albumTTL {
		return c.songs, nil
	}
	songs, err := d.SP.Save(ctx, u)
	if err != nil {
		return nil, err
	}
	if d.albums == nil {
		d.albums = map[string]albumSongs{}
	}
	d.albums[u] = albumSongs{songs, time.Now()}
	return songs, nil
}
