package spotdl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/sources"
)

func fake(t *testing.T) *Client {
	t.Helper()
	bin, err := filepath.Abs("testdata/fake-spotdl")
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(bin, "true") // ffmpeg isn't run by the fake
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParseURL(t *testing.T) {
	const id = "2dgjBzSYiTVqlSc3u1Yosb"
	kind, u, err := ParseURL("https://open.spotify.com/intl-de/track/" + id + "?si=x")
	if err != nil || kind != "track" || u != "https://open.spotify.com/track/"+id {
		t.Fatalf("ParseURL = %q, %q, %v", kind, u, err)
	}
	if kind, _, err := ParseURL("https://open.spotify.com/album/" + id); err != nil || kind != "album" {
		t.Fatalf("album: %q, %v", kind, err)
	}
	for _, bad := range []string{
		"https://open.spotify.com/playlist/" + id,
		"https://open.spotify.com/artist/" + id,
		"https://evil.example/track/" + id,
		"https://open.spotify.com.evil.example/track/" + id,
		"-o/tmp/x",
		"",
	} {
		if _, _, err := ParseURL(bad); err == nil {
			t.Errorf("ParseURL(%q) accepted", bad)
		}
	}
}

func TestQuery(t *testing.T) {
	for _, c := range []struct{ artist, title, want string }{
		{"Massive Attack", "Teardrop", "Massive Attack - Teardrop"},
		{"-Evil", "--x", "Evil - --x"},
		{"A|B", "Song", "A B - Song"},
		{"album:Foo", "Song", "album Foo - Song"},
		{"", "Song", "Song"},
	} {
		got, err := Query(c.artist, c.title)
		if err != nil || got != c.want {
			t.Errorf("Query(%q, %q) = %q, %v; want %q", c.artist, c.title, got, err, c.want)
		}
	}
	if _, err := Query("https://x.example/y", "Song"); err == nil {
		t.Error("Query accepted a URL")
	}
}

func TestSave(t *testing.T) {
	c := fake(t)
	songs, err := c.Save(context.Background(), "https://open.spotify.com/album/0sNOF9WDwhWunNAHPD3Baj")
	if err != nil || len(songs) != 3 || songs[1].Name != "Teardrop" || songs[1].Length() != 2*time.Second ||
		songs[1].URL != "https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb" || songs[1].Artists[0] != "Massive Attack" {
		t.Fatalf("songs = %+v, %v", songs, err)
	}
}

func TestDownload(t *testing.T) {
	c := fake(t)
	dir := t.TempDir()
	path, err := c.Download(context.Background(), "https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb?si=x", dir)
	if err != nil || filepath.Base(path) != "2dgjBzSYiTVqlSc3u1Yosb.opus" {
		t.Fatalf("path = %q, %v", path, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	// only track links are downloaded
	if _, err := c.Download(context.Background(), "https://open.spotify.com/album/0sNOF9WDwhWunNAHPD3Baj", t.TempDir()); err == nil {
		t.Fatal("downloaded an album URL")
	}
	_, err = c.Download(context.Background(), "https://open.spotify.com/track/failfailfailfailfail12", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no match") {
		t.Fatalf("err = %v", err)
	}
}

func TestFind(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("FAKE_SPOTDL_LOG", log)
	s := Source{C: fake(t)}
	ctx := context.Background()

	// an exact link needs no search
	cands, err := s.Find(ctx, sources.Want{Title: "Teardrop", SpotifyURL: "https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb"})
	if err != nil || len(cands) != 1 || cands[0].Ref != "https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb" {
		t.Fatalf("link: %+v, %v", cands, err)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("spotdl ran for an exact link")
	}
	// a link that isn't a track is ignored
	if cands, _ := s.Find(ctx, sources.Want{SpotifyURL: "https://evil.example/track/x"}); len(cands) != 0 {
		t.Fatalf("bad link: %+v", cands)
	}

	// search: title and length fit
	cands, err = s.Find(ctx, sources.Want{Title: "Teardrop", Artists: []string{"Massive Attack"}, Duration: 2 * time.Second})
	if err != nil || len(cands) != 1 || cands[0].Ref != "https://open.spotify.com/track/3yLSbYwZ7MkjfVvLtdb2Vr" {
		t.Fatalf("search: %+v, %v", cands, err)
	}
	calls, _ := os.ReadFile(log)
	if string(calls) != "save Massive Attack - Teardrop\n" {
		t.Fatalf("calls = %q", calls)
	}

	// search: the right title but 6 minutes instead of 2 s is rejected
	cands, err = s.Find(ctx, sources.Want{Title: "Angel", Artists: []string{"Massive Attack"}, Duration: 2 * time.Second})
	if err != nil || len(cands) != 0 {
		t.Fatalf("length mismatch accepted: %+v, %v", cands, err)
	}
	// search: another title is rejected
	cands, _ = s.Find(ctx, sources.Want{Title: "Teardrop (Mad Professor Remix)", Artists: []string{"Massive Attack"}, Duration: 2 * time.Second})
	if len(cands) != 0 {
		t.Fatalf("title mismatch accepted: %+v", cands)
	}
	// no known length, no way to check
	cands, _ = s.Find(ctx, sources.Want{Title: "Teardrop", Artists: []string{"Massive Attack"}})
	if len(cands) != 0 {
		t.Fatalf("unchecked result accepted: %+v", cands)
	}
}

func TestFetch(t *testing.T) {
	s := Source{C: fake(t)}
	f, err := s.Fetch(context.Background(), sources.Candidate{Source: Name, Ref: "https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Cleanup()
	r, err := os.Open(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if d, err := audio.ProbeDuration(r); err != nil || d.Round(time.Second) != 2*time.Second {
		t.Fatalf("duration = %v, %v", d, err)
	}
}
