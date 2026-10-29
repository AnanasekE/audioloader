package ytdlp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func fake(t *testing.T) *Client {
	t.Helper()
	bin, err := filepath.Abs("testdata/fake-yt-dlp")
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(bin, "true") // ffmpeg isn't run by the fake
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParse(t *testing.T) {
	cases := map[string]Ref{
		"https://www.youtube.com/watch?v=aqz-KE-bpKQ":                              {Video: "aqz-KE-bpKQ"},
		"https://youtu.be/aqz-KE-bpKQ?si=abc":                                      {Video: "aqz-KE-bpKQ"},
		"https://music.youtube.com/watch?v=aqz-KE-bpKQ&list=PLabcdefghij":          {Video: "aqz-KE-bpKQ", Playlist: "PLabcdefghij"},
		"https://www.youtube.com/shorts/aqz-KE-bpKQ":                               {Video: "aqz-KE-bpKQ"},
		"https://www.youtube.com/playlist?list=PLa1F2ddGya_-UvuAqHAksYnB0qL9yWDO6": {Playlist: "PLa1F2ddGya_-UvuAqHAksYnB0qL9yWDO6"},
	}
	for raw, want := range cases {
		got, err := Parse(raw)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, bad := range []string{
		"https://evil.example/watch?v=aqz-KE-bpKQ",
		"https://www.youtube.com.evil.example/watch?v=aqz-KE-bpKQ",
		"file:///etc/passwd",
		"https://www.youtube.com/watch?v=--exec=rm",
		"https://www.youtube.com/watch",
		"-o/tmp/x",
	} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
}

func TestInfo(t *testing.T) {
	c := fake(t)
	info, err := c.Info(context.Background(), "https://www.youtube.com/playlist?list=PLa1F2ddGya_-UvuAqHAksYnB0qL9yWDO6")
	if err != nil {
		t.Fatal(err)
	}
	if info.Type != "playlist" || info.Title != "Mezzanine" || len(info.Videos()) != 3 {
		t.Fatalf("playlist = %+v", info)
	}
	info, err = c.Info(context.Background(), VideoURL("aqz-KE-bpKQ"))
	if err != nil || len(info.Videos()) != 1 || info.Videos()[0].ID != "aqz-KE-bpKQ" || info.Videos()[0].Title == "" || info.Videos()[0].ABR == 0 {
		t.Fatalf("video = %+v, %v", info, err)
	}
}

func TestDownload(t *testing.T) {
	c := fake(t)
	dir := t.TempDir()
	var got []float64
	path, err := c.Download(context.Background(), "aqz-KE-bpKQ", dir, func(p float64) { got = append(got, p) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2] != 1 {
		t.Fatalf("progress = %v", got)
	}
	if _, err := c.Download(context.Background(), "failfailfai", dir, nil); err == nil {
		t.Fatal("failed download returned no error")
	}
}

func TestSearch(t *testing.T) {
	c := fake(t)
	res, err := c.Search(context.Background(), "Massive Attack - Teardrop", 5)
	if err != nil {
		t.Fatal(err)
	}
	// the private video is dropped; the rest keep yt-dlp's order
	if len(res) != 5 || res[0].ID != "aqz-KE-bpKQ" || !res[0].ChannelIsVerified || res[1].Channel != "Massive Attack - Topic" || res[1].ChannelIsVerified {
		t.Fatalf("results = %+v", res)
	}
	if res[2].Duration != 243 || res[0].Thumbnail != "https://i.ytimg.com/vi/aqz-KE-bpKQ/mqdefault.jpg" {
		t.Fatalf("results = %+v", res)
	}
	if res, _ := c.Search(context.Background(), "x", 2); len(res) != 2 {
		t.Fatalf("limit ignored: %d results", len(res))
	}
	for _, bad := range []string{"", "  \n "} {
		if _, err := c.Search(context.Background(), bad, 5); err == nil {
			t.Errorf("Search(%q) accepted", bad)
		}
	}
}

// testdata/music_search.json is the flat list yt-dlp 2026.08.19 prints for
// `-J --flat-playlist "https://music.youtube.com/search?q=...#songs"`: only
// id and title per entry. The lengths come from video-<id>.json, which
// fake-yt-dlp serves for the first three.
func TestMusicSearch(t *testing.T) {
	c := fake(t)
	res, err := c.MusicSearch(context.Background(), "Massive Attack Teardrop", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 5 || res[1].ID != "u7qY1uQ2l0w" || res[1].Duration != 330 || ArtistName(res[1]) != "Massive Attack" ||
		len(res[1].Artists) != 1 || res[1].Thumbnail != "https://i.ytimg.com/vi/u7qY1uQ2l0w/mqdefault.jpg" {
		t.Fatalf("results = %+v", res)
	}
	// only the top 3 are looked up
	if res[0].Duration != 243 || res[2].Duration != 380 || res[3].Duration != 0 || res[4].Duration != 0 {
		t.Fatalf("durations = %v %v %v %v %v", res[0].Duration, res[1].Duration, res[2].Duration, res[3].Duration, res[4].Duration)
	}
	if res, _ := c.MusicSearch(context.Background(), "x", 2); len(res) != 2 {
		t.Fatalf("limit ignored: %d results", len(res))
	}
	for _, bad := range []string{"", "  \n "} {
		if _, err := c.MusicSearch(context.Background(), bad, 5); err == nil {
			t.Errorf("MusicSearch(%q) accepted", bad)
		}
	}
}
