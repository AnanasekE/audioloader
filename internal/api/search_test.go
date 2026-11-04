package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"audioloader/internal/dbtest"
)

func TestSearchPageType(t *testing.T) {
	dbtest.New(t)
	modeLibrary(t)
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)
	get := func(path string) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d", path, rec.Code)
		}
		return rec.Body.String()
	}

	all := get("/search?q=zzpublic")
	if !strings.Contains(all, "ZZPUBLIC Song 1") || !strings.Contains(all, "ZZPUBLIC Album") {
		t.Fatal("all: want tracks and releases")
	}
	rel := get("/search?q=zzpublic&type=releases")
	if !strings.Contains(rel, "ZZPUBLIC Album") || strings.Contains(rel, "ZZPUBLIC Song 1") {
		t.Fatal("type=releases: want only releases")
	}
	// (track rows name their release, so look for what only artists have)
	ar := get("/search?q=zzpublic&type=artists")
	if !strings.Contains(ar, "ZZPUBLIC Artist") || strings.Contains(ar, "ZZPUBLIC Song 1") {
		t.Fatal("type=artists: want only artists")
	}
	// an unknown type is all
	if body := get("/search?q=zzpublic&type=nope"); !strings.Contains(body, "ZZPUBLIC Album") || !strings.Contains(body, "ZZPUBLIC Song 1") {
		t.Fatal("unknown type: want all")
	}
}
