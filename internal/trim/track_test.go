package trim

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
)

func TestApplyToTrack(t *testing.T) {
	dbtest.New(t)
	src := toneThenSilence(t, t.TempDir())
	c, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	f, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	a, _ := db.AddArtist(db.All, "Ella Red")
	res, err := db.AddTrack(db.All, []string{a.ID.String()}, &audio.Track{Title: "Muse", Duration: 8 * time.Second}, f, ".flac", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.MustParse(res.TrackID)
	if err := db.SavePosition(res.TrackID, 7, false); err != nil {
		t.Fatal(err)
	}
	fingerprint := func() (size int64, hash string) {
		db.DB.QueryRow(`SELECT file_size, head_hash FROM tracks WHERE uuid = ?;`, res.TrackID).Scan(&size, &hash)
		return
	}
	size, hash := fingerprint()
	path, _ := db.TrackFilePath(id)
	before, _ := os.Stat(path)

	if plan, err := c.Preview(ctx, id); err != nil || !near(plan.CutAt, 4*time.Second) {
		t.Fatalf("preview = %+v, %v", plan, err)
	}
	plan, err := c.Apply(ctx, id)
	if err != nil || plan.None() {
		t.Fatalf("apply = %+v, %v", plan, err)
	}
	after, _ := os.Stat(path)
	if after.Size() >= before.Size() {
		t.Fatalf("file is %d bytes, was %d", after.Size(), before.Size())
	}
	tr, err := db.GetTrack(db.All, res.TrackID)
	if err != nil || tr.Duration != 4*time.Second {
		t.Fatalf("track = %+v, %v", tr, err)
	}
	var pos float64
	db.DB.QueryRow(`SELECT seconds FROM playback_positions WHERE track_id = ?;`, res.TrackID).Scan(&pos)
	if pos != 4 {
		t.Fatalf("saved position = %v", pos)
	}
	if s, h := fingerprint(); s != size || h != hash {
		t.Fatalf("fingerprint changed: %d %s", s, h)
	}
	if left, _ := filepath.Glob(filepath.Join(db.TracksDir, ".trim-*")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}

	// nothing left to trim
	if plan, err := c.Apply(ctx, id); err != nil || !plan.None() {
		t.Fatalf("second apply = %+v, %v", plan, err)
	}
}
