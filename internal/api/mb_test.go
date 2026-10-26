package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"audioloader/internal/mb"
)

// fakeMusicBrainz points the shared MusicBrainz client at a server that
// answers with the given JSON by path prefix (404 otherwise) for the test.
func fakeMusicBrainz(t *testing.T, routes map[string]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for prefix, body := range routes {
			if strings.HasPrefix(r.URL.Path, "/ws/2"+prefix) {
				w.Write([]byte(body))
				return
			}
		}
		http.NotFound(w, r)
	}))
	c := mb.Default()
	oldURL, oldSleep := c.MusicBrainzURL, c.Sleep
	c.MusicBrainzURL = srv.URL + "/ws/2"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() {
		c.MusicBrainzURL, c.Sleep = oldURL, oldSleep
		srv.Close()
	})
}
