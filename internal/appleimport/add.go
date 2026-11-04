package appleimport

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"audioloader/internal/addsong"
	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/mb"
)

// ErrMusicBrainz wraps a failure to read MusicBrainz, which is worth a retry.
var ErrMusicBrainz = errors.New("couldn't read MusicBrainz")

// AddTrack adds a matched or unsure song of an import as a placeholder on
// its MusicBrainz release with a queued download, and saves its new state:
// added, or owned when the library has it already. releases caches the
// releases read so far, so several songs of one read it once. A recording
// that isn't on its release any more is added as a single; one MusicBrainz
// doesn't have gives mb.ErrNotFound.
func AddTrack(ctx context.Context, c *mb.Client, releases map[string]*mb.Release, library []*audio.Track, t *db.AppleImportTrack) error {
	song, it, err := trackSong(ctx, c, releases, t)
	if err != nil {
		return err
	}
	trackID, err := addsong.Queue(library, song, it)
	switch {
	case errors.Is(err, addsong.ErrAlreadyOwned):
		t.State, t.TrackID = db.AppleSongOwned, trackID
	case err != nil:
		return err
	default:
		t.State, t.TrackID = db.AppleSongAdded, trackID
	}
	return db.SetAppleTrackMatch(*t)
}

// trackSong reads what db.AddSong needs for t, on the MusicBrainz release it
// was matched to.
func trackSong(ctx context.Context, c *mb.Client, releases map[string]*mb.Release, t *db.AppleImportTrack) (db.NewSong, db.InboxTrack, error) {
	if t.ReleaseID != "" {
		rel, seen := releases[t.ReleaseID]
		if !seen {
			var err error
			rel, err = c.GetRelease(ctx, t.ReleaseID)
			if errors.Is(err, mb.ErrNotFound) {
				rel = nil
			} else if err != nil {
				return db.NewSong{}, db.InboxTrack{}, fmt.Errorf("%w: %w", ErrMusicBrainz, err)
			}
			releases[t.ReleaseID] = rel
		}
		if rel != nil {
			for _, rs := range addsong.ReleaseSongs(rel) {
				if rs.Song.RecordingMBID == t.RecordingID {
					return rs.Song, rs.Track, nil
				}
			}
		}
	}
	song, it, err := addsong.Single(ctx, c, t.RecordingID)
	if err != nil && !errors.Is(err, mb.ErrNotFound) {
		err = fmt.Errorf("%w: %w", ErrMusicBrainz, err)
	}
	return song, it, err
}

// SyncPlaylist makes the import's audioloader playlist if it has none, and
// fills it with the songs the library has, in Apple's order. Tracks that
// are in it already stay, and run again it adds only what is missing.
func SyncPlaylist(imp *db.AppleImport, tracks []*db.AppleImportTrack) error {
	playlistID := imp.PlaylistID
	var existing []string
	if playlistID == "" {
		p, err := db.CreatePlaylist(db.Normal, imp.Name)
		if err != nil {
			return err
		}
		playlistID = p.ID.String()
		if err := db.SetAppleImportPlaylist(imp.ID, playlistID); err != nil {
			return err
		}
	} else {
		p, err := db.GetPlaylist(db.Normal, playlistID)
		if err != nil {
			return err
		}
		for _, t := range p.Tracks {
			existing = append(existing, t.ID.String())
		}
	}
	var ids []string
	for _, t := range tracks {
		if (t.State == db.AppleSongOwned || t.State == db.AppleSongAdded) && t.TrackID != "" && !slices.Contains(ids, t.TrackID) {
			ids = append(ids, t.TrackID)
		}
	}
	// what a person put there themselves keeps its place after Apple's songs
	for _, id := range existing {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return db.SetPlaylistTracks(db.Normal, playlistID, ids)
}
