package appleimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"audioloader/internal/applemusic"
	"audioloader/internal/db"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
)

// JobSync looks for new songs in one followed Apple Music playlist. It runs
// in the lane for everything but downloads.
const JobSync = "apple_sync"

// SyncEvery is how often each followed playlist is checked.
var SyncEvery = time.Hour

// Messages a follow shows in place of an error.
const (
	msgNotPublic  = "Not public any more"
	msgUnreadable = "Apple Music changed its page; this link can't be read."
)

// EnqueueSync queues a check of a follow, unless one is queued or running.
func EnqueueSync(followID int64) error {
	return jobs.EnqueueAt(JobSync, SyncKey(followID), map[string]int64{"FollowID": followID}, time.Time{})
}

// SyncKey is the dedupe key of a follow's job, for jobs.Latest.
func SyncKey(followID int64) string { return strconv.FormatInt(followID, 10) }

// Schedule queues checks for followed playlists that are due, now and then
// every minute, until ctx is cancelled.
func Schedule(ctx context.Context) {
	for {
		due, err := db.AppleFollowsDue(SyncEvery)
		if err != nil {
			log.Println("appleimport:", err)
		}
		for _, id := range due {
			if err := EnqueueSync(id); err != nil {
				log.Println("appleimport:", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

func registerSync() {
	jobs.Register(JobSync, func(ctx context.Context, payload json.RawMessage) error {
		var p struct{ FollowID int64 }
		if err := json.Unmarshal(payload, &p); err != nil {
			return jobs.Permanent(err)
		}
		return Sync(ctx, applemusic.Default(), mb.Default(), p.FollowID)
	})
}

// Sync reads the followed playlist and adds the songs it hasn't seen to the
// follow's import: a confident match is added to the library (and to the
// import's audioloader playlist, if it made one), the rest wait on the
// import page to be picked. Songs removed from the playlist are ignored. A
// page that can't be read is recorded on the follow, not retried.
func Sync(ctx context.Context, am *applemusic.Client, mbc *mb.Client, followID int64) error {
	f, err := db.GetAppleFollow(followID)
	if errors.Is(err, db.ErrNotFound) {
		return nil // unfollowed since
	}
	if err != nil {
		return err
	}
	imp, err := db.GetAppleImport(f.ImportID)
	if err != nil {
		return err
	}
	if imp.Status == db.ImportQueued || imp.Status == db.ImportRunning {
		return nil // its first read isn't done
	}
	pl, err := am.GetPlaylist(ctx, f.URL)
	switch {
	case errors.Is(err, applemusic.ErrNotFound):
		return db.MarkAppleFollowChecked(f.ID, msgNotPublic)
	case errors.Is(err, applemusic.ErrUnreadable):
		return db.MarkAppleFollowChecked(f.ID, msgUnreadable)
	case err != nil:
		return err
	}
	if err := addNewSongs(f, pl.Songs); err != nil {
		return err
	}
	if err := matchAndAdd(ctx, mbc, f.ImportID); err != nil {
		return err
	}
	note := ""
	if len(pl.Songs) < pl.TrackCount {
		note = fmt.Sprintf("Apple Music shows the first %d of %d songs", len(pl.Songs), pl.TrackCount)
	}
	return db.MarkAppleFollowChecked(f.ID, note)
}

// addNewSongs appends the songs the follow hasn't seen to its import. Songs
// of an import made before song IDs were stored are told apart by title and
// artist, and get their ID marked seen.
func addNewSongs(f *db.AppleFollow, songs []applemusic.Song) error {
	seen, err := db.AppleFollowSeen(f.ID)
	if err != nil {
		return err
	}
	tracks, err := db.AppleImportTracks(f.ImportID)
	if err != nil {
		return err
	}
	old := map[string]bool{}
	for _, t := range tracks {
		if t.AppleID == "" {
			old[strings.ToLower(t.Title+"\x00"+t.Artist)] = true
		}
	}
	var fresh []db.AppleImportTrack
	var known []string
	for _, s := range songs {
		switch {
		case s.ID == "" || seen[s.ID]:
		case old[strings.ToLower(s.Title+"\x00"+s.Artist)]:
			known = append(known, s.ID)
		default:
			seen[s.ID] = true // a song listed twice is new once
			fresh = append(fresh, db.AppleImportTrack{AppleID: s.ID, Title: s.Title, Artist: s.Artist, Album: s.Album, DurationMs: int(s.Duration / time.Millisecond)})
		}
	}
	if err := db.MarkAppleFollowSeen(f.ID, known); err != nil {
		return err
	}
	if len(fresh) == 0 {
		return nil
	}
	_, err = db.AddAppleFollowSongs(f.ID, fresh)
	return err
}

// matchAndAdd matches the import's pending songs, adds the confident ones
// to the library, and brings the import's playlist up to date. A retried job
// finds the songs it left pending.
func matchAndAdd(ctx context.Context, mbc *mb.Client, importID int64) error {
	tracks, err := db.AppleImportTracks(importID)
	if err != nil {
		return err
	}
	library, err := db.ListTracks(db.Normal)
	if err != nil {
		return err
	}
	var pending []*db.AppleImportTrack
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
		pending = append(pending, t)
	}
	if len(pending) == 0 {
		return nil
	}
	// matchTrack saved the results; read them back
	if tracks, err = db.AppleImportTracks(importID); err != nil {
		return err
	}
	pos := map[int]bool{}
	for _, t := range pending {
		pos[t.Position] = true
	}
	releases := map[string]*mb.Release{}
	for _, t := range tracks {
		if !pos[t.Position] || t.State != db.AppleSongMatched {
			continue
		}
		err := AddTrack(ctx, mbc, releases, library, t)
		if errors.Is(err, mb.ErrNotFound) {
			// the recording went away; leave the song to be picked
			if err := db.SetAppleTrackState(importID, t.Position, db.AppleSongUnsure); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
	}
	imp, err := db.GetAppleImport(importID)
	if err != nil {
		return err
	}
	if imp.PlaylistID == "" {
		return nil
	}
	return SyncPlaylist(imp, tracks)
}
