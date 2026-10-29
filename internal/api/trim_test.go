package api

import (
	"bytes"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/trim"
)

func TestTrimNeedsFFmpeg(t *testing.T) {
	a := newModeApp(t)
	trim.SetDefault(nil, trim.ErrUnavailable)
	id := a.ID["public track"]
	before, _ := db.GetTrack(db.Normal, id)

	if body := a.do(false, "GET", "/ui/tracks/"+id+"/trim", nil).Body.String(); !strings.Contains(body, "ffmpeg needs to be installed") {
		t.Fatalf("preview:\n%s", body)
	}
	rec := a.do(false, "POST", "/ui/tracks/"+id+"/trim", url.Values{})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "ffmpeg needs to be installed") || rec.Header().Get("HX-Location") != "" {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	if after, _ := db.GetTrack(db.Normal, id); after.Duration != before.Duration {
		t.Fatalf("duration changed: %v", after.Duration)
	}
}

func TestTrimHidesTracks(t *testing.T) {
	a := newModeApp(t)
	trim.SetDefault(nil, trim.ErrUnavailable)
	for _, target := range []string{
		"/ui/tracks/00000000-0000-0000-0000-000000000000/trim",
		"/ui/tracks/" + a.ID["secret track"] + "/trim", // in normal mode
	} {
		if rec := a.do(false, "GET", target, nil); rec.Code != 404 {
			t.Errorf("GET %s = %d", target, rec.Code)
		}
		if rec := a.do(false, "POST", target, url.Values{}); rec.Code != 404 {
			t.Errorf("POST %s = %d", target, rec.Code)
		}
	}
	if rec := a.do(true, "GET", "/ui/tracks/"+a.ID["secret track"]+"/trim", nil); rec.Code != 200 {
		t.Errorf("secret mode: %d", rec.Code)
	}
}

func TestTrimWithFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg isn't installed")
	}
	a := newModeApp(t)
	c, err := trim.New(ffmpeg)
	if err != nil {
		t.Fatal(err)
	}
	trim.SetDefault(c, nil)

	// 3 s of tone, then 5 s of silence
	src := filepath.Join(t.TempDir(), "tone.flac")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3,apad=pad_dur=5", "-c:a", "flac", src).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.AddTrack(db.Normal, []string{a.ID["public artist"]}, &audio.Track{Title: "ZZPUBLIC Silent End", Duration: 8 * time.Second},
		bytes.NewReader(data), ".flac", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	target := "/ui/tracks/" + res.TrackID + "/trim"

	if body := a.do(false, "GET", target, nil).Body.String(); !strings.Contains(body, "0:08 → 0:04") || !strings.Contains(body, "This can&#39;t be undone") {
		t.Fatalf("preview:\n%s", body)
	}
	if rec := a.do(false, "POST", target, url.Values{}); rec.Code != 204 || rec.Header().Get("HX-Location") == "" {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	if tr, _ := db.GetTrack(db.Normal, res.TrackID); tr.Duration != 4*time.Second {
		t.Fatalf("duration = %v", tr.Duration)
	}
	if body := a.do(false, "GET", "/tracks", nil).Body.String(); !strings.Contains(body, "0:04") {
		t.Fatalf("track list lacks the new length")
	}
	if body := a.do(false, "GET", target, nil).Body.String(); !strings.Contains(body, "No silence at the end") {
		t.Fatalf("second preview:\n%s", body)
	}
}
