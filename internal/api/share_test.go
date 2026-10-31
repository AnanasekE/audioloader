package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/sources/secret"
	"audioloader/internal/sources/secret/secrettest"
	"audioloader/internal/vault"
)

func TestShareToken(t *testing.T) {
	dbtest.New(t)
	hash, _ := vault.Hash("pass")
	theVault = vault.New([]byte(hash), []byte("0123456789abcdef0123456789abcdef"))
	secret.SetDefault(secret.New("pages.example.test"))
	t.Cleanup(func() {
		theVault = nil
		SetShareToken("")
		secret.SetDefault(nil)
	})
	token := strings.Repeat("a", 32)
	req := func(auth string) *http.Request {
		r := httptest.NewRequest("POST", "/api/share", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		return r
	}

	SetShareToken(token)
	if !shareEnabled() {
		t.Fatal("not enabled with a token, a vault and a source")
	}
	for _, c := range []struct {
		name, auth string
		want       bool
	}{
		{"right token", "Bearer " + token, true},
		{"wrong token", "Bearer " + strings.Repeat("b", 32), false},
		{"missing header", "", false},
		{"bearer without a token", "Bearer ", false},
		{"no bearer prefix", token, false},
		{"shorter token", "Bearer " + token[:31], false},
		{"longer token", "Bearer " + token + "a", false},
	} {
		if got := shareToken(req(c.auth)); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}

	// off: no token, a short token, no vault, no source
	SetShareToken("")
	if shareEnabled() || shareToken(req("Bearer "+token)) {
		t.Error("enabled without a token")
	}
	SetShareToken(token[:31])
	if shareEnabled() || shareToken(req("Bearer "+token[:31])) {
		t.Error("enabled with a short token")
	}
	SetShareToken(token)
	theVault = nil
	if shareEnabled() {
		t.Error("enabled without a vault")
	}
	theVault = vault.New([]byte(hash), []byte("0123456789abcdef0123456789abcdef"))
	secret.SetDefault(nil)
	if shareEnabled() {
		t.Error("enabled without a source")
	}
}

func TestShareEndpoint(t *testing.T) {
	a := newModeApp(t)
	fake, src := secrettest.New(t)
	shareFailDelay = 0
	t.Cleanup(func() {
		shareFailDelay = time.Second
		SetShareToken("")
		secret.SetDefault(nil)
	})
	m4a, err := os.ReadFile("../audio/testdata/chapters.m4a")
	if err != nil {
		t.Fatal(err)
	}
	fake.Files[secrettest.MediaHost+"/sounds/one.m4a"] = m4a
	page := func(title string) string {
		return `<div class="jp-title">` + title + `</div><div class="jp-description">About it [Calm]</div>
			<script>$("#p").jPlayer("setMedia", { m4a: "https:\/\/media.example.test\/sounds\/one.m4a" })</script>`
	}
	known := fake.Page("/items/ZZSECRET Artist/one", page("One"))
	fresh := fake.Page("/items/Newcomer/one", page("Fresh"))
	fresh2 := fake.Page("/items/Newcomer/two", page("Fresh Two"))
	untitled := fake.Page("/items/Newcomer/some name", `<script>x("https://media.example.test/sounds/one.m4a")</script>`)
	fake.Page("/solo", page("Solo"))
	fake.Page("/items/Newcomer", `<a href="/items/Newcomer/one">One</a> <a href="/items/Newcomer/two">Two</a>`)
	fake.Page("/items/Newcomer/silent", `<p>nothing here</p>`)
	token := strings.Repeat("t", 32)

	var answers []string
	share := func(auth, url string, cookie bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/share", strings.NewReader(neturl.Values{"url": {url}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		if cookie {
			req.AddCookie(a.v.Cookie(time.Now(), false))
		}
		rec := httptest.NewRecorder()
		a.h.ServeHTTP(rec, req)
		answers = append(answers, rec.Body.String())
		return rec
	}
	ok := "Bearer " + token

	// off: no token set, no source
	secret.SetDefault(src)
	SetShareToken("")
	if rec := share(ok, known, false); rec.Code != http.StatusNotFound {
		t.Errorf("token unset = %d", rec.Code)
	}
	SetShareToken(token)
	secret.SetDefault(nil)
	if rec := share(ok, known, false); rec.Code != http.StatusNotFound {
		t.Errorf("source off = %d", rec.Code)
	}
	secret.SetDefault(src)
	theVault = nil
	if rec := share(ok, known, false); rec.Code != http.StatusNotFound {
		t.Errorf("vault off = %d", rec.Code)
	}
	theVault = a.v

	// the token, and only the token
	for _, c := range []struct {
		auth   string
		cookie bool
	}{{"", false}, {"Bearer nope", false}, {"", true}, {"Bearer nope", true}} {
		if rec := share(c.auth, known, c.cookie); rec.Code != http.StatusUnauthorized {
			t.Errorf("auth %q, cookie %v = %d", c.auth, c.cookie, rec.Code)
		}
	}
	if n := countJobs(downloads.JobSecretDownload); n != 0 {
		t.Fatalf("%d jobs queued without the token", n)
	}

	// a known uploader: queued for them, no new creator
	before, _ := db.ListArtists(db.Secret)
	rec := share(ok, known, false)
	if rec.Code != http.StatusAccepted || strings.TrimSpace(rec.Body.String()) != "Queued: One by ZZSECRET Artist" ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("share = %d %q %s", rec.Code, rec.Body, rec.Header().Get("Content-Type"))
	}
	if after, _ := db.ListArtists(db.Secret); len(after) != len(before) {
		t.Error("a creator was added for a known uploader")
	}
	// the same page again while queued: still accepted, one job
	if rec := share(ok, known, false); rec.Code != http.StatusAccepted {
		t.Errorf("second share = %d", rec.Code)
	}
	if n := countJobs(downloads.JobSecretDownload); n != 1 {
		t.Errorf("%d jobs for one page", n)
	}
	normalized, _ := src.Normalize(known)
	job, err := jobs.Latest(downloads.JobSecretDownload, normalized)
	if err != nil || job == nil {
		t.Fatalf("job = %v, %v", job, err)
	}
	var imp downloads.SecretImport
	if err := json.Unmarshal(job.Payload, &imp); err != nil {
		t.Fatal(err)
	}
	if imp.Title != "One" || imp.Description != "About it [Calm]" || !slices.Equal(imp.Tags, []string{"Calm"}) ||
		!slices.Equal(imp.ArtistIDs, []string{a.ID["secret artist"]}) {
		t.Errorf("payload = %+v", imp)
	}

	// an unknown uploader: the creator is made once, and a JSON body works
	req := httptest.NewRequest("POST", "/api/share", strings.NewReader(`{"url": "`+fresh+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", ok)
	rec = httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "by Newcomer") {
		t.Fatalf("json share = %d %s", rec.Code, rec.Body)
	}
	if rec := share(ok, fresh2, false); rec.Code != http.StatusAccepted {
		t.Fatalf("second page of the uploader = %d %s", rec.Code, rec.Body)
	}
	n := 0
	artists, _ := db.ListArtists(db.Secret)
	for _, ar := range artists {
		if ar.Name == "Newcomer" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d creators named Newcomer", n)
	}
	if got, _ := db.ListArtists(db.Normal); len(got) != 2 {
		t.Errorf("normal mode sees %d artists", len(got))
	}

	// the title falls back to the address
	if rec := share(ok, untitled, false); rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "Queued: some name by Newcomer") {
		t.Errorf("untitled = %d %s", rec.Code, rec.Body)
	}

	// refusals
	if rec := share(ok, "https://evil.example.test/items/a/b", false); rec.Code != http.StatusBadRequest {
		t.Errorf("wrong host = %d", rec.Code)
	}
	if rec := share(ok, fake.Srv.URL, false); rec.Code != http.StatusBadRequest {
		t.Errorf("not the source = %d", rec.Code)
	}
	if rec := share(ok, "https://"+secrettest.PageHost+"/items/Newcomer", false); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "list of recordings") {
		t.Errorf("listing = %d %s", rec.Code, rec.Body)
	}
	if rec := share(ok, "https://"+secrettest.PageHost+"/items/Newcomer/silent", false); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("no audio = %d %s", rec.Code, rec.Body)
	}
	solo := "https://" + secrettest.PageHost + "/solo"
	rec = share(ok, solo, false)
	if lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n"); rec.Code != http.StatusUnprocessableEntity || len(lines) != 2 ||
		lines[0] != "No creator in that address. Finish it in the app:" ||
		lines[1] != "http://example.com/upload/secret-source?url="+neturl.QueryEscape(solo) {
		t.Errorf("no uploader = %d %q", rec.Code, rec.Body)
	}
	if n := countJobs(downloads.JobSecretDownload); n != 4 {
		t.Errorf("%d jobs after the refusals, want 4", n)
	}

	// a page already in the library
	downloads.Register()
	for range 4 {
		if ran, err := jobs.RunOnce(context.Background(), jobs.Lane{Only: []string{downloads.JobSecretDownload}}); err != nil || !ran {
			t.Fatalf("run = %v, %v", ran, err)
		}
	}
	if rec := share(ok, known, false); rec.Code != http.StatusConflict || !strings.HasPrefix(rec.Body.String(), "Already in your library: ") {
		t.Errorf("known page again = %d %s", rec.Code, rec.Body)
	}

	// nothing from the normal side in any answer
	for _, body := range answers {
		if strings.Contains(strings.ToLower(body), "zzpublic") {
			t.Errorf("an answer names a normal-mode item: %s", body)
		}
	}

	// the rate limit
	shareBucket.Lock()
	shareBucket.tokens = 0
	shareBucket.at = time.Now()
	shareBucket.Unlock()
	if rec := share(ok, fresh, false); rec.Code != http.StatusTooManyRequests {
		t.Errorf("over the rate = %d", rec.Code)
	}
}
