package tagging

import (
	"context"
	"errors"

	"audioloader/internal/db"
	"audioloader/internal/mb"
)

// ErrNoMove is returned when the track has no pending suggestion to move it.
var ErrNoMove = errors.New("this track has nothing to move")

// ErrNotOnRelease is returned when MusicBrainz no longer lists the track's
// recording on the release its match named.
var ErrNotOnRelease = errors.New("MusicBrainz doesn't list this recording on that release")

// Move puts a YouTube import on the release its applied match is on. The
// release is read from MusicBrainz for the track's disc and position, and
// created in the library if it isn't there. Tags are not written into the
// file, so there is nothing to rewrite there: the library's records change.
func Move(ctx context.Context, mbc *mb.Client, trackID string) error {
	list, err := db.MoveSuggestions(db.Normal)
	if err != nil {
		return err
	}
	var s *db.MoveSuggestion
	for i := range list {
		if list[i].TrackID == trackID {
			s = &list[i]
		}
	}
	if s == nil {
		return ErrNoMove
	}

	rel, err := mbc.GetRelease(ctx, s.TargetID)
	if err != nil {
		return err
	}
	target := db.NewSongRelease{
		MBID:             rel.ID,
		ReleaseGroupMBID: rel.ReleaseGroup.ID,
		Title:            rel.Title,
		Type:             ReleaseType(rel.ReleaseGroup),
		Date:             rel.Date,
		SpotifyURL:       mb.SpotifyURL(rel.Relations, "album"),
		Artists:          matchArtists(rel.ArtistCredit),
	}
	found := false
	for _, m := range rel.Media {
		for _, t := range m.Tracks {
			if t.Recording.ID == s.RecordingID {
				target.Disc, target.Position, found = max(m.Position, 1), t.Position, true
			}
		}
	}
	if !found {
		return ErrNotOnRelease
	}

	moved, err := db.MoveTrackToRelease(trackID, s.FromID, target)
	if err != nil {
		return err
	}
	if moved.NewRelease {
		return EnqueueCover(moved.ReleaseID)
	}
	return nil
}
