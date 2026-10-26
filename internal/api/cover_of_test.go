package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
)

func TestCoverOfForm(t *testing.T) {
	dbtest.New(t)
	fixture, err := os.ReadFile("../mb/testdata/recording.json")
	if err != nil {
		t.Fatal(err)
	}
	fakeMusicBrainz(t, map[string]string{"/recording/f3bba4cd-8018-468b-902e-bc8f029593e5": string(fixture)})
	ids := secretLibrary(t)
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)

	id, artist := ids["public track"], ids["public artist"]
	save := func(form url.Values) *httptest.ResponseRecorder {
		form.Set("title", "Open Song")
		form.Set("kind", "music")
		form.Set("artist_id", artist)
		req := httptest.NewRequest("POST", "/ui/tracks/"+id, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	coverOf := func() string {
		tr, err := db.GetTrack(db.Normal, id)
		if err != nil {
			t.Fatal(err)
		}
		if tr.CoverOf == nil {
			return ""
		}
		return tr.CoverOf.String()
	}

	// the three modes
	if rec := save(url.Values{"cover_mode": {"text"}, "cover_title": {"Some Song"}, "cover_artists": {"X & Y"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("text = %d %s", rec.Code, rec.Body)
	}
	if got := coverOf(); got != "Some Song by X & Y" {
		t.Fatalf("text cover = %q", got)
	}
	if rec := save(url.Values{"cover_mode": {"mbid"}, "cover_mb_url": {"https://musicbrainz.org/recording/f3bba4cd-8018-468b-902e-bc8f029593e5"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("mbid = %d %s", rec.Code, rec.Body)
	}
	if got := coverOf(); got != "Teardrop by Massive Attack" {
		t.Fatalf("mbid cover = %q", got)
	}
	tr, _ := db.GetTrack(db.Normal, id)
	if tr.CoverOf.MBID != "f3bba4cd-8018-468b-902e-bc8f029593e5" {
		t.Fatalf("mbid = %q", tr.CoverOf.MBID)
	}
	// leaving the mode alone keeps it
	save(url.Values{})
	if coverOf() == "" {
		t.Fatal("a save without cover_mode removed the cover")
	}
	closing := ids["cover of secret"]
	if rec := save(url.Values{"cover_mode": {"library"}, "cover_track_id": {closing}}); rec.Code != http.StatusNoContent {
		t.Fatalf("library = %d %s", rec.Code, rec.Body)
	}
	if got := coverOf(); got != "Public Cover by Public Artist" {
		t.Fatalf("library cover = %q", got)
	}
	// bad input
	for name, form := range map[string]url.Values{
		"itself":      {"cover_mode": {"library"}, "cover_track_id": {id}},
		"secret":      {"cover_mode": {"library"}, "cover_track_id": {ids["secret track"]}},
		"not a track": {"cover_mode": {"library"}, "cover_track_id": {"x"}},
		"no title":    {"cover_mode": {"text"}},
		"bad url":     {"cover_mode": {"mbid"}, "cover_mb_url": {"https://example.com/recording/x"}},
		"unknown":     {"cover_mode": {"remix"}},
	} {
		if rec := save(form); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d", name, rec.Code)
		}
	}
	if got := coverOf(); got != "Public Cover by Public Artist" {
		t.Fatalf("failed saves changed the cover: %q", got)
	}
	// MusicBrainz doesn't know the recording
	if rec := save(url.Values{"cover_mode": {"mbid"}, "cover_mb_url": {"00000000-0000-0000-0000-000000000000"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown recording = %d", rec.Code)
	}
	if rec := save(url.Values{"cover_mode": {"none"}}); rec.Code != http.StatusNoContent || coverOf() != "" {
		t.Fatalf("remove = %d, %q", rec.Code, coverOf())
	}

	// the picker leaves out the track itself and what a locked viewer can't see
	get := func(path string) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Body.String()
	}
	if body := get("/ui/tracks/pick?q=song&exclude=" + id); strings.Contains(body, "Open Song") || !strings.Contains(body, "Closing Song") && !strings.Contains(body, "Public Cover") {
		t.Errorf("picker = %s", body)
	}
	if body := get("/ui/tracks/pick?q=open"); !strings.Contains(body, "Open Song") {
		t.Errorf("picker = %s", body)
	}
	if body := get("/ui/tracks/pick?q=zzsecret"); strings.Contains(strings.ToLower(body), "zzsecret") {
		t.Errorf("picker leaks: %s", body)
	}
}

func TestCoverSuggestionRoutes(t *testing.T) {
	dbtest.New(t)
	ids := secretLibrary(t)
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)
	post := func(path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", path, nil))
		return rec.Code
	}
	// a locked viewer can neither apply nor reject a hidden track's suggestion
	for _, action := range []string{"apply", "reject"} {
		if code := post("/ui/tagging/" + ids["secret track"] + "/cover-of/" + action); code != http.StatusNotFound {
			t.Errorf("%s hidden = %d", action, code)
		}
	}
	id := ids["public track"]
	db.SaveCoverSuggestion(id, db.CoverSuggestion{Title: "Orig", Artists: "Band"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/tagging", nil))
	if !strings.Contains(rec.Body.String(), "Orig") || strings.Contains(rec.Body.String(), "ZZSECRET") {
		t.Fatalf("tagging page = %s", rec.Body)
	}
	if code := post("/ui/tagging/" + id + "/cover-of/apply"); code != http.StatusNoContent {
		t.Fatalf("apply = %d", code)
	}
	if tr, _ := db.GetTrack(db.Normal, id); tr.CoverOf == nil || tr.CoverOf.Title != "Orig" {
		t.Fatalf("cover of = %+v", tr.CoverOf)
	}
	if code := post("/ui/tagging/" + id + "/cover-of/apply"); code != http.StatusNotFound {
		t.Fatalf("second apply = %d", code)
	}
}
