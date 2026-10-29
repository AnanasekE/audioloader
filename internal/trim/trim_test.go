package trim

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func sec(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

func TestParseSilences(t *testing.T) {
	log, err := os.ReadFile("testdata/silencedetect.log")
	if err != nil {
		t.Fatal(err)
	}
	got := ParseSilences(string(log))
	want := []Silence{{sec(2), sec(5.000023)}, {sec(7), sec(13)}}
	if len(got) != len(want) {
		t.Fatalf("silences = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("silence %d = %v, want %v", i, got[i], want[i])
		}
	}
	// a silence still running at the end has no end line
	if got := ParseSilences("[silencedetect @ 0x1] silence_start: 251.2\n"); len(got) != 1 || got[0].End != 0 {
		t.Fatalf("open silence = %v", got)
	}
}

func TestTrailingCut(t *testing.T) {
	total := sec(13)
	cases := []struct {
		name string
		in   []Silence
		want time.Duration
	}{
		{"no end", []Silence{{Start: sec(7)}}, sec(8)},
		{"ends at total", []Silence{{sec(2), sec(5)}, {sec(7), sec(13)}}, sec(8)},
		{"only a middle silence", []Silence{{sec(2), sec(5)}}, 0},
		{"saves under a second", []Silence{{Start: sec(11.5)}}, 0},
		{"starts in the first second", []Silence{{Start: sec(0.3)}}, 0},
		{"no silences", nil, 0},
	}
	for _, c := range cases {
		p := TrailingCut(c.in, total)
		if p.CutAt != c.want || p.Total != total || p.None() != (c.want == 0) {
			t.Errorf("%s: plan = %+v, want cut at %v", c.name, p, c.want)
		}
	}
}

// toneThenSilence writes an 8 s FLAC: 3 s of tone, then 5 s of silence.
func toneThenSilence(t *testing.T, dir string) string {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg isn't installed")
	}
	path := filepath.Join(dir, "tone.flac")
	out, err := exec.Command(ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3,apad=pad_dur=5", "-c:a", "flac", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	return path
}

func near(got, want time.Duration) bool {
	d := got - want
	return d > -100*time.Millisecond && d < 100*time.Millisecond
}

func TestCutWithFFmpeg(t *testing.T) {
	dir := t.TempDir()
	src := toneThenSilence(t, dir)
	c, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	plan, err := c.Detect(ctx, src)
	if err != nil || !near(plan.Total, 8*time.Second) || !near(plan.CutAt, 4*time.Second) {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	dst := filepath.Join(dir, "cut.flac")
	d, err := c.Cut(ctx, src, dst, plan.CutAt)
	if err != nil || !near(d, 4*time.Second) {
		t.Fatalf("cut = %v, %v", d, err)
	}
	if again, err := c.Detect(ctx, dst); err != nil || !again.None() {
		t.Fatalf("detect after cut = %+v, %v", again, err)
	}
	if _, err := c.Cut(ctx, src, filepath.Join(dir, "cut.xyz"), plan.CutAt); err == nil {
		t.Fatal("cut to an unknown extension worked")
	}
}

// every stored format can be cut
func TestCutFormats(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg isn't installed")
	}
	c, _ := New("")
	for _, ext := range []string{".mp3", ".m4a", ".opus", ".ogg", ".wav"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "tone"+ext)
			if out, err := exec.Command(ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
				"-f", "lavfi", "-i", "sine=frequency=440:duration=3,apad=pad_dur=5", src).CombinedOutput(); err != nil {
				t.Skipf("ffmpeg can't write %s: %v\n%s", ext, err, out)
			}
			d, err := c.Cut(context.Background(), src, filepath.Join(dir, "cut"+ext), 4*time.Second)
			if err != nil || !near(d, 4*time.Second) {
				t.Fatalf("cut = %v, %v", d, err)
			}
		})
	}
}
