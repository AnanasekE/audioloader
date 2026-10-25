// Package tracking follows artists on MusicBrainz: it checks their
// discographies for releases the library doesn't have and puts those in
// the inbox. Confirming one creates it with placeholder tracks and queues
// their downloads.
package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"slices"
	"time"

	"audioloader/internal/db"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
	"audioloader/internal/tagging"
)

// JobCheck looks for new releases of one followed artist.
const JobCheck = "check_artist"

// CheckEvery is how often each followed artist is checked.
var CheckEvery = 24 * time.Hour

// Register sets up the job handler.
func Register() {
	jobs.Register(JobCheck, func(ctx context.Context, payload json.RawMessage) error {
		var p struct{ ArtistID string }
		if err := json.Unmarshal(payload, &p); err != nil {
			return jobs.Permanent(err)
		}
		return Check(ctx, mb.Default(), p.ArtistID)
	})
}

// EnqueueCheck queues a check of an artist, unless one is already queued.
func EnqueueCheck(artistID string, at time.Time) error {
	return jobs.EnqueueAt(JobCheck, artistID, map[string]string{"ArtistID": artistID}, at)
}

// Schedule queues checks for followed artists that are due, now and then
// every hour, until ctx is cancelled. Checks are spread out so a long list
// of artists doesn't block other jobs for long at a time.
func Schedule(ctx context.Context) {
	for {
		due, err := db.FollowsDue(CheckEvery)
		if err != nil {
			log.Println("tracking:", err)
		}
		for i, id := range due {
			if err := EnqueueCheck(id, time.Now().Add(time.Duration(i)*2*time.Minute)); err != nil {
				log.Println("tracking:", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

// Check browses a followed artist's release groups and puts the ones that
// pass the follow's filters and aren't in the library into the inbox.
func Check(ctx context.Context, c *mb.Client, artistID string) error {
	f, err := db.GetFollow(artistID)
	if errors.Is(err, db.ErrNotFound) {
		return nil // unfollowed since
	}
	if err != nil {
		return err
	}
	if f.Artist.MBID == "" {
		return jobs.Permanent(errors.New("followed artist has no MusicBrainz ID"))
	}

	var groups []mb.ReleaseGroup
	for offset := 0; ; {
		page, count, err := c.BrowseReleaseGroups(ctx, f.Artist.MBID, offset)
		if err != nil {
			return err
		}
		groups = append(groups, page...)
		offset += len(page)
		if len(page) == 0 || offset >= count {
			break
		}
	}

	// secret tracks aren't matched: the inbox would mark them as owned
	library, err := db.ListTracks(db.Public)
	if err != nil {
		return err
	}
	for _, rg := range groups {
		typ := tagging.ReleaseType(rg)
		if !Wanted(f.Filters, typ, rg.SecondaryTypes) {
			continue
		}
		if known, err := db.InboxKnows(rg.ID); err != nil || known {
			if err != nil {
				return err
			}
			continue
		}
		if id, err := db.LibraryReleaseFor(artistID, rg.ID, rg.Title); err != nil || id != "" {
			if err != nil {
				return err
			}
			continue
		}

		releases, _, err := c.BrowseReleases(ctx, rg.ID, 0)
		if err != nil {
			return err
		}
		rel := PickRelease(releases)
		if rel == nil {
			continue // only bootlegs or promos
		}
		ir := db.InboxRelease{
			ReleaseGroupMBID: rg.ID,
			ArtistID:         artistID,
			ReleaseMBID:      rel.ID,
			Title:            rg.Title,
			Type:             typ,
			Date:             firstNonEmpty(rg.FirstReleaseDate, rel.Date),
			Data: db.InboxData{
				CoverURL: c.CoverArtURL + "/release-group/" + rg.ID + "/front-250",
				Artists:  credits(rel.ArtistCredit),
			},
		}
		for _, m := range rel.Media {
			for _, t := range m.Tracks {
				it := db.InboxTrack{
					Disc:          max(m.Position, 1),
					Position:      t.Position,
					Title:         t.Title,
					RecordingMBID: t.Recording.ID,
					LengthMs:      firstPositive(t.Length, t.Recording.Length),
					Artists:       credits(t.Recording.ArtistCredit),
				}
				if len(t.Recording.ISRCs) > 0 {
					it.ISRC = t.Recording.ISRCs[0]
				}
				it.OwnedTrackID = db.OwnedTrack(library, it)
				ir.Data.Tracks = append(ir.Data.Tracks, it)
			}
		}
		if err := db.AddInboxRelease(ir); err != nil {
			return err
		}
	}
	return db.MarkFollowChecked(artistID)
}

// Wanted applies a follow's filters to a release group of type typ ("" for
// types we don't model, which are never wanted).
func Wanted(f db.FollowFilters, typ string, secondary []string) bool {
	if typ == "" || !slices.Contains(f.Types, typ) {
		return false
	}
	for _, s := range secondary {
		// a compilation the user asked for isn't skipped for being one
		if s == "Compilation" && typ == "compilation" {
			continue
		}
		if slices.Contains(f.SkipSecondary, s) {
			return false
		}
	}
	return true
}

// PickRelease chooses the release that stands for its group: official
// first, then digital, then the earliest. nil if none is official.
func PickRelease(releases []mb.Release) *mb.Release {
	var best *mb.Release
	rank := func(r *mb.Release) int {
		n := 0
		for _, m := range r.Media {
			if m.Format == "Digital Media" {
				n = 1
			}
		}
		return n
	}
	for i := range releases {
		r := &releases[i]
		if r.Status != "Official" || len(r.Media) == 0 {
			continue
		}
		if best == nil || rank(r) > rank(best) || rank(r) == rank(best) && earlier(r.Date, best.Date) {
			best = r
		}
	}
	return best
}

func earlier(a, b string) bool {
	if a == "" {
		return false
	}
	return b == "" || a < b
}

// Confirm creates an inbox release in the library and queues the downloads
// of its missing tracks and its cover.
func Confirm(releaseGroupMBID string) (string, error) {
	releaseID, placeholders, err := db.ConfirmInboxRelease(releaseGroupMBID)
	if err != nil {
		return "", err
	}
	for _, id := range placeholders {
		if err := downloads.Enqueue(id); err != nil {
			return releaseID, err
		}
	}
	return releaseID, tagging.EnqueueCover(releaseID)
}

func credits(cs mb.Credits) []db.MatchArtist {
	out := make([]db.MatchArtist, 0, len(cs))
	for _, c := range cs {
		name := c.Artist.Name
		if name == "" {
			name = c.Name
		}
		out = append(out, db.MatchArtist{ID: c.Artist.ID, Name: name})
	}
	return out
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func firstPositive(ns ...int) int {
	for _, n := range ns {
		if n > 0 {
			return n
		}
	}
	return 0
}
