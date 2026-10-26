package db

import (
	"context"
	"database/sql"
	"uuid"

	"audioloader/internal/audio"
)

// Mode says which side of the library a viewer reads: the normal tracks, or
// the secret ones. The two are kept apart: normal mode shows no secret track
// and secret mode shows no normal one.
type Mode int

const (
	ModeNormal Mode = iota
	ModeSecret
	// ModeAll sees both: background jobs only, never a request.
	ModeAll
)

// Viewer is who is reading. Every read of tracks takes one, so the compiler
// finds each place that has to filter by mode.
type Viewer struct {
	Mode Mode
}

var (
	Normal = Viewer{ModeNormal}
	Secret = Viewer{ModeSecret}
	All    = Viewer{ModeAll}
)

// IsSecret reports whether v reads the secret side only.
func (v Viewer) IsSecret() bool { return v.Mode == ModeSecret }

// SeesAll reports whether v isn't limited to one side (background jobs).
func (v Viewer) SeesAll() bool { return v.Mode == ModeAll }

// modeMask is the bit of the search index's modes column for v's side.
func (v Viewer) modeMask() int {
	switch v.Mode {
	case ModeSecret:
		return 2
	case ModeAll:
		return 3
	}
	return 1
}

// The conditions below are SQL on a table aliased alias. A release, artist
// or show belongs to a mode when it has a track of that mode. One with no
// tracks at all belongs to normal mode, except an artist or show that was
// created in secret mode (its secret column), which waits there for its
// first track.

func (v Viewer) trackCond(alias string) string {
	switch v.Mode {
	case ModeSecret:
		return alias + ".secret = 1"
	case ModeAll:
		return "1"
	}
	return alias + ".secret = 0"
}

func (v Viewer) releaseCond(alias string) string {
	switch v.Mode {
	case ModeSecret:
		return `EXISTS (SELECT 1 FROM release_tracks rs JOIN tracks ts ON ts.uuid = rs.track_id WHERE rs.release_id = ` + alias + `.uuid AND ts.secret = 1)`
	case ModeAll:
		return "1"
	}
	return `(NOT EXISTS (SELECT 1 FROM release_tracks rs WHERE rs.release_id = ` + alias + `.uuid)
		OR EXISTS (SELECT 1 FROM release_tracks rs JOIN tracks ts ON ts.uuid = rs.track_id WHERE rs.release_id = ` + alias + `.uuid AND ts.secret = 0))`
}

func (v Viewer) artistCond(alias string) string {
	own := func(secret string) string {
		return `EXISTS (SELECT 1 FROM track_artist_map ms JOIN tracks ts ON ts.uuid = ms.track_id WHERE ms.artist_id = ` + alias + `.uuid AND ts.secret = ` + secret + `)`
	}
	noTracks := `NOT EXISTS (SELECT 1 FROM track_artist_map ms WHERE ms.artist_id = ` + alias + `.uuid)`
	switch v.Mode {
	case ModeSecret:
		return `(` + own("1") + ` OR (` + noTracks + ` AND ` + alias + `.secret = 1))`
	case ModeAll:
		return "1"
	}
	return `(` + own("0") + ` OR (` + noTracks + ` AND ` + alias + `.secret = 0)
		OR EXISTS (SELECT 1 FROM release_artist_map ma JOIN releases ra ON ra.uuid = ma.release_id WHERE ma.artist_id = ` + alias + `.uuid AND ` + v.releaseCond("ra") + `))`
}

func (v Viewer) showCond(alias string) string {
	own := func(secret string) string {
		return `EXISTS (SELECT 1 FROM tracks ts WHERE ts.show_id = ` + alias + `.uuid AND ts.secret = ` + secret + `)`
	}
	noTracks := `NOT EXISTS (SELECT 1 FROM tracks ts WHERE ts.show_id = ` + alias + `.uuid)`
	switch v.Mode {
	case ModeSecret:
		return `(` + own("1") + ` OR (` + noTracks + ` AND ` + alias + `.secret = 1))`
	case ModeAll:
		return "1"
	}
	return `(` + own("0") + ` OR (` + noTracks + ` AND ` + alias + `.secret = 0))`
}

type viewerKey struct{}

// WithViewer stores v in ctx, for request handlers and templates.
func WithViewer(ctx context.Context, v Viewer) context.Context {
	return context.WithValue(ctx, viewerKey{}, v)
}

// ViewerFrom returns the viewer stored in ctx, or Normal.
func ViewerFrom(ctx context.Context) Viewer {
	v, _ := ctx.Value(viewerKey{}).(Viewer)
	return v
}

// SetSecret moves a track to secret mode or back to normal mode. A track in
// secret mode is on no release, so it leaves its releases (and a release it
// leaves empty is deleted), and its playlist entries in playlists of the
// other mode go. It stays without a release when made normal again.
func SetSecret(trackID string, secret bool) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setSecret(tx, trackID, secret); err != nil {
		return err
	}
	return tx.Commit()
}

func setSecret(tx *sql.Tx, trackID string, secret bool) error {
	if err := expectRow(tx.Exec(`UPDATE tracks SET secret = ? WHERE uuid = ?;`, secret, trackID)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM playlist_tracks WHERE track_id = ? AND playlist_id IN (SELECT uuid FROM playlists WHERE secret != ?);`, trackID, secret); err != nil {
		return err
	}
	if !secret {
		return nil
	}
	rows, err := tx.Query(`SELECT release_id FROM release_tracks WHERE track_id = ?;`, trackID)
	if err != nil {
		return err
	}
	var releases []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		releases = append(releases, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM release_tracks WHERE track_id = ?;`, trackID); err != nil {
		return err
	}
	for _, id := range releases {
		var left int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM release_tracks WHERE release_id = ?;`, id).Scan(&left); err != nil {
			return err
		}
		if left > 0 {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM releases WHERE uuid = ?;`, id); err != nil {
			return err
		}
		if rid, err := uuid.Parse(id); err == nil {
			defer removeCover(rid)
		}
	}
	return nil
}

// ReleasesOfTrack lists the releases a track is on, for the confirm dialog
// that warns before it is made secret.
func ReleasesOfTrack(trackID string) ([]*audio.Release, error) {
	t := &audio.Track{}
	t.ID = uuid.MustParse(trackID)
	if err := attachReleases([]*audio.Track{t}); err != nil {
		return nil, err
	}
	return t.Releases, nil
}
