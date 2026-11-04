// Package addsong adds songs to the library as placeholder tracks on their
// MusicBrainz releases, with a queued download: the part of adding a song
// that the Add, Find and Apple Music pages and the Apple Music sync share.
package addsong

import (
	"context"
	"errors"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
	"audioloader/internal/tagging"
)

// ReleaseSong is one track of a MusicBrainz release, ready to add to the
// library on that release.
type ReleaseSong struct {
	Song           db.NewSong
	Track          db.InboxTrack // what the library's ownership rule compares
	Artists        string
	Disc, Position int
}

// ReleaseSongs lists rel's tracks in disc and position order.
func ReleaseSongs(rel *mb.Release) []ReleaseSong {
	var out []ReleaseSong
	for _, m := range rel.Media {
		for _, t := range m.Tracks {
			it := SongTrack(t.Recording)
			it.Title, it.LengthMs = t.Title, firstPositive(t.Length, t.Recording.Length)
			out = append(out, ReleaseSong{
				Track:    it,
				Artists:  t.Recording.ArtistCredit.String(),
				Disc:     max(m.Position, 1),
				Position: t.Position,
				Song: db.NewSong{
					RecordingMBID: t.Recording.ID,
					Title:         it.Title,
					ISRC:          it.ISRC,
					LengthMs:      it.LengthMs,
					SpotifyURL:    mb.SpotifyURL(t.Recording.Relations, "track"),
					Artists:       it.Artists,
					CoverWorkID:   coverWorkID(t.Recording),
					Release: &db.NewSongRelease{
						MBID:             rel.ID,
						ReleaseGroupMBID: rel.ReleaseGroup.ID,
						Title:            rel.Title,
						Type:             tagging.ReleaseType(rel.ReleaseGroup),
						Date:             rel.Date,
						SpotifyURL:       mb.SpotifyURL(rel.Relations, "album"),
						Artists:          tagging.MatchArtists(rel.ArtistCredit),
						Disc:             max(m.Position, 1),
						Position:         t.Position,
					},
				},
			})
		}
	}
	return out
}

func coverWorkID(rec mb.Recording) string {
	if w := rec.CoverWork(); w != nil {
		return w.ID
	}
	return ""
}

// SongTrack is the recording as an inbox tracklist entry, which is what the
// library's ownership rule compares.
func SongTrack(rec mb.Recording) db.InboxTrack {
	it := db.InboxTrack{
		Title:         rec.Title,
		RecordingMBID: rec.ID,
		LengthMs:      rec.Length,
		Artists:       tagging.MatchArtists(rec.ArtistCredit),
	}
	if len(rec.ISRCs) > 0 {
		it.ISRC = rec.ISRCs[0]
	}
	return it
}

// Owned finds the library's track for t, or its queued placeholder. The
// release is the one the track is on; "" for both when the library has none.
func Owned(library []*audio.Track, t db.InboxTrack) (trackID, releaseID string, hasFile bool, err error) {
	if id := db.OwnedTrack(library, t); id != "" {
		for _, lt := range library {
			if lt.ID.String() == id && len(lt.Releases) > 0 {
				releaseID = lt.Releases[0].ID.String()
			}
		}
		return id, releaseID, true, nil
	}
	if t.RecordingMBID == "" {
		return "", "", false, nil
	}
	trackID, releaseID, err = db.QueuedSong(t.RecordingMBID)
	return trackID, releaseID, false, err
}

var ErrAlreadyOwned = errors.New("the library already has this song")

// Queue adds song (it is the same song as the library's ownership rule
// sees it) as a placeholder and queues its download, unless the library has
// it or has it queued. It returns the track's ID, which with ErrAlreadyOwned
// is the library's.
func Queue(library []*audio.Track, song db.NewSong, it db.InboxTrack) (string, error) {
	// another tab may have added it since the search
	if id, _, _, err := Owned(library, it); err != nil {
		return "", err
	} else if id != "" {
		return id, ErrAlreadyOwned
	}
	trackID, newReleaseID, newRelease, err := db.AddSong(song)
	if err != nil {
		return "", err
	}
	if err := downloads.Enqueue(trackID); err != nil {
		return "", err
	}
	if song.CoverWorkID != "" {
		payload := map[string]string{"TrackID": trackID, "WorkID": song.CoverWorkID, "RecordingID": song.RecordingMBID}
		if err := jobs.EnqueueAt(tagging.JobCoverOfApply, trackID, payload, time.Time{}); err != nil {
			return "", err
		}
	}
	if newRelease {
		return trackID, tagging.EnqueueCover(newReleaseID)
	}
	return trackID, nil
}

func firstPositive(a, b int) int {
	if a > 0 {
		return a
	}
	return b
}

// Single reads what db.AddSong needs for a recording on its own, without a
// release. mb.ErrNotFound means MusicBrainz doesn't have it.
func Single(ctx context.Context, c *mb.Client, recordingID string) (db.NewSong, db.InboxTrack, error) {
	rec, err := c.GetRecording(ctx, recordingID)
	if err != nil {
		return db.NewSong{}, db.InboxTrack{}, err
	}
	it := SongTrack(*rec)
	return db.NewSong{
		RecordingMBID: rec.ID,
		Title:         rec.Title,
		ISRC:          it.ISRC,
		LengthMs:      rec.Length,
		SpotifyURL:    mb.SpotifyURL(rec.Relations, "track"),
		Artists:       it.Artists,
		CoverWorkID:   coverWorkID(*rec),
	}, it, nil
}
