package trim

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"uuid"

	"audioloader/internal/db"
)

// one trim at a time: each decodes a whole file
var mu sync.Mutex

// Preview plans the trim of a track's stored file.
func (c *Client) Preview(ctx context.Context, id uuid.UUID) (Plan, error) {
	path, err := db.TrackFilePath(id)
	if err != nil {
		return Plan{}, err
	}
	return c.Detect(ctx, path)
}

// Apply detects again (never trusting a cut point from the browser), cuts
// into a temp file next to the original, renames it over the original and
// stores the new length. A Plan with None() did nothing.
func (c *Client) Apply(ctx context.Context, id uuid.UUID) (Plan, error) {
	mu.Lock()
	defer mu.Unlock()

	path, err := db.TrackFilePath(id)
	if err != nil {
		return Plan{}, err
	}
	plan, err := c.Detect(ctx, path)
	if err != nil || plan.None() {
		return plan, err
	}
	// the name must not start with the track's ID, or TrackFilePath's glob could find it
	tmp, err := os.CreateTemp(db.TracksDir, ".trim-*"+filepath.Ext(path))
	if err != nil {
		return Plan{}, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name()) // a no-op after the rename

	length, err := c.Cut(ctx, path, tmp.Name(), plan.CutAt)
	if err != nil {
		return Plan{}, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return Plan{}, err
	}
	// a stream already open keeps reading the old file
	return plan, db.SetTrimmedLength(id.String(), length)
}
