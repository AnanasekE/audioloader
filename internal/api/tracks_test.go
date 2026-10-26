package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"audioloader/internal/dbtest"
)

func TestTrackEditHiddenWhileLocked(t *testing.T) {
	dbtest.New(t)
	ids := secretLibrary(t)
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
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/tracks/"+ids["public track"]+"/edit", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Open Song") {
		t.Fatalf("public edit page = %d", rec.Code)
	}
}
