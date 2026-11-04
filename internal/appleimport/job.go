package appleimport

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"audioloader/internal/applemusic"
	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
)

// JobImport reads an Apple Music playlist and matches each song on
// MusicBrainz. It runs in the lane for everything but downloads.
const JobImport = "apple_import"

// Register sets up the job handler.
func Register() {
	jobs.Register(JobImport, func(ctx context.Context, payload json.RawMessage) error {
		var p struct{ ImportID int64 }
		if err := json.Unmarshal(payload, &p); err != nil {
			return jobs.Permanent(err)
		}
		return Run(ctx, applemusic.Default(), mb.Default(), p.ImportID)
	})
}

// Start makes an import of the playlist at url and queues its job.
func Start(url string) (int64, error) {
	id, err := db.CreateAppleImport(url)
	if err != nil {
		return 0, err
	}
	return id, Enqueue(id)
}

// Enqueue queues the job of an import, unless it is queued or running.
func Enqueue(importID int64) error {
	return jobs.EnqueueAt(JobImport, Key(importID), map[string]int64{"ImportID": importID}, time.Time{})
}

// Key is the dedupe key of an import's job, for jobs.Latest.
func Key(importID int64) string { return strconv.FormatInt(importID, 10) }

// Run reads the import's page, if that wasn't done yet, and matches the
// songs that are still pending, saving each result as it goes: a retried
// job picks up where the last one stopped.
func Run(ctx context.Context, am *applemusic.Client, mbc *mb.Client, importID int64) error {
	imp, err := db.GetAppleImport(importID)
	if errors.Is(err, db.ErrNotFound) {
		return jobs.Permanent(err)
	}
	if err != nil {
		return err
	}
	if imp.Status == db.ImportDone || imp.Status == db.ImportFailed {
		return nil
	}
	if err := db.SetAppleImportStatus(importID, db.ImportRunning, ""); err != nil {
		return err
	}
	if err := readPlaylist(ctx, am, imp); err != nil {
		return err
	}
	tracks, err := db.AppleImportTracks(importID)
	if err != nil {
		return err
	}
	library, err := db.ListTracks(db.Normal)
	if err != nil {
		return err
	}
	for _, t := range tracks {
		if t.State != db.AppleSongPending {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := matchTrack(ctx, mbc, library, t); err != nil {
			return err
		}
	}
	return db.SetAppleImportStatus(importID, db.ImportDone, "")
}

// readPlaylist stores the playlist's songs, unless they are stored. A page
// that can't be read fails the import for good; any other error is retried.
func readPlaylist(ctx context.Context, am *applemusic.Client, imp *db.AppleImport) error {
	counts, err := db.AppleImportCounts(imp.ID)
	if err != nil {
		return err
	}
	if len(counts) > 0 {
		return nil
	}
	pl, err := am.GetPlaylist(ctx, imp.URL)
	if errors.Is(err, applemusic.ErrNotFound) || errors.Is(err, applemusic.ErrUnreadable) {
		msg := err.Error()
		if errors.Is(err, applemusic.ErrNotFound) {
			msg = "That playlist isn't on Apple Music, or it isn't public."
		}
		if err := db.SetAppleImportStatus(imp.ID, db.ImportFailed, msg); err != nil {
			return err
		}
		return jobs.Permanent(err)
	}
	if err != nil {
		return err
	}
	tracks := make([]db.AppleImportTrack, len(pl.Songs))
	for i, s := range pl.Songs {
		tracks[i] = db.AppleImportTrack{AppleID: s.ID, Title: s.Title, Artist: s.Artist, Album: s.Album, DurationMs: int(s.Duration / time.Millisecond)}
	}
	return db.StartAppleImport(imp.ID, pl.Name, pl.TrackCount, tracks)
}

// matchTrack finds the song on MusicBrainz, or in the library, and saves
// the result.
func matchTrack(ctx context.Context, mbc *mb.Client, library []*audio.Track, t *db.AppleImportTrack) error {
	song := applemusic.Song{
		Title:    t.Title,
		Artist:   t.Artist,
		Album:    t.Album,
		Duration: time.Duration(t.DurationMs) * time.Millisecond,
	}
	out := *t
	// owned is the library's track for the song, "" if it has none
	owned := func(recordingID string) string {
		it := db.InboxTrack{Title: song.Title, RecordingMBID: recordingID, LengthMs: t.DurationMs}
		for _, n := range artistNames(song.Artist) {
			it.Artists = append(it.Artists, db.MatchArtist{Name: n})
		}
		if id := db.OwnedTrack(library, it); id != "" {
			return id
		}
		if recordingID == "" {
			return ""
		}
		id, _, _ := db.QueuedSong(recordingID)
		return id
	}
	if id := owned(""); id != "" {
		out.State, out.TrackID = db.AppleSongOwned, id
		return db.SetAppleTrackMatch(out)
	}
	cands, err := Find(ctx, mbc, song)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		out.State = db.AppleSongNotFound
		return db.SetAppleTrackMatch(out)
	}
	top := cands[0]
	out.RecordingID = top.Recording.ID
	out.MatchTitle = top.Recording.Title
	out.MatchArtist = top.Recording.ArtistCredit.String()
	out.Score = top.Score
	if top.Release != nil {
		out.ReleaseID, out.ReleaseTitle = top.Release.ID, top.Release.Title
	}
	out.State = db.AppleSongUnsure
	if top.Confident {
		out.State = db.AppleSongMatched
		if id := owned(top.Recording.ID); id != "" {
			out.State, out.TrackID = db.AppleSongOwned, id
		}
	}
	return db.SetAppleTrackMatch(out)
}
