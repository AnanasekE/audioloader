package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
	"uuid"

	"audioloader/internal/audio"
	"audioloader/internal/db"
)

func TestArtistImageFallbacks(t *testing.T) {
	a := newModeApp(t)
	const cover = "\x89PNG\r\n\x1a\nCOVERBYTES"
	const own = "\x89PNG\r\n\x1a\nOWNPICTURE"
	pngHead := "\x89PNG"

	get := func(secret bool, artist string, header ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/artist-images/"+artist, nil)
		for i := 0; i+1 < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		if secret {
			req.AddCookie(a.v.Cookie(time.Now(), false))
		}
		rec := httptest.NewRecorder()
		a.h.ServeHTTP(rec, req)
		return rec
	}
	isTile := func(rec *httptest.ResponseRecorder) bool {
		b := rec.Body.String()
		return rec.Code == 200 && strings.HasPrefix(b, pngHead) && !strings.Contains(b, "COVERBYTES") && !strings.Contains(b, "OWNPICTURE")
	}

	shared := a.ID["shared artist"]
	// no releases yet: a tile, in both modes
	rec := get(false, shared)
	if !isTile(rec) || rec.Header().Get("Cache-Control") != "private, max-age=3600" || rec.Header().Get("ETag") == "" {
		t.Fatalf("no release: %d %q %v", rec.Code, rec.Header().Get("Cache-Control"), rec.Header())
	}
	if !isTile(get(true, shared)) {
		t.Fatal("secret mode, no release: want a tile")
	}

	// a release with a cover, on both sides
	rel, err := db.CreateRelease("ZZPUBLIC Shared Album", audio.ReleaseAlbum, "2020", []string{shared})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []db.Viewer{db.Normal, db.Secret} {
		_, err := db.AddTrack(v, []string{shared}, &audio.Track{Title: "ZZ on release", Duration: time.Minute},
			bytes.NewReader([]byte("x"+strconv.Itoa(int(v.Mode)))), ".mp3", &db.ReleaseLink{ReleaseID: rel.ID.String(), Disc: 1, Position: int(v.Mode) + 1}, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetReleaseCover(rel.ID, strings.NewReader(cover), ".png"); err != nil {
		t.Fatal(err)
	}
	rec = get(false, shared)
	if rec.Code != 200 || rec.Body.String() != cover {
		t.Fatalf("normal mode wants the release cover: %d %q", rec.Code, rec.Body.String())
	}
	if !isTile(get(true, shared)) {
		t.Fatal("secret mode must not show a release cover")
	}

	// the ETag gives a 304
	etag := rec.Header().Get("ETag")
	if rec := get(false, shared, "If-None-Match", etag); rec.Code != http.StatusNotModified {
		t.Fatalf("etag %q = %d", etag, rec.Code)
	}
	tile := get(true, shared)
	if rec := get(true, shared, "If-None-Match", tile.Header().Get("ETag")); rec.Code != http.StatusNotModified {
		t.Fatalf("tile etag = %d", rec.Code)
	}

	// an own picture wins, in both modes
	id := uuid.MustParse(shared)
	if err := db.SaveArtistImage(id, strings.NewReader(own)); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []bool{false, true} {
		if rec := get(secret, shared); rec.Body.String() != own {
			t.Fatalf("secret=%v: own picture lost: %q", secret, rec.Body.String())
		}
	}
	// removing it falls back again
	if err := db.RemoveArtistImage(id); err != nil {
		t.Fatal(err)
	}
	if rec := get(false, shared); rec.Body.String() != cover {
		t.Fatalf("after remove: %q", rec.Body.String())
	}

	// a secret creator stays hidden in normal mode
	if rec := get(false, a.ID["secret artist"]); rec.Code != http.StatusNotFound {
		t.Fatalf("secret artist in normal mode = %d", rec.Code)
	}
	if !isTile(get(true, a.ID["secret artist"])) {
		t.Fatal("secret artist in secret mode: want a tile")
	}
}
