// Package spotdl downloads tracks with the spotDL program, which finds each
// Spotify track's audio on YouTube Music and tags it. Like yt-dlp, it runs
// without a shell and without user-supplied flags: Spotify links are
// checked and rebuilt in canonical form, and a search query is cleaned and
// goes after "--" as one argument.
//
// Checked against spotDL 4.5: "save" writes a JSON list of songs with
// "name", "artists", "duration" (whole seconds) and "url"; the --output
// template knows {track-id} and {output-ext}. Global options must come
// before the operation, and "--" right after it.
package spotdl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"audioloader/internal/match"
	"audioloader/internal/mb"
	"audioloader/internal/sources"
)

// Name is the source name stored on downloads.
const Name = "spotdl"

// Timeout caps one track's download; SaveTimeout caps a metadata lookup.
var (
	Timeout     = 10 * time.Minute
	SaveTimeout = 2 * time.Minute
)

// MatchTolerance is how far a search result's length may be from the
// track's, the same rule as mapping a playlist.
const MatchTolerance = 5 * time.Second

var (
	// ErrBadURL is returned for anything that isn't a Spotify track or album URL.
	ErrBadURL = errors.New("not a Spotify track or album URL")
	// ErrUnavailable is returned when spotdl or ffmpeg isn't installed.
	ErrUnavailable = errors.New("spotdl and ffmpeg need to be installed")
)

// searchPrefix matches the prefixes spotDL reads as "search albums" and the like.
var searchPrefix = regexp.MustCompile(`(?i)^\s*(album|playlist|artist):`)

// Client runs spotdl. Bin and FFmpeg come from SPOTDL and FFMPEG,
// defaulting to the programs on PATH.
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
// if spotdl or ffmpeg is missing.
func Default() (*Client, error) {
	defaultOnce.Do(func() {
		defaultC, defaultErr = New(os.Getenv("SPOTDL"), os.Getenv("FFMPEG"))
	})
	return defaultC, defaultErr
}

// New finds the programs; empty names mean "spotdl" and "ffmpeg" on PATH.
func New(bin, ffmpeg string) (*Client, error) {
	if bin == "" {
		bin = "spotdl"
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

// Version returns spotdl's version, e.g. for the log at startup.
func (c *Client) Version(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, c.Bin, "--version").Output()
	return strings.TrimSpace(string(out)), err
}

// Song is what spotDL knows about a track, from "spotdl save".
type Song struct {
	Name     string   `json:"name"`
	Artists  []string `json:"artists"`
	Duration float64  `json:"duration"` // seconds
	URL      string   `json:"url"`      // the track's Spotify URL
}

// Length is the song's duration.
func (s Song) Length() time.Duration { return time.Duration(s.Duration * float64(time.Second)) }

// ParseURL checks that raw is a Spotify track or album URL and returns its
// kind ("track" or "album") and canonical form.
func ParseURL(raw string) (kind, canonical string, err error) {
	canonical = mb.ParseSpotifyURL(raw)
	if canonical == "" {
		return "", "", ErrBadURL
	}
	return strings.Split(canonical, "/")[3], canonical, nil
}

// Query makes the text spotDL searches for: "artist - title". It can't
// become an option, a URL or one of spotDL's query prefixes.
func Query(artist, title string) (string, error) {
	q := strings.Join(strings.Fields(strings.NewReplacer("|", " ", "\x00", " ").Replace(artist+" - "+title)), " ")
	q = strings.TrimLeft(q, "- ")
	if q == "" || strings.Contains(q, "://") {
		return "", errors.New("spotdl: not a usable search query")
	}
	if searchPrefix.MatchString(q) {
		q = strings.Replace(q, ":", " ", 1)
	}
	return q, nil
}

// Save looks up songs without downloading: query is a track or album URL,
// or search text from Query (which gives the top result alone).
func (c *Client) Save(ctx context.Context, query string) ([]Song, error) {
	ctx, cancel := context.WithTimeout(ctx, SaveTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "audioloader-spotdl-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "songs.spotdl")

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Bin, "--save-file", file, "save", "--", query)
	cmd.Stderr = &stderr
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("spotdl: %w: %s", err, tail(stderr.String()))
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("spotdl wrote no song list: %w", err)
	}
	var songs []Song
	if err := json.Unmarshal(data, &songs); err != nil {
		return nil, fmt.Errorf("spotdl song list: %w", err)
	}
	return songs, nil
}

// Download fetches a Spotify track as Opus into dir and returns the file's
// path. Opus is Ogg, which the duration reader handles.
func (c *Client) Download(ctx context.Context, trackURL, dir string) (string, error) {
	kind, canonical, err := ParseURL(trackURL)
	if err != nil || kind != "track" {
		return "", ErrBadURL
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Bin,
		"--format", "opus",
		"--output", filepath.Join(dir, "{track-id}.{output-ext}"),
		"--ffmpeg", c.FFmpeg,
		"download", "--", canonical)
	cmd.Stderr = &stderr
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("spotdl: %w: %s", err, tail(stderr.String()))
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(files) != 1 || files[0].IsDir() {
		return "", fmt.Errorf("spotdl wrote %d files, expected one", len(files))
	}
	return filepath.Join(dir, files[0].Name()), nil
}

// tail keeps the end of spotdl's error output, where the reason is.
func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = "…" + s[len(s)-500:]
	}
	return s
}

// Source adapts the client to sources.Source.
type Source struct{ C *Client }

func (Source) Name() string { return Name }

// Find returns the track's own Spotify link if it has one. Otherwise it
// searches and trusts the top result only if its title matches and its
// length is within MatchTolerance of the track's.
func (s Source) Find(ctx context.Context, want sources.Want) ([]sources.Candidate, error) {
	if want.SpotifyURL != "" {
		kind, u, err := ParseURL(want.SpotifyURL)
		if err != nil || kind != "track" {
			return nil, nil
		}
		return []sources.Candidate{{Source: Name, Ref: u, Label: u}}, nil
	}
	if want.Title == "" || want.Duration <= 0 {
		return nil, nil // a result can't be checked without a length
	}
	artist := ""
	if len(want.Artists) > 0 {
		artist = want.Artists[0]
	}
	q, err := Query(artist, want.Title)
	if err != nil {
		return nil, nil
	}
	songs, err := s.C.Save(ctx, q)
	if err != nil || len(songs) == 0 {
		return nil, err
	}
	top := songs[0]
	if !match.Same(top.Name, want.Title) {
		return nil, nil
	}
	if d := top.Length() - want.Duration; d < -MatchTolerance || d > MatchTolerance {
		return nil, nil
	}
	if _, u, err := ParseURL(top.URL); err == nil {
		return []sources.Candidate{{Source: Name, Ref: u, Label: u}}, nil
	}
	return nil, nil
}

func (s Source) Fetch(ctx context.Context, c sources.Candidate, progress func(float64)) (sources.File, error) {
	dir, err := os.MkdirTemp("", "audioloader-spotdl-*")
	if err != nil {
		return sources.File{}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	path, err := s.C.Download(ctx, c.Ref, dir)
	if err != nil {
		cleanup()
		return sources.File{}, err
	}
	return sources.File{Path: path, Cleanup: cleanup}, nil
}
