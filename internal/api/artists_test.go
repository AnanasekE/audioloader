package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCandidateReleases(t *testing.T) {
	fakeMusicBrainz(t, map[string]string{"/release-group": `{"release-groups": [
		{"id": "a", "title": "Old Album", "primary-type": "Album", "first-release-date": "1991"},
		{"id": "b", "title": "New Single", "primary-type": "Single", "first-release-date": "2020-05-01"}], "release-group-count": 2}`})
	mux := http.NewServeMux()
	Register(mux, nil)

	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		WithViewer(mux).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code, rec.Body.String()
	}
	code, body := get("/ui/musicbrainz/artists/10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8/releases")
	if code != 200 || strings.Index(body, "New Single") > strings.Index(body, "Old Album") || !strings.Contains(body, "Single, 2020-05-01") {
		t.Fatalf("%d %s", code, body)
	}
	if code, _ := get("/ui/musicbrainz/artists/not-a-uuid/releases"); code != http.StatusBadRequest {
		t.Fatalf("bad id = %d", code)
	}
	// an unreachable MusicBrainz shows a message, not an error page
	fakeMusicBrainz(t, nil)
	if code, body := get("/ui/musicbrainz/artists/10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8/releases"); code != 200 || !strings.Contains(body, "Couldn't load") {
		t.Fatalf("%d %s", code, body)
	}
}
