package folder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"audioloader/internal/sources"
)

func copyFixture(t *testing.T, dir, name, as string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../tags/testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, as), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFind(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	copyFixture(t, dir, "tagged.mp3", "sub/a.mp3")
	copyFixture(t, dir, "tagged.flac", "b.flac")
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	s := &Source{Dir: dir}

	// by MusicBrainz ID: both fixtures carry it; the FLAC ranks first
	cands, err := s.Find(context.Background(), sources.Want{Title: "whatever", MBID: "f3bba4cd-8018-468b-902e-bc8f029593e5"})
	if err != nil {
		t.Fatal(err)
	}
	sources.Rank(cands, nil)
	if len(cands) != 2 || !cands[0].Lossless || filepath.Base(cands[0].Ref) != "b.flac" {
		t.Fatalf("cands = %+v", cands)
	}

	// another MBID doesn't match, even with the same title
	cands, _ = s.Find(context.Background(), sources.Want{Title: "Teardrop", Artists: []string{"Massive Attack"}, MBID: "00000000-0000-0000-0000-000000000000"})
	if len(cands) != 0 {
		t.Fatalf("matched a different recording: %+v", cands)
	}

	// a removed file drops out of the index
	os.Remove(filepath.Join(dir, "b.flac"))
	cands, _ = s.Find(context.Background(), sources.Want{MBID: "f3bba4cd-8018-468b-902e-bc8f029593e5"})
	if len(cands) != 1 {
		t.Fatalf("after removal: %+v", cands)
	}
	f, err := s.Fetch(context.Background(), cands[0], nil)
	if err != nil || f.Path != cands[0].Ref {
		t.Fatalf("fetch = %+v, %v", f, err)
	}
}
