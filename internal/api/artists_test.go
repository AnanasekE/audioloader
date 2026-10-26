package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
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

func TestParseArtistMBID(t *testing.T) {
	const id = "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"
	for in, want := range map[string]string{
		id:                                     id,
		" " + strings.ToUpper(id) + " ":        id,
		"https://musicbrainz.org/artist/" + id: id,
		"musicbrainz.org/artist/" + id + "/releases":        id,
		"https://beta.musicbrainz.org/artist/" + id:         id,
		"https://musicbrainz.org/release/" + id:             "",
		"https://evil.example/artist/" + id:                 "",
		"https://musicbrainz.org.evil.example/artist/" + id: "",
		"https://musicbrainz.org/artist/not-an-id":          "",
		"hello": "",
		"":      "",
	} {
		if got := parseArtistMBID(in); got != want {
			t.Errorf("parseArtistMBID(%q) = %q, want %q", in, got, want)
		}
	}
}

func postForm(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLinkArtist(t *testing.T) {
	dbtest.New(t)
	fakeMusicBrainz(t, map[string]string{"/artist": `{"artists": [
		{"id": "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", "name": "Massive Attack", "type": "Group", "country": "GB",
		 "area": {"name": "United Kingdom"}, "life-span": {"begin": "1987", "ended": null}}]}`})
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)

	a, _ := db.AddArtist(db.All, "Massive Attack")
	b, _ := db.AddArtist(db.All, "Massive Attak")
	const mbid = "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/ui/artists/"+a.ID.String()+"/link", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "United Kingdom") || !strings.Contains(rec.Body.String(), "1987–") {
		t.Fatalf("candidates = %d %s", rec.Code, rec.Body.String())
	}

	if rec := postForm(h, "/ui/artists/"+a.ID.String()+"/link", "mbid_manual=https://musicbrainz.org/release/"+mbid); rec.Code != http.StatusBadRequest {
		t.Fatalf("release URL = %d", rec.Code)
	}
	if rec := postForm(h, "/ui/artists/"+a.ID.String()+"/link", "mbid="+mbid); rec.Code != http.StatusNoContent {
		t.Fatalf("link = %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := db.GetArtist(db.All, a.ID.String()); got.MBID != mbid {
		t.Fatalf("mbid = %q", got.MBID)
	}
	// linked artists can't be linked again
	if rec := postForm(h, "/ui/artists/"+a.ID.String()+"/link", "mbid="+mbid); rec.Code != http.StatusConflict {
		t.Fatalf("relink = %d", rec.Code)
	}

	// the same ID on another artist offers a merge and sets nothing
	rec = postForm(h, "/ui/artists/"+b.ID.String()+"/link", "mbid_manual=musicbrainz.org/artist/"+mbid)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Merge into Massive Attack") {
		t.Fatalf("taken = %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := db.GetArtist(db.All, b.ID.String()); got.MBID != "" {
		t.Fatalf("duplicate mbid set: %q", got.MBID)
	}
}
