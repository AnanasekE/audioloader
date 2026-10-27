package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestUploadRefusesFilesThatArentAudio(t *testing.T) {
	a := newModeApp(t)
	mp3, err := os.ReadFile("../tags/testdata/tagged.mp3")
	if err != nil {
		t.Fatal(err)
	}
	// valid MP3 bytes under a name that would be served as a page
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("title", "Page")
	mw.WriteField("artist_id", a.ID["public artist"])
	fw, _ := mw.CreateFormFile("track", "x.html")
	fw.Write(mp3)
	mw.Close()
	req := httptest.NewRequest("POST", "/ui/tracks", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	WithSecurityHeaders(a.h).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "must be MP3") {
		t.Fatalf("upload of x.html = %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
}
