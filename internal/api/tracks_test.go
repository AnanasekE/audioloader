package api

import (
	"bytes"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"audioloader/internal/dbtest"
)

func TestHiddenWhileLocked(t *testing.T) {
	dbtest.New(t)
	ids := modeLibrary(t)
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/tracks/"+ids["secret track"]+"/edit", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("edit page = %d", rec.Code)
	}

	req := httptest.NewRequest("POST", "/ui/tracks/"+ids["secret track"], strings.NewReader("title=Pwned&kind=music&artist_id="+ids["public artist"]))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("save = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/artists/"+ids["secret artist"], nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("hidden artist page = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/artists/"+ids["public artist"], nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ZZPUBLIC Album") {
		t.Fatalf("artist page = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/tracks/"+ids["public track"]+"/edit", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ZZPUBLIC Song 1") {
		t.Fatalf("public edit page = %d", rec.Code)
	}
}

func TestTracksPageSort(t *testing.T) {
	dbtest.New(t)
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)
	for _, s := range []struct{ title, artist, body string }{
		{"Charlie", "Xan", "c"}, {"Alpha", "Zed", "a"}, {"Bravo", "Yan", "b"},
	} {
		a, err := db.AddArtist(db.Normal, s.artist)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.AddTrack(db.Normal, []string{a.ID.String()}, &audio.Track{Title: s.title, Duration: time.Minute},
			bytes.NewReader([]byte(s.body)), ".mp3", nil, ""); err != nil {
			t.Fatal(err)
		}
	}
	for sort, want := range map[string][]string{
		"":        {"Charlie", "Alpha", "Bravo"},
		"added":   {"Charlie", "Alpha", "Bravo"},
		"title":   {"Alpha", "Bravo", "Charlie"},
		"creator": {"Charlie", "Bravo", "Alpha"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/tracks?sort="+sort, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("sort %q = %d", sort, rec.Code)
		}
		body := rec.Body.String()
		last := -1
		for _, title := range want {
			i := strings.Index(body, ">"+title+"<")
			if i < 0 || i < last {
				t.Errorf("sort %q: %s out of order", sort, title)
			}
			last = i
		}
	}
}
