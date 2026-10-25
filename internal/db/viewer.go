package db

import "context"

// Viewer is who is reading: secret tracks are only visible to a viewer
// who unlocked the vault. Every read of tracks takes one, so the compiler
// finds each place that has to filter them; background jobs pass All.
type Viewer struct {
	Secret bool
}

var (
	// Public sees everything except secret tracks and what only they make up.
	Public = Viewer{}
	// All sees everything: background jobs, and unlocked viewers.
	All = Viewer{Secret: true}
)

// The conditions below are SQL on a table aliased alias. A locked viewer
// doesn't see secret tracks, nor releases, artists and shows that only
// secret tracks make up; ones with no tracks at all stay visible.

func (v Viewer) trackCond(alias string) string {
	if v.Secret {
		return "1"
	}
	return alias + ".secret = 0"
}

func (v Viewer) releaseCond(alias string) string {
	if v.Secret {
		return "1"
	}
	return `NOT (
		EXISTS (SELECT 1 FROM release_tracks rs JOIN tracks ts ON ts.uuid = rs.track_id WHERE rs.release_id = ` + alias + `.uuid AND ts.secret = 1)
		AND NOT EXISTS (SELECT 1 FROM release_tracks rs JOIN tracks ts ON ts.uuid = rs.track_id WHERE rs.release_id = ` + alias + `.uuid AND ts.secret = 0))`
}

func (v Viewer) artistCond(alias string) string {
	if v.Secret {
		return "1"
	}
	return `NOT (
		EXISTS (SELECT 1 FROM track_artist_map ms JOIN tracks ts ON ts.uuid = ms.track_id WHERE ms.artist_id = ` + alias + `.uuid AND ts.secret = 1)
		AND NOT EXISTS (SELECT 1 FROM track_artist_map ms JOIN tracks ts ON ts.uuid = ms.track_id WHERE ms.artist_id = ` + alias + `.uuid AND ts.secret = 0)
		AND NOT EXISTS (SELECT 1 FROM release_artist_map ma JOIN releases ra ON ra.uuid = ma.release_id WHERE ma.artist_id = ` + alias + `.uuid AND ` + v.releaseCond("ra") + `))`
}

func (v Viewer) showCond(alias string) string {
	if v.Secret {
		return "1"
	}
	return `NOT (
		EXISTS (SELECT 1 FROM tracks ts WHERE ts.show_id = ` + alias + `.uuid AND ts.secret = 1)
		AND NOT EXISTS (SELECT 1 FROM tracks ts WHERE ts.show_id = ` + alias + `.uuid AND ts.secret = 0))`
}

type viewerKey struct{}

// WithViewer stores v in ctx, for request handlers and templates.
func WithViewer(ctx context.Context, v Viewer) context.Context {
	return context.WithValue(ctx, viewerKey{}, v)
}

// ViewerFrom returns the viewer stored in ctx, or Public.
func ViewerFrom(ctx context.Context) Viewer {
	v, _ := ctx.Value(viewerKey{}).(Viewer)
	return v
}

// SetSecret makes a track secret or public.
func SetSecret(trackID string, secret bool) error {
	return expectRow(DB.Exec(`UPDATE tracks SET secret = ? WHERE uuid = ?;`, secret, trackID))
}
