package mb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fixtureServer serves testdata files by the request path's last element
// and records the requests it got.
func fixtureServer(t *testing.T, routes map[string]string) (*Client, *[]*http.Request) {
	t.Helper()
	var reqs []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		// the longest matching prefix wins, so "/recording/" beats "/recording"
		best := ""
		for prefix := range routes {
			if strings.HasPrefix(r.URL.Path, prefix) && len(prefix) > len(best) {
				best = prefix
			}
		}
		if best == "" {
			http.NotFound(w, r)
			return
		}
		data, err := os.ReadFile(filepath.Join("testdata", routes[best]))
		if err != nil {
			t.Error(err)
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	c := New()
	c.MusicBrainzURL, c.AcoustIDURL, c.CoverArtURL = srv.URL+"/ws/2", srv.URL+"/acoustid", srv.URL+"/caa"
	c.Sleep = func(ctx context.Context, d time.Duration) error { return nil }
	return c, &reqs
}

func TestDecodeFixtures(t *testing.T) {
	c, reqs := fixtureServer(t, map[string]string{
		"/ws/2/recording/":    "recording.json",
		"/ws/2/release/":      "release.json",
		"/ws/2/recording":     "recording_search.json",
		"/ws/2/artist":        "artist_search.json",
		"/ws/2/release-group": "release_groups.json",
		"/ws/2/release":       "releases_browse.json",
		"/acoustid/lookup":    "acoustid_lookup.json",
	})
	ctx := context.Background()

	rec, err := c.GetRecording(ctx, "f3bba4cd-8018-468b-902e-bc8f029593e5")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Title != "Teardrop" || len(rec.ISRCs) == 0 || len(rec.Releases) == 0 || rec.ArtistCredit.String() != "Massive Attack" {
		t.Fatalf("recording = %+v", rec)
	}
	if got := (*reqs)[0].URL.Query().Get("inc"); got != "artists releases isrcs release-groups" {
		t.Fatalf("inc = %q", got)
	}
	if ua := (*reqs)[0].Header.Get("User-Agent"); !strings.HasPrefix(ua, "audioloader/") {
		t.Fatalf("User-Agent = %q", ua)
	}

	rel, err := c.GetRelease(ctx, "0d33ef7a-1f5d-4365-b807-b412271b99c3")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Title != "Mezzanine" || rel.ReleaseGroup.PrimaryType != "Album" || len(rel.Media) == 0 || rel.Media[0].Tracks[2].Title != "Teardrop" {
		t.Fatalf("release = %+v", rel.Title)
	}

	recs, err := c.SearchRecordings(ctx, "recording:"+Phrase("Teardrop"), 2)
	if err != nil || len(recs) == 0 || recs[0].Score == 0 {
		t.Fatalf("search = %v, %v", recs, err)
	}
	artists, err := c.SearchArtists(ctx, "Massive Attack", 3)
	if err != nil || len(artists) == 0 || artists[0].Name != "Massive Attack" {
		t.Fatalf("artists = %v, %v", artists, err)
	}
	groups, count, err := c.BrowseReleaseGroups(ctx, "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", 0)
	if err != nil || len(groups) == 0 || count < len(groups) || groups[0].PrimaryType == "" {
		t.Fatalf("groups = %v, %d, %v", groups, count, err)
	}
	releases, _, err := c.BrowseReleases(ctx, "6f9f6899-c0d3-311d-ae87-a10ae6bc53a9", 0)
	if err != nil || len(releases) == 0 || len(releases[0].Media) == 0 {
		t.Fatalf("releases = %v, %v", len(releases), err)
	}
	results, err := c.AcoustIDLookup(ctx, "AQAA", 330)
	if err != nil || len(results) != 2 || results[0].Recordings[0].Title != "Teardrop" {
		t.Fatalf("acoustid = %+v, %v", results, err)
	}
	if _, _, err := c.FrontCover(ctx, "release", "x"); err != ErrNotFound {
		t.Fatalf("missing cover err = %v", err)
	}
}

func TestLimiterSpacesRequests(t *testing.T) {
	var waited time.Duration
	l := &limiter{interval: time.Second}
	fake := func(ctx context.Context, d time.Duration) error {
		waited += d
		return nil
	}
	l.wait(context.Background(), fake)
	l.wait(context.Background(), fake)
	// the second call has to wait for (almost) the full interval
	if waited < 900*time.Millisecond {
		t.Fatalf("waited %v between two requests", waited)
	}
}

func TestRetryOn503(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"id": "x", "title": "ok"}`))
	}))
	defer srv.Close()
	c := New()
	c.MusicBrainzURL = srv.URL
	c.Sleep = func(ctx context.Context, d time.Duration) error { return nil }
	rec, err := c.GetRecording(context.Background(), "x")
	if err != nil || rec.Title != "ok" || calls.Load() != 2 {
		t.Fatalf("rec = %v, err = %v, calls = %d", rec, err, calls.Load())
	}
}

func TestPhrase(t *testing.T) {
	if got := Phrase(`say "hi" \o/`); got != `"say \"hi\" \\o/"` {
		t.Fatalf("Phrase = %s", got)
	}
}
