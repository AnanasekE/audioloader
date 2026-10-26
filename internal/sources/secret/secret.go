// Package secret downloads a track from "the secret source": one website,
// named privately in AUDIOLOADER_SECRET_SOURCE, whose pages each hold one
// audio file. A pasted page URL is checked against the configured host, the
// page is fetched once to find the audio file, title and description, and the
// file is fetched once from an allowed media host. A page without audio may
// be a listing, such as an uploader's page: its links to pages one level
// below it are listed, and none of them is fetched until it's picked.
package secret

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/mb"
)

var (
	// ErrOff is returned when no secret source is configured.
	ErrOff = errors.New("no secret source is configured")
	// ErrBadURL is returned for a URL that isn't a page of the source.
	ErrBadURL = errors.New("that isn't a page of the secret source")
	// ErrOffHost is returned when a request would leave the allowed hosts.
	ErrOffHost = errors.New("the secret source sent us to a host that isn't allowed")
	// ErrNoAudio is returned for a page without an audio file.
	ErrNoAudio = errors.New("no audio file found on that page")
	// ErrAudioHost is returned when the page names an audio file on a host that isn't allowed.
	ErrAudioHost = errors.New("the audio file is on a host that isn't allowed")
	// ErrTooLarge is returned for a page or audio file over its size cap.
	ErrTooLarge = errors.New("that is larger than allowed")
)

// Source is the configured secret source.
type Source struct {
	// Host is the page host. Page URLs must have exactly this host.
	Host string
	// Media lists the further hosts audio files may live on.
	Media []string
	// Client is the base HTTP client; nil means a plain one. Its redirect
	// policy is replaced by one that refuses hosts that aren't allowed.
	Client    *http.Client
	UserAgent string

	// Caps: a page is small, an audio file isn't.
	MaxPage  int64
	MaxAudio int64
	// Timeouts of the page request and of the audio download.
	PageTimeout  time.Duration
	AudioTimeout time.Duration
}

// FromEnv returns the source named by AUDIOLOADER_SECRET_SOURCE (the page
// host) and AUDIOLOADER_SECRET_SOURCE_MEDIA (comma separated media hosts),
// or nil when the first is unset and the feature is off.
func FromEnv() *Source {
	host := cleanHost(os.Getenv("AUDIOLOADER_SECRET_SOURCE"))
	if host == "" {
		return nil
	}
	s := New(host)
	for _, m := range strings.Split(os.Getenv("AUDIOLOADER_SECRET_SOURCE_MEDIA"), ",") {
		if m = cleanHost(m); m != "" {
			s.Media = append(s.Media, m)
		}
	}
	return s
}

// New returns a source for the page host with the default caps.
func New(host string) *Source {
	return &Source{
		Host:         cleanHost(host),
		UserAgent:    mb.UserAgent(),
		MaxPage:      2 << 20,
		MaxAudio:     300 << 20,
		PageTimeout:  20 * time.Second,
		AudioTimeout: 10 * time.Minute,
	}
}

// cleanHost lowercases a configured host and drops a scheme or path someone
// pasted along with it.
func cleanHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if _, rest, ok := strings.Cut(h, "://"); ok {
		h = rest
	}
	h, _, _ = strings.Cut(h, "/")
	return strings.TrimSuffix(h, ":443")
}

// hostOf is the host of u as the config spells it: lowercase, and without
// the default port.
func hostOf(u *url.URL) string {
	return strings.TrimSuffix(strings.ToLower(u.Host), ":443")
}

// Allowed reports whether u is on the page host or a media host, over https.
func (s *Source) Allowed(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	h := hostOf(u)
	if h == s.Host {
		return true
	}
	for _, m := range s.Media {
		if h == m {
			return true
		}
	}
	return false
}

func (s *Source) isMedia(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	h := hostOf(u)
	for _, m := range append([]string{s.Host}, s.Media...) {
		if h == m {
			return true
		}
	}
	return false
}

// Normalize checks that raw is a page of the source and returns it in the
// form used as the track's source URL: https, lowercase host, no query,
// fragment or trailing slash. The same page always gives the same string.
// As pasted from a browser it may also be http, lack a scheme, or have a
// "www." in front of the host; it's always fetched over https.
func (s *Source) Normalize(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") && !strings.Contains(raw, ":") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return "", ErrBadURL
	}
	host := strings.TrimSuffix(hostOf(u), ":80")
	if host != s.Host && host != "www."+s.Host {
		return "", ErrBadURL
	}
	p := strings.TrimRight(u.EscapedPath(), "/")
	if p == "" {
		return "", ErrBadURL
	}
	return "https://" + s.Host + p, nil
}

// Parent is the page one level above pageURL, a normalized page URL, such as
// the uploader's page of a recording; "" if there's none.
func (s *Source) Parent(pageURL string) string {
	u, err := url.Parse(pageURL)
	if err != nil || hostOf(u) != s.Host {
		return ""
	}
	dir := path.Dir(u.EscapedPath())
	if dir == "/" || dir == "." {
		return ""
	}
	return "https://" + s.Host + dir
}

// client is the base client with a redirect policy that keeps every hop on
// an allowed host over https.
func (s *Source) client() *http.Client {
	var c http.Client
	if s.Client != nil {
		c = *s.Client
	}
	c.Timeout = 0 // the contexts carry the deadlines
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if !s.Allowed(req.URL) {
			return ErrOffHost
		}
		return nil
	}
	return &c
}

func (s *Source) get(ctx context.Context, target, referer string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.UserAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		if errors.Is(err, ErrOffHost) {
			return nil, ErrOffHost
		}
		return nil, err
	}
	return resp, nil
}

// Page is what a page of the source tells about its audio.
type Page struct {
	// URL is the normalized page URL, the track's source URL.
	URL string
	// Title and Description are defaults for the user to edit; either may be empty.
	Title       string
	Description string
	// Tags are the [Tag] markers in the title, then the description, as
	// defaults for the user to edit.
	Tags []string
	// AudioURL is the audio file, on an allowed host.
	AudioURL string
}

// Listing is a page that links to other pages of the source one level below
// it, such as an uploader's page.
type Listing struct {
	// URL is the normalized listing URL.
	URL     string
	Title   string
	Entries []Entry
}

// Entry is a page a listing links to.
type Entry struct {
	URL   string // normalized
	Title string // the link's text; may be empty
}

// Inspect fetches the page at raw, which must be a page of the source, and
// finds its audio file, title and description.
func (s *Source) Inspect(ctx context.Context, raw string) (*Page, error) {
	page, _, err := s.Look(ctx, raw)
	if err == nil && page == nil {
		err = ErrNoAudio
	}
	return page, err
}

// Look fetches the page at raw once. A page with an audio file gives a Page;
// one without gives a Listing of the pages it links to one level below it,
// or ErrNoAudio when it links to none.
func (s *Source) Look(ctx context.Context, raw string) (*Page, *Listing, error) {
	pageURL, err := s.Normalize(raw)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.PageTimeout)
	defer cancel()
	resp, err := s.get(ctx, pageURL, "")
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("the page answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, s.MaxPage+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(body)) > s.MaxPage {
		return nil, nil, ErrTooLarge
	}
	base, _ := url.Parse(pageURL)
	found := parsePage(body, base)

	page := &Page{URL: pageURL, Title: found.title, Description: found.description, Tags: BracketTags(found.title, found.description)}
	switch {
	case found.tagged != "":
		// a file the page names itself; where it lives is checked, not guessed around
		u, err := base.Parse(found.tagged)
		if err != nil || !s.isMedia(u) {
			return nil, nil, ErrAudioHost
		}
		page.AudioURL = u.String()
	default:
		for _, c := range found.texts {
			if u, err := url.Parse(c); err == nil && s.isMedia(u) && audioExt(u.Path) {
				page.AudioURL = u.String()
				break
			}
		}
	}
	if page.AudioURL != "" {
		return page, nil, nil
	}

	l := &Listing{URL: pageURL, Title: found.title}
	seen := map[string]int{} // URL to its index in Entries
	for _, ln := range found.links {
		u, err := base.Parse(ln.href)
		if err != nil {
			continue
		}
		entryURL, err := s.Normalize(u.String())
		if err != nil || s.Parent(entryURL) != pageURL {
			continue
		}
		if i, ok := seen[entryURL]; ok {
			if l.Entries[i].Title == "" {
				l.Entries[i].Title = ln.text
			}
			continue
		}
		seen[entryURL] = len(l.Entries)
		l.Entries = append(l.Entries, Entry{URL: entryURL, Title: ln.text})
	}
	if len(l.Entries) == 0 {
		return nil, nil, ErrNoAudio
	}
	return nil, l, nil
}

// audioExts are the file types the source's files may have.
var audioExts = map[string]bool{".m4a": true, ".mp3": true, ".ogg": true, ".opus": true}

func audioExt(p string) bool { return audioExts[strings.ToLower(path.Ext(p))] }

// File is a downloaded audio file in the directory given to Fetch.
type File struct {
	Path     string
	Ext      string // with the dot, e.g. ".m4a"
	Duration time.Duration
}

// Fetch downloads the page's audio file into dir, sending the page URL as
// Referer, and checks that it is audio of a readable length. A failed
// download leaves nothing behind.
func (s *Source) Fetch(ctx context.Context, page *Page, dir string) (*File, error) {
	u, err := url.Parse(page.AudioURL)
	if err != nil || !s.isMedia(u) || !audioExt(u.Path) {
		return nil, ErrAudioHost
	}
	ctx, cancel := context.WithTimeout(ctx, s.AudioTimeout)
	defer cancel()
	resp, err := s.get(ctx, page.AudioURL, page.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the audio file answered %d", resp.StatusCode)
	}
	if resp.ContentLength > s.MaxAudio {
		return nil, ErrTooLarge
	}

	ext := strings.ToLower(path.Ext(u.Path))
	f, err := os.CreateTemp(dir, "secret-*"+ext)
	if err != nil {
		return nil, err
	}
	name := f.Name()
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	n, err := io.Copy(f, io.LimitReader(resp.Body, s.MaxAudio+1))
	if err != nil {
		return nil, err
	}
	if n > s.MaxAudio {
		return nil, ErrTooLarge
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	dur, err := audio.ProbeDuration(f)
	if err != nil {
		return nil, fmt.Errorf("that isn't audio we can read: %w", err)
	}
	ok = true
	return &File{Path: filepath.Clean(name), Ext: ext, Duration: dur.Round(time.Second)}, nil
}

var (
	defaultMu     sync.Mutex
	defaultSource *Source
	defaultLoaded bool
)

// Default is the process-wide source, from the environment on first use; nil
// when the feature is off.
func Default() *Source {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if !defaultLoaded {
		defaultSource, defaultLoaded = FromEnv(), true
	}
	return defaultSource
}

// SetDefault replaces the process-wide source (nil turns the feature off);
// for tests.
func SetDefault(s *Source) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultSource, defaultLoaded = s, true
}
