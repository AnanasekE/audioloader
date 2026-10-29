// Package trim cuts silence off the end of a track's file with ffmpeg.
package trim

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"audioloader/internal/audio"
)

// ErrUnavailable is returned when ffmpeg isn't installed.
var ErrUnavailable = errors.New("ffmpeg needs to be installed")

// Client runs ffmpeg. FFmpeg comes from FFMPEG, defaulting to ffmpeg on PATH.
type Client struct{ FFmpeg string }

var (
	defaultOnce sync.Once
	defaultC    *Client
	defaultErr  error
)

// New finds ffmpeg; an empty name means "ffmpeg" on PATH.
func New(ffmpeg string) (*Client, error) {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	f, err := exec.LookPath(ffmpeg)
	if err != nil {
		return nil, fmt.Errorf("%w: %s not found", ErrUnavailable, ffmpeg)
	}
	return &Client{FFmpeg: f}, nil
}

// Default returns the shared client, or ErrUnavailable (with the reason)
// if ffmpeg is missing.
func Default() (*Client, error) {
	defaultOnce.Do(func() {
		defaultC, defaultErr = New(os.Getenv("FFMPEG"))
	})
	return defaultC, defaultErr
}

// SetDefault replaces the shared client; for tests.
func SetDefault(c *Client, err error) {
	defaultOnce.Do(func() {})
	defaultC, defaultErr = c, err
}

const (
	Noise  = "-50dB"
	MinGap = 2 * time.Second
	Tail   = time.Second // kept after the silence starts
	MinCut = time.Second // smaller savings aren't offered
)

// Silence is one quiet stretch; End is 0 when it runs to the end.
type Silence struct{ Start, End time.Duration }

// Plan is what a trim would do: Total is the file's length now, CutAt the
// new length; CutAt 0 means there's nothing to trim.
type Plan struct{ Total, CutAt time.Duration }

func (p Plan) None() bool { return p.CutAt == 0 }

// Saved is how much the trim removes.
func (p Plan) Saved() time.Duration { return p.Total - p.CutAt }

var (
	silenceStart = regexp.MustCompile(`silence_start: (-?[0-9.]+)`)
	silenceEnd   = regexp.MustCompile(`silence_end: (-?[0-9.]+)`)
)

func seconds(s string) time.Duration {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}

// ParseSilences reads silencedetect's log lines
// ("silence_start: 251.2", "silence_end: 260 | silence_duration: 8.8").
func ParseSilences(log string) []Silence {
	var out []Silence
	for _, line := range strings.Split(log, "\n") {
		if m := silenceStart.FindStringSubmatch(line); m != nil {
			out = append(out, Silence{Start: seconds(m[1])})
		} else if m := silenceEnd.FindStringSubmatch(line); m != nil && len(out) > 0 {
			out[len(out)-1].End = seconds(m[1])
		}
	}
	return out
}

// TrailingCut applies the rules to the silences of a file of length total:
// only the last silence counts, and only if it runs to the end of the file.
// The cut is Tail after it starts; savings under MinCut, or a silence that
// starts in the first second (an all-silent file), are not offered.
func TrailingCut(silences []Silence, total time.Duration) Plan {
	if len(silences) == 0 {
		return Plan{Total: total}
	}
	last := silences[len(silences)-1]
	if last.End != 0 && last.End < total-500*time.Millisecond {
		return Plan{Total: total}
	}
	cut := last.Start + Tail
	if last.Start < time.Second || total-cut < MinCut {
		return Plan{Total: total}
	}
	return Plan{Total: total, CutAt: cut}
}

// Detect plans the trim of the file at path.
func (c *Client) Detect(ctx context.Context, path string) (Plan, error) {
	f, err := os.Open(path)
	if err != nil {
		return Plan{}, err
	}
	total, err := audio.ProbeDuration(f)
	f.Close()
	if err != nil {
		return Plan{}, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Plan{}, err
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.FFmpeg, "-hide_banner", "-nostdin", "-nostats", "-i", abs,
		"-map", "0:a:0", "-af", "silencedetect=noise="+Noise+":d=2", "-f", "null", "-")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Plan{}, fmt.Errorf("ffmpeg: %w: %s", err, tail(stderr.String()))
	}
	return TrailingCut(ParseSilences(stderr.String()), total), nil
}

// codecs are the audio encoders for the re-encode fallback, by extension.
var codecs = map[string][]string{
	".mp3":  {"libmp3lame", "-q:a", "2"},
	".m4a":  {"aac", "-b:a", "256k"},
	".mp4":  {"aac", "-b:a", "256k"},
	".ogg":  {"libopus", "-b:a", "160k"},
	".opus": {"libopus", "-b:a", "160k"},
	".flac": {"flac"},
	".wav":  {"pcm_s16le"},
}

// Cut writes src's first at of audio to dst (same extension), stream-
// copied, re-encoding only if the copy fails or misses at by > 0.5 s.
// It returns dst's length.
func (c *Client) Cut(ctx context.Context, src, dst string, at time.Duration) (time.Duration, error) {
	ext := strings.ToLower(filepath.Ext(dst))
	codec, ok := codecs[ext]
	if !ok {
		return 0, fmt.Errorf("can't trim %s files", ext)
	}
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return 0, err
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return 0, err
	}
	t := fmt.Sprintf("%.3f", at.Seconds())
	run := func(codecArgs ...string) (time.Duration, error) {
		args := append([]string{"-hide_banner", "-nostdin", "-y", "-i", absSrc, "-map", "0", "-map_metadata", "0", "-t", t}, codecArgs...)
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, c.FFmpeg, append(args, absDst)...)
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return 0, fmt.Errorf("ffmpeg: %w: %s", err, tail(stderr.String()))
		}
		f, err := os.Open(absDst)
		if err != nil {
			return 0, err
		}
		defer f.Close()
		return audio.ProbeDuration(f)
	}
	d, err := run("-c", "copy")
	if err == nil && math.Abs((d-at).Seconds()) <= 0.5 {
		return d, nil
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	return run(append([]string{"-c:v", "copy", "-c:a"}, codec...)...)
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = "…" + s[len(s)-500:]
	}
	return s
}
