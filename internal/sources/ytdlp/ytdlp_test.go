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
	if info.Type != "playlist" || len(info.Videos()) != 3 {
		t.Fatalf("playlist = %+v", info)
	}
	info, err = c.Info(context.Background(), VideoURL("aqz-KE-bpKQ"))
	if err != nil || len(info.Videos()) != 1 || info.Videos()[0].ABR == 0 {
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
