// Package mb talks to MusicBrainz, AcoustID and the Cover Art Archive.
// Every request goes through a per-host rate limiter, shared by all
// callers of a Client, so background jobs stay within the services' limits
// (MusicBrainz allows 1 request per second).
package mb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned for a 404 from any of the services.
var ErrNotFound = errors.New("not found")

// Client is safe for concurrent use. Base URLs are fields so tests can
// point them at an httptest server.
type Client struct {
	HTTP        *http.Client
	UserAgent   string
	AcoustIDKey string

	MusicBrainzURL string
	AcoustIDURL    string
	CoverArtURL    string

	// Sleep waits between retries; tests replace it.
	Sleep func(ctx context.Context, d time.Duration) error

	mu       sync.Mutex
	limiters map[string]*limiter
}

// Intervals between requests per host.
var intervals = map[string]time.Duration{
	"musicbrainz": time.Second,
	"acoustid":    time.Second / 3,
	"coverart":    time.Second,
}

// New returns a client configured from AUDIOLOADER_CONTACT (put into the
// User-Agent, which MusicBrainz requires) and ACOUSTID_KEY.
func New() *Client {
	contact := os.Getenv("AUDIOLOADER_CONTACT")
	if contact == "" {
		contact = "https://github.com/AnanasekE/audioloader"
	}
	return &Client{
		HTTP:           &http.Client{Timeout: 10 * time.Second},
		UserAgent:      "audioloader/0.1 ( " + contact + " )",
		AcoustIDKey:    os.Getenv("ACOUSTID_KEY"),
		MusicBrainzURL: "https://musicbrainz.org/ws/2",
		AcoustIDURL:    "https://api.acoustid.org/v2",
		CoverArtURL:    "https://coverartarchive.org",
		Sleep:          sleep,
	}
}

var (
	defaultOnce   sync.Once
	defaultClient *Client
)

// Default is the process-wide client, so all jobs share its limiters.
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

// limiter spaces requests at least interval apart.
type limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

func (l *limiter) wait(ctx context.Context, sleepFn func(context.Context, time.Duration) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if d := time.Until(l.next); d > 0 {
		if err := sleepFn(ctx, d); err != nil {
			return err
		}
	}
	l.next = time.Now().Add(l.interval)
	return nil
}

func (c *Client) limiter(host string) *limiter {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limiters == nil {
		c.limiters = map[string]*limiter{}
	}
	l := c.limiters[host]
	if l == nil {
		l = &limiter{interval: intervals[host]}
		c.limiters[host] = l
	}
	return l
}

// retries503 is how often a 503 (MusicBrainz's "busy, slow down") is
// retried, waiting 2, 4, then 8 seconds.
const retries503 = 3

// get fetches rawURL for the given host's limiter, retrying on 503. The
// caller closes the body.
func (c *Client) get(ctx context.Context, host, rawURL string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		if err := c.limiter(host).wait(ctx, c.Sleep); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return resp, nil
		case resp.StatusCode == http.StatusNotFound:
			resp.Body.Close()
			return nil, ErrNotFound
		case resp.StatusCode == http.StatusServiceUnavailable && attempt < retries503:
			resp.Body.Close()
			log.Printf("%s busy (503), retrying: %s", host, rawURL)
			if err := c.Sleep(ctx, 2*time.Second<<attempt); err != nil {
				return nil, err
			}
			continue
		default:
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			return nil, fmt.Errorf("%s: %s: %s", host, resp.Status, strings.TrimSpace(string(body)))
		}
	}
}

func (c *Client) getJSON(ctx context.Context, host, rawURL string, v any) error {
	resp, err := c.get(ctx, host, rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// mbURL builds a MusicBrainz ws/2 URL for path with the given query, in JSON.
func (c *Client) mbURL(path string, q url.Values) string {
	if q == nil {
		q = url.Values{}
	}
	q.Set("fmt", "json")
	return c.MusicBrainzURL + "/" + path + "?" + q.Encode()
}

// Phrase quotes s as a Lucene phrase for MusicBrainz search queries.
func Phrase(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
