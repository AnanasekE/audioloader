// Package ytdlp downloads audio from YouTube URLs with the yt-dlp program.
// URLs come from the user (pasted on a download, a release or the upload
// page). They are checked against YouTube's hosts and rebuilt from their
// video or playlist ID, so only a canonical URL ever reaches yt-dlp, and
// yt-dlp runs without a shell and without user-supplied flags.
package ytdlp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"audioloader/internal/sources"
)

// Name is the source name stored on downloads.
const Name = "yt-dlp"

// Timeout caps one video's download.
var Timeout = 10 * time.Minute

var (
	// ErrBadURL is returned for anything that isn't a YouTube video or playlist URL.
	ErrBadURL = errors.New("not a YouTube video or playlist URL")
	// ErrUnavailable is returned when yt-dlp or ffmpeg isn't installed.
	ErrUnavailable = errors.New("yt-dlp and ffmpeg need to be installed")
)

var (
	hosts      = map[string]bool{"youtube.com": true, "www.youtube.com": true, "m.youtube.com": true, "music.youtube.com": true, "youtu.be": true}
	videoID    = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	playlistID = regexp.MustCompile(`^[A-Za-z0-9_-]{10,64}$`)
	percent    = regexp.MustCompile(`([0-9.]+)%`)
)

// Ref is a parsed YouTube URL.
type Ref struct {
	Video    string // video ID, if the URL names one
	Playlist string // playlist ID, if the URL names one
}

// VideoURL is the canonical URL of the video.
func (r Ref) VideoURL() string { return VideoURL(r.Video) }

// PlaylistURL is the canonical URL of the playlist.
func (r Ref) PlaylistURL() string {
	return "https://www.youtube.com/playlist?list=" + r.Playlist
}

// VideoURL is the canonical URL of a video ID.
func VideoURL(id string) string { return "https://www.youtube.com/watch?v=" + id }

// Parse checks that raw is a YouTube URL and extracts its IDs.
func Parse(raw string) (Ref, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !hosts[strings.ToLower(u.Hostname())] {
		return Ref{}, ErrBadURL
	}
	var ref Ref
	q := u.Query()
	switch {
	case strings.EqualFold(u.Hostname(), "youtu.be"):
		ref.Video = strings.Trim(u.Path, "/")
	case u.Path == "/watch":
		ref.Video = q.Get("v")
	case strings.HasPrefix(u.Path, "/shorts/"), strings.HasPrefix(u.Path, "/live/"):
		// /shorts/<id>
		if parts := strings.Split(strings.Trim(u.Path, "/"), "/"); len(parts) == 2 {
			ref.Video = parts[1]
		}
	}
	if l := q.Get("list"); l != "" {
		ref.Playlist = l
	}
	if ref.Video != "" && !videoID.MatchString(ref.Video) {
		return Ref{}, ErrBadURL
	}
	if ref.Playlist != "" && !playlistID.MatchString(ref.Playlist) {
		return Ref{}, ErrBadURL
	}
	if ref.Video == "" && ref.Playlist == "" {
		return Ref{}, ErrBadURL
	}
	return ref, nil
}

// Client runs yt-dlp. Bin and FFmpeg come from YTDLP and FFMPEG, defaulting
// to the programs on PATH.
type Client struct {
	Bin    string
	FFmpeg string
}

var (
	defaultOnce sync.Once
	defaultC    *Client
	defaultErr  error
)

// Default returns the shared client, or ErrUnavailable (with the reason)
// if yt-dlp or ffmpeg is missing.
func Default() (*Client, error) {
	defaultOnce.Do(func() {
		defaultC, defaultErr = New(os.Getenv("YTDLP"), os.Getenv("FFMPEG"))
	})
	return defaultC, defaultErr
}

// New finds the programs; empty names mean "yt-dlp" and "ffmpeg" on PATH.
func New(bin, ffmpeg string) (*Client, error) {
	if bin == "" {
		bin = "yt-dlp"
	}
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	b, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("%w: %s not found", ErrUnavailable, bin)
	}
	f, err := exec.LookPath(ffmpeg)
	if err != nil {
		return nil, fmt.Errorf("%w: %s not found", ErrUnavailable, ffmpeg)
	}
	return &Client{Bin: b, FFmpeg: f}, nil
}

// Version returns yt-dlp's version, e.g. for the log at startup.
func (c *Client) Version(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, c.Bin, "--version").Output()
	return strings.TrimSpace(string(out)), err
}

// Entry is one video of a playlist (or the video itself).
type Entry struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration"` // seconds
	Channel  string  `json:"channel"`
	Uploader string  `json:"uploader"`
	ABR      float64 `json:"abr"` // audio bitrate, kbit/s; only for single videos
}

// Info is what yt-dlp knows about a URL.
type Info struct {
	Type    string  `json:"_type"` // "video" or "playlist"
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Entries []Entry `json:"entries"`
	Entry           // the video's own fields when Type is "video"
}

// Videos returns the playlist's entries, or the video itself.
func (i *Info) Videos() []Entry {
	if i.Type == "playlist" {
		return i.Entries
	}
	return []Entry{i.Entry}
}

// Info fetches the metadata of a video or playlist without downloading.
func (c *Client) Info(ctx context.Context, canonicalURL string) (*Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Bin, "-J", "--flat-playlist", "--no-warnings", "--", canonicalURL)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp: %w: %s", err, tail(stderr.String()))
	}
	var info Info
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("yt-dlp output: %w", err)
	}
	// the flat list leaves unavailable videos in with a placeholder title
	entries := info.Entries[:0]
	for _, e := range info.Entries {
		if videoID.MatchString(e.ID) && e.Title != "[Deleted video]" && e.Title != "[Private video]" {
			entries = append(entries, e)
		}
	}
	info.Entries = entries
	return &info, nil
}

// Download fetches a video's audio into dir as Opus, reporting progress
// (0..1), and returns the file's path. Opus sources are only remuxed into
// Ogg, which the duration reader handles; others are converted.
func (c *Client) Download(ctx context.Context, id, dir string, progress func(float64)) (string, error) {
	if !videoID.MatchString(id) {
		return "", ErrBadURL
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin,
		"-f", "bestaudio",
		"-x", "--audio-format", "opus",
		"--ffmpeg-location", c.FFmpeg,
		"--no-playlist",
		"--no-warnings",
		"--newline",
		"--progress-template", "download:%(progress._percent_str)s",
		"-o", filepath.Join(dir, "%(id)s.%(ext)s"),
		"--", VideoURL(id),
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if m := percent.FindStringSubmatch(sc.Text()); m != nil && progress != nil {
			if p, err := strconv.ParseFloat(m[1], 64); err == nil {
				progress(p / 100)
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("yt-dlp: %w: %s", err, tail(stderr.String()))
	}
	path := filepath.Join(dir, id+".opus")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("yt-dlp didn't produce %s", filepath.Base(path))
	}
	return path, nil
}

// tail keeps the end of yt-dlp's error output, where the reason is.
func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = "…" + s[len(s)-500:]
	}
	return s
}

// Source adapts the client to sources.Source. It only fetches candidates
// made from pasted URLs (see Candidate); it doesn't search YouTube.
type Source struct{ C *Client }

func (Source) Name() string { return Name }

func (Source) Find(context.Context, sources.Want) ([]sources.Candidate, error) { return nil, nil }

// Candidate makes a candidate from a video ID.
func Candidate(id string, abr float64) sources.Candidate {
	return sources.Candidate{Source: Name, Ref: id, Label: VideoURL(id), Bitrate: int(abr)}
}

func (s Source) Fetch(ctx context.Context, c sources.Candidate, progress func(float64)) (sources.File, error) {
	dir, err := os.MkdirTemp("", "audioloader-ytdlp-*")
	if err != nil {
		return sources.File{}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	path, err := s.C.Download(ctx, c.Ref, dir, progress)
	if err != nil {
		cleanup()
		return sources.File{}, err
	}
	return sources.File{Path: path, Cleanup: cleanup}, nil
}
