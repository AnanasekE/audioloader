package downloads

import (
	"context"
	"errors"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/jobs"
	"audioloader/internal/match"
	"audioloader/internal/sources/ytdlp"
	"audioloader/internal/tagging"
)

// YouTubeImport is one video of an "Import from YouTube" upload.
type YouTubeImport struct {
	VideoID   string
	Title     string
	ArtistIDs []string
	// ReleaseID places the track on a release at Disc and Position; without
	// it the track becomes its own single, released on Date.
	ReleaseID string
	Disc      int
	Position  int
	Date      string
}

// EnqueueImport queues a YouTube import.
func EnqueueImport(imp YouTubeImport) error {
	return jobs.EnqueueAt(JobYouTubeImport, imp.VideoID, imp, time.Time{})
}

// Import downloads the video and adds it like an upload.
func (d *Downloader) Import(ctx context.Context, imp YouTubeImport) error {
	if d.YT == nil {
		return jobs.Permanent(d.YTErr)
	}
	var link *db.ReleaseLink
	if imp.ReleaseID != "" {
		link = &db.ReleaseLink{ReleaseID: imp.ReleaseID, Disc: max(imp.Disc, 1), Position: imp.Position}
	}
	sourceURL := ytdlp.VideoURL(imp.VideoID)
	existing, err := db.TrackBySourceURL(sourceURL)
	if err != nil {
		return err
	}
	if existing != "" {
		// downloaded before: place that track instead of downloading again
		if link == nil {
			return nil
		}
		err := db.PlaceTrack(existing, *link)
		if errors.Is(err, db.ErrAlreadyOnRelease) {
			return nil
		}
		return permanentIfBad(err)
	}

	dir, err := os.MkdirTemp("", "audioloader-ytdlp-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path, err := d.YT.Download(ctx, imp.VideoID, dir, nil)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		return jobs.Permanent(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dur, err := audio.ProbeDuration(f)
	if err != nil {
		return jobs.Permanent(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	res, err := db.AddTrack(imp.ArtistIDs, &audio.Track{Title: imp.Title, Duration: dur.Round(time.Second)}, f, ".opus", link, imp.Date)
	if err != nil {
		return permanentIfBad(err)
	}
	if !res.Linked {
		if err := db.SetSourceURL(res.TrackID, sourceURL); err != nil {
			return err
		}
		if err := tagging.Enqueue(res.TrackID); err != nil {
			return err
		}
	}
	return nil
}

// permanentIfBad marks errors that a retry can't fix as permanent.
func permanentIfBad(err error) error {
	for _, e := range []error{db.ErrExists, db.ErrSlotTaken, db.ErrBadReference, db.ErrAlreadyOnRelease} {
		if errors.Is(err, e) {
			return jobs.Permanent(err)
		}
	}
	return err
}

var (
	// "(Official Video)", "[Official Audio]", "(Lyrics)", "(HD)", ...
	noise = regexp.MustCompile(`(?i)\s*[\(\[][^\)\]]*\b(official|video|audio|lyrics?|visuali[sz]er|hd|hq|4k|mv)\b[^\)\]]*[\)\]]`)
	dash  = regexp.MustCompile(`\s+[-–—|]\s+`)
	// any bracketed part, e.g. "(Remastered 2019)"
	parens = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)
)

// CleanTitle turns a video title into a track title: it drops "(Official
// Video)" and the like, and an "Artist - " prefix naming one of artists.
func CleanTitle(videoTitle string, artists []string) string {
	t := strings.TrimSpace(noise.ReplaceAllString(videoTitle, ""))
	if loc := dash.FindStringIndex(t); loc != nil {
		prefix := t[:loc[0]]
		for _, a := range artists {
			if match.Same(prefix, a) || strings.Contains(match.Normalize(prefix), match.Normalize(a)) && a != "" {
				return strings.TrimSpace(t[loc[1]:])
			}
		}
	}
	return t
}

// Mapping pairs a release's placeholder track with a playlist video.
type Mapping struct {
	Track   *audio.ReleaseTrack
	VideoID string // "" if no video fits
	Title   string // the video's title
	Score   float64
}

// MapPlaylist matches playlist videos to the release's tracks without a
// file, by title (cleaned of video noise) and a length within 5 seconds.
// Each video is used once, best pairs first.
func MapPlaylist(r *audio.Release, videos []ytdlp.Entry) []Mapping {
	var artists []string
	for _, a := range r.Artists {
		artists = append(artists, a.Name)
	}
	type pair struct {
		ti, vi int
		score  float64
	}
	var missing []*audio.ReleaseTrack
	for _, rt := range r.Tracks {
		if !rt.Track.HasFile {
			missing = append(missing, rt)
		}
	}
	var pairs []pair
	for ti, rt := range missing {
		for vi, v := range videos {
			if rt.Track.Duration > 0 && v.Duration > 0 {
				diff := rt.Track.Duration - time.Duration(v.Duration*float64(time.Second))
				if diff < -5*time.Second || diff > 5*time.Second {
					continue
				}
			}
			vt := CleanTitle(v.Title, artists)
			// "Song (Remastered)" is still the song
			s := max(match.Similarity(vt, rt.Track.Title), match.Similarity(parens.ReplaceAllString(vt, ""), parens.ReplaceAllString(rt.Track.Title, "")))
			if s >= 0.6 {
				pairs = append(pairs, pair{ti, vi, s})
			}
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].score > pairs[j].score })

	out := make([]Mapping, len(missing))
	for i, rt := range missing {
		out[i].Track = rt
	}
	usedV := map[int]bool{}
	for _, p := range pairs {
		if out[p.ti].VideoID != "" || usedV[p.vi] {
			continue
		}
		usedV[p.vi] = true
		out[p.ti].VideoID, out[p.ti].Title, out[p.ti].Score = videos[p.vi].ID, videos[p.vi].Title, p.score
	}
	return out
}
