package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"audioloader/internal/applemusic"
	"audioloader/internal/dbtest"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
)

// fakeAppleMusic points the shared Apple Music client at a server that
// answers with the given pages by path (404 otherwise) for the test.
func fakeAppleMusic(t *testing.T, pages map[string]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, ok := pages[r.URL.Path]; ok {
			w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
	c := applemusic.Default()
	oldURL, oldSleep := c.BaseURL, c.Sleep
	c.BaseURL = srv.URL
	c.Sleep = func(ctx context.Context, d time.Duration) error { return nil }
	t.Cleanup(func() {
		c.BaseURL, c.Sleep = oldURL, oldSleep
		srv.Close()
	})
}

// appleSongPage is a song page of the shape Apple serves, with the length
// in its schema data.
func appleSongPage(id, title, artists, album, length string) string {
	data := map[string]any{"data": []any{map[string]any{"data": map[string]any{"sections": []any{map[string]any{
		"itemKind": "songDetailHeader",
		"items": []any{map[string]any{
			"title": title, "artists": artists, "album": album,
			"contentDescriptor": map[string]any{"identifiers": map[string]string{"storeAdamID": id}},
		}},
	}}}}}}
	b, _ := json.Marshal(data)
	return fmt.Sprintf(`<html><body><script type="application/json" id="serialized-server-data">%s</script>
<script id=schema:song type="application/ld+json">{"audio":{"@type":"MusicRecording","duration":%q}}</script></body></html>`, b, length)
}

const (
	appleTeardrop    = "https://music.apple.com/us/song/teardrop/1001"
	appleTeardropEP  = "https://music.apple.com/us/song/teardrop/1002"
	appleUnreadable  = "https://music.apple.com/us/song/broken/1003"
	appleMissingSong = "https://music.apple.com/us/song/gone/1004"
)

// fakeTeardrop serves Apple song pages for Teardrop (5:30, on Mezzanine) and
// a version that is 4:00 long, and MusicBrainz with the Mezzanine recording
// first and the Spirits of Nature one second.
func fakeTeardrop(t *testing.T) {
	t.Helper()
	fakeAppleMusic(t, map[string]string{
		"/us/song/teardrop/1001": appleSongPage("1001", "Teardrop", "Massive Attack", "Mezzanine", "PT5M30S"),
		"/us/song/teardrop/1002": appleSongPage("1002", "Teardrop", "Massive Attack", "Mezzanine", "PT4M0S"),
		"/us/song/broken/1003":   "<html>nothing</html>",
	})
	artist := `"artist-credit":[{"name":"Massive Attack","artist":{"id":"10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8","name":"Massive Attack"}}]`
	fakeMusicBrainz(t, map[string]string{
		"/recording": `{"recordings":[
			{"id":"` + teardropRecording + `","title":"Teardrop","length":330026,` + artist + `,
				"releases":[{"id":"` + mezzanine + `","title":"Mezzanine","status":"Official","date":"1998-04-20","release-group":{"primary-type":"Album"}}]},
			{"id":"` + searchedRecording + `","title":"Teardrop","length":322000,` + artist + `,
				"releases":[{"id":"f9199933-4e7a-4a71-baa9-687258054114","title":"Spirits of Nature 5","status":"Official","date":"1998","release-group":{"primary-type":"Album"}}]}]}`,
		"/release/": mustRead(t, "../mb/testdata/release.json"),
	})
}

func TestAddLookupAppleSong(t *testing.T) {
	dbtest.New(t)
	fakeTeardrop(t)
	h := downloadsHandler(t)
	lookup := func(q string) (int, string) {
		rec := do(h, "POST", "/ui/add/lookup", url.Values{"q": {q}})
		return rec.Code, rec.Body.String()
	}

	// the song is shown as the MusicBrainz recordings it may be; the sure one
	// is first, on the release titled like the Apple album, and marked
	code, body := lookup(appleTeardrop)
	if code != 200 || strings.Count(body, "Best match") != 1 || strings.Count(body, ">Add<") != 2 ||
		strings.Index(body, "Mezzanine") > strings.Index(body, "Spirits of Nature 5") || strings.Index(body, "Best match") > strings.Index(body, "Mezzanine") {
		t.Fatalf("song = %d\n%s", code, body)
	}

	// a length that doesn't fit leaves nothing marked
	code, body = lookup(appleTeardropEP)
	if code != 200 || strings.Contains(body, "Best match") || strings.Count(body, ">Add<") != 2 {
		t.Fatalf("unsure song = %d\n%s", code, body)
	}

	// an album link without a song, and a playlist, are not a song
	for _, q := range []string{"https://music.apple.com/us/album/mezzanine/1", "https://music.apple.com/us/artist/x/1"} {
		if code, body := lookup(q); code != 400 || strings.TrimSpace(body) != msgAppleLink {
			t.Errorf("%s = %d %q", q, code, body)
		}
	}
	// pages Apple doesn't have, or that changed
	if code, body := lookup(appleMissingSong); code != 404 || !strings.Contains(body, "isn't on Apple Music") {
		t.Errorf("missing = %d %q", code, body)
	}
	if code, body := lookup(appleUnreadable); code != 502 || !strings.Contains(body, "Apple Music changed its page; this link can't be read.") {
		t.Errorf("unreadable = %d %q", code, body)
	}
}

func TestShareAppleSong(t *testing.T) {
	dbtest.New(t)
	fakeTeardrop(t)
	h := downloadsHandler(t)
	shareFailDelay = 0
	token := strings.Repeat("t", 32)
	SetShareToken(token)
	t.Cleanup(func() {
		shareFailDelay = time.Second
		SetShareToken("")
	})
	share := func(auth, link string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/share", strings.NewReader(url.Values{"url": {link}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	ok := "Bearer " + token

	// any other page is for the secret source, which is off here
	if rec := share(ok, "https://example.com/x"); rec.Code != http.StatusNotFound {
		t.Fatalf("other page = %d", rec.Code)
	}

	// the token is still needed
	if rec := share("", appleTeardrop); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token = %d", rec.Code)
	}

	// no vault source is set up here, and a song with a sure match is added
	rec := share(ok, appleTeardrop)
	if rec.Code != http.StatusAccepted || strings.TrimSpace(rec.Body.String()) != "Queued: Teardrop by Massive Attack" ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("share = %d %q", rec.Code, rec.Body)
	}
	if counts, _ := jobs.Counts(downloads.JobDownload); counts["queued"] != 1 {
		t.Fatalf("download jobs = %v", counts)
	}
	// the same song again is already there
	if rec := share(ok, appleTeardrop); rec.Code != http.StatusConflict || !strings.HasPrefix(rec.Body.String(), "Already in your library: Teardrop") {
		t.Fatalf("again = %d %q", rec.Code, rec.Body)
	}
	if counts, _ := jobs.Counts(downloads.JobDownload); counts["queued"] != 1 {
		t.Fatalf("download jobs after the repeat = %v", counts)
	}

	// without a sure match nothing is added, and the Add page is named
	rec = share(ok, appleTeardropEP)
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if rec.Code != http.StatusUnprocessableEntity || len(lines) != 2 || lines[0] != "Open the Add page to pick." ||
		lines[1] != "http://example.com/add?q="+url.QueryEscape(appleTeardropEP) {
		t.Fatalf("unsure = %d %q", rec.Code, rec.Body)
	}
	if rec := share(ok, "https://music.apple.com/us/playlist/x/pl.abc"); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "import a playlist") {
		t.Fatalf("playlist = %d %q", rec.Code, rec.Body)
	}
	if rec := share(ok, "https://music.apple.com/us/album/x/1"); rec.Code != http.StatusBadRequest {
		t.Fatalf("album = %d", rec.Code)
	}
	if rec := share(ok, appleMissingSong); rec.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", rec.Code)
	}
	if counts, _ := jobs.Counts(downloads.JobDownload); counts["queued"] != 1 {
		t.Fatalf("download jobs after the refusals = %v", counts)
	}
}
