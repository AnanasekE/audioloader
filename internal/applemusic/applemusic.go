// Package applemusic reads public Apple Music playlist and song pages. There
// is no Apple account or API token: the pages embed their data as JSON in a
// script tag, and that is all this package looks at. Every request goes
// through one rate limiter (1 request per second).
package applemusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrUnreadable is returned when a page has none of the data we look for.
	ErrUnreadable = errors.New("Apple Music changed its page; this link can't be read.")
	// ErrNotFound is returned for a 404 from Apple Music.
	ErrNotFound = errors.New("not found")
)

// maxPage caps how much of a page is read; a 244-song playlist is 1.2 MB.
const maxPage = 16 << 20

// Song is one song as a page lists it.
type Song struct {
	ID       string // Apple's song ID
	Title    string
	Artist   string // as credited, e.g. "Drake & Don Toliver"
	Album    string
	Duration time.Duration // 0 when the page doesn't say
}

// Playlist is a public playlist page.
type Playlist struct {
	Name       string
	TrackCount int    // what the page says the playlist has
	Songs      []Song // what the page lists; fewer than TrackCount for a long one
}

// Client is safe for concurrent use. BaseURL is a field so tests can point
// it at an httptest server; when set, pages are fetched from there instead
// of from the host in the link.
type Client struct {
	HTTP    *http.Client
	BaseURL string

	// Interval is the wait between requests.
	Interval time.Duration
	// Sleep waits for the limiter; tests replace it.
	Sleep func(ctx context.Context, d time.Duration) error

	mu   sync.Mutex
	next time.Time
}

// New returns a client for music.apple.com.
func New() *Client {
	return &Client{
		HTTP:     &http.Client{Timeout: 20 * time.Second},
		Interval: time.Second,
		Sleep:    sleep,
	}
}

var (
	defaultOnce   sync.Once
	defaultClient *Client
)

// Default is the process-wide client, so every caller shares its limiter.
func Default() *Client {
	defaultOnce.Do(func() { defaultClient = New() })
	return defaultClient
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

var (
	playlistPath = regexp.MustCompile(`^/([a-z]{2})/playlist/(?:[^/]+/)?(pl\.[A-Za-z0-9_-]+)/?$`)
	songPath     = regexp.MustCompile(`^/([a-z]{2})/song/(?:[^/]+/)?(\d+)/?$`)
	albumPath    = regexp.MustCompile(`^/([a-z]{2})/album/(?:[^/]+/)?(\d+)/?$`)
	digits       = regexp.MustCompile(`^\d+$`)
)

// ParseLink reads an Apple Music link: a playlist, a song, or an album
// (kind "playlist", "song" or "album"). An album link with ?i=<songID> is
// that song. id is Apple's ID, storefront the two-letter country in the
// path. ok is false for another site or another kind of page.
func ParseLink(s string) (kind, id, storefront string, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", "", false
	}
	if h := strings.ToLower(u.Hostname()); h != "music.apple.com" && h != "www.music.apple.com" {
		return "", "", "", false
	}
	if m := playlistPath.FindStringSubmatch(u.Path); m != nil {
		return "playlist", m[2], m[1], true
	}
	if m := songPath.FindStringSubmatch(u.Path); m != nil {
		return "song", m[2], m[1], true
	}
	if m := albumPath.FindStringSubmatch(u.Path); m != nil {
		if song := u.Query().Get("i"); digits.MatchString(song) {
			return "song", song, m[1], true
		}
		return "album", m[2], m[1], true
	}
	return "", "", "", false
}

// GetPlaylist reads a public playlist page.
func (c *Client) GetPlaylist(ctx context.Context, link string) (Playlist, error) {
	kind, _, _, ok := ParseLink(link)
	if !ok || kind != "playlist" {
		return Playlist{}, ErrUnreadable
	}
	sections, _, err := c.page(ctx, link)
	if err != nil {
		return Playlist{}, err
	}
	var pl Playlist
	for _, sec := range sections {
		switch sec.ItemKind {
		case "containerDetailHeaderLockup":
			if len(sec.Items) > 0 && pl.Name == "" {
				var h struct {
					Title      string `json:"title"`
					TrackCount int    `json:"trackCount"`
				}
				if json.Unmarshal(sec.Items[0], &h) == nil {
					pl.Name, pl.TrackCount = strings.TrimSpace(h.Title), h.TrackCount
				}
			}
		case "trackLockup":
			for _, raw := range sec.Items {
				if s, ok := trackSong(raw); ok {
					pl.Songs = append(pl.Songs, s)
				}
			}
		}
	}
	if len(pl.Songs) == 0 {
		return Playlist{}, ErrUnreadable
	}
	if pl.Name == "" {
		pl.Name = "Apple Music playlist"
	}
	return pl, nil
}

// GetSong reads a song page: /song/<slug>/<id>, or /album/<slug>/<id>?i=<songID>.
func (c *Client) GetSong(ctx context.Context, link string) (Song, error) {
	kind, id, _, ok := ParseLink(link)
	if !ok || kind != "song" {
		return Song{}, ErrUnreadable
	}
	sections, body, err := c.page(ctx, link)
	if err != nil {
		return Song{}, err
	}
	album := ""
	for _, sec := range sections {
		switch sec.ItemKind {
		case "songDetailHeader":
			// a song page: the header is the song
			if len(sec.Items) == 0 {
				continue
			}
			var h struct {
				Title   string `json:"title"`
				Artists string `json:"artists"`
				Album   string `json:"album"`
				Content struct {
					Identifiers struct {
						ID string `json:"storeAdamID"`
					} `json:"identifiers"`
				} `json:"contentDescriptor"`
			}
			if json.Unmarshal(sec.Items[0], &h) != nil || h.Title == "" {
				continue
			}
			return Song{
				ID:       cmpFirst(h.Content.Identifiers.ID, id),
				Title:    strings.TrimSpace(h.Title),
				Artist:   strings.TrimSpace(h.Artists),
				Album:    strings.TrimSpace(h.Album),
				Duration: pageDuration(body),
			}, nil
		case "containerDetailHeaderLockup":
			// an album page: the song is one of its tracks
			if len(sec.Items) > 0 {
				var h struct {
					Title string `json:"title"`
				}
				json.Unmarshal(sec.Items[0], &h)
				album = strings.TrimSpace(h.Title)
			}
		case "trackLockup":
			for _, raw := range sec.Items {
				if s, ok := trackSong(raw); ok && s.ID == id {
					if s.Album == "" {
						s.Album = album
					}
					return s, nil
				}
			}
		}
	}
	return Song{}, ErrUnreadable
}

func cmpFirst(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// trackSong reads one "trackLockup" item.
func trackSong(raw json.RawMessage) (Song, bool) {
	var t struct {
		Title        string `json:"title"`
		ArtistName   string `json:"artistName"`
		Duration     int    `json:"duration"` // milliseconds
		TertiaryLink []struct {
			Title string `json:"title"`
		} `json:"tertiaryLinks"`
		Content struct {
			Identifiers struct {
				ID string `json:"storeAdamID"`
			} `json:"identifiers"`
		} `json:"contentDescriptor"`
	}
	if json.Unmarshal(raw, &t) != nil || strings.TrimSpace(t.Title) == "" {
		return Song{}, false
	}
	s := Song{
		ID:       t.Content.Identifiers.ID,
		Title:    strings.TrimSpace(t.Title),
		Artist:   strings.TrimSpace(t.ArtistName),
		Duration: time.Duration(t.Duration) * time.Millisecond,
	}
	if len(t.TertiaryLink) > 0 {
		s.Album = strings.TrimSpace(t.TertiaryLink[0].Title)
	}
	return s, true
}

var (
	dataScript = regexp.MustCompile(`(?s)<script type="application/json" id="serialized-server-data">(.*?)</script>`)
	// the song schema's "duration":"PT3M38S"
	songSchema = regexp.MustCompile(`(?s)<script id=schema:song type="application/ld\+json">(.*?)</script>`)
	isoTime    = regexp.MustCompile(`"duration":"PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?"`)
)

type section struct {
	ItemKind string            `json:"itemKind"`
	Items    []json.RawMessage `json:"items"`
}

// page fetches link and returns the sections of its embedded data, and the
// page itself.
func (c *Client) page(ctx context.Context, link string) ([]section, string, error) {
	body, err := c.fetch(ctx, link)
	if err != nil {
		return nil, "", err
	}
	m := dataScript.FindStringSubmatch(body)
	if m == nil {
		return nil, body, ErrUnreadable
	}
	var data struct {
		Data []struct {
			Data struct {
				Sections []section `json:"sections"`
			} `json:"data"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(m[1]), &data) != nil {
		return nil, body, ErrUnreadable
	}
	var all []section
	for _, d := range data.Data {
		all = append(all, d.Data.Sections...)
	}
	return all, body, nil
}

// pageDuration reads the length of a song page from its schema.org data.
func pageDuration(body string) time.Duration {
	m := songSchema.FindStringSubmatch(body)
	if m == nil {
		return 0
	}
	t := isoTime.FindStringSubmatch(m[1])
	if t == nil {
		return 0
	}
	n := func(s string) time.Duration {
		v, _ := strconv.Atoi(s)
		return time.Duration(v)
	}
	return n(t[1])*time.Hour + n(t[2])*time.Minute + n(t[3])*time.Second
}

// wait spaces requests at least Interval apart.
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d := time.Until(c.next); d > 0 {
		if err := c.Sleep(ctx, d); err != nil {
			return err
		}
	}
	c.next = time.Now().Add(c.Interval)
	return nil
}

// fetch gets the page behind link, from BaseURL when that is set.
func (c *Client) fetch(ctx context.Context, link string) (string, error) {
	if !strings.Contains(link, "://") {
		link = "https://" + link
	}
	u, err := url.Parse(link)
	if err != nil {
		return "", err
	}
	if c.BaseURL != "" {
		base, err := url.Parse(c.BaseURL)
		if err != nil {
			return "", err
		}
		u.Scheme, u.Host = base.Scheme, base.Host
	}
	if err := c.wait(ctx); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	// without a browser's User-Agent the page comes back without its data
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0")
	req.Header.Set("Accept", "text/html")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("apple music: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPage))
	return string(b), err
}
