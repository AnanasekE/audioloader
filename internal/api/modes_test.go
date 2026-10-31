package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/sources/secret"
	"audioloader/internal/sources/secret/secrettest"
	"audioloader/internal/vault"
)

// modeApp is the app with a vault, and a way to ask as either mode.
type modeApp struct {
	t  *testing.T
	h  http.Handler
	v  *vault.Vault
	ID map[string]string
}

func newModeApp(t *testing.T) *modeApp {
	t.Helper()
	dbtest.New(t)
	ids := modeLibrary(t)
	hash, _ := vault.Hash("pass")
	v := vault.New([]byte(hash), []byte("0123456789abcdef0123456789abcdef"))
	mux := http.NewServeMux()
	Register(mux, v)
	return &modeApp{t: t, h: WithViewer(mux), v: v, ID: ids}
}

func (a *modeApp) do(secret bool, method, target string, form url.Values) *httptest.ResponseRecorder {
	a.t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	if secret {
		req.AddCookie(a.v.Cookie(time.Now(), false))
	}
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	return rec
}

func TestSecretTrackListSorts(t *testing.T) {
	a := newModeApp(t)
	if err := db.SetTrackTagsForTest(a.ID["secret track"], "rain", "asmr"); err != nil {
		t.Fatal(err)
	}
	body := func(q string) string { return a.do(true, "GET", "/tracks"+q, nil).Body.String() }
	at := func(s, sub string) int { return strings.Index(s, sub) }

	// date added: the newest first
	b := body("")
	if !(at(b, "ZZSECRET Cover") < at(b, "ZZSECRET Shared Song") && at(b, "ZZSECRET Shared Song") < at(b, "ZZSECRET Song")) {
		t.Errorf("not newest first")
	}
	// creator: a group per creator, in name order
	b = body("?sort=creator")
	if !(at(b, ">Shared Artist<") > 0 && at(b, ">Shared Artist<") < at(b, ">ZZSECRET Artist<")) {
		t.Errorf("creator groups out of order")
	}
	// tag: groups by tag, the untagged last
	b = body("?sort=tag")
	if !(at(b, ">asmr<") > 0 && at(b, ">asmr<") < at(b, ">rain<") && at(b, ">rain<") < at(b, ">Untagged<")) {
		t.Errorf("tag groups out of order")
	}
	// normal mode has no sorting
	if b := a.do(false, "GET", "/tracks?sort=creator", nil).Body.String(); strings.Contains(b, "Sort by") {
		t.Error("normal mode offers sorting")
	}
}

func TestPlaylistsBelongToAMode(t *testing.T) {
	a := newModeApp(t)
	// the secret playlist is created in secret mode and holds secret tracks only
	rec := a.do(true, "POST", "/ui/playlists", url.Values{"name": {"Night"}})
	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Fatalf("create = %d", rec.Code)
	}
	playlists, _ := db.ListPlaylists(db.Secret)
	var id string
	for _, p := range playlists {
		if p.Name == "Night" {
			id = p.ID.String()
		}
	}
	if id == "" {
		t.Fatal("not in secret mode's list")
	}
	if ps, _ := db.ListPlaylists(db.Normal); len(ps) != 2 {
		t.Fatalf("normal mode lists %d playlists", len(ps))
	}
	add := func(secret bool, track string) int {
		return a.do(secret, "POST", "/ui/playlists/"+id+"/tracks/add", url.Values{"track_id": {track}}).Code
	}
	if code := add(true, a.ID["public track"]); code != http.StatusNotFound { // not visible in secret mode at all
		t.Errorf("adding a normal track = %d", code)
	}
	if code := add(true, a.ID["secret track"]); code != http.StatusOK {
		t.Errorf("adding a secret track = %d", code)
	}
	if code := add(false, a.ID["public track"]); code != http.StatusNotFound { // playlist of the other mode
		t.Errorf("normal mode adding to a secret playlist = %d", code)
	}
	// and the other mode can't rename or delete it
	for _, path := range []string{"/rename", "/delete"} {
		if rec := a.do(false, "POST", "/ui/playlists/"+id+path, url.Values{"name": {"x"}}); rec.Code != http.StatusNotFound {
			t.Errorf("%s from normal mode = %d", path, rec.Code)
		}
	}
	// a track can't be made secret without leaving a normal playlist
	if err := db.SetSecret(a.ID["public track 2"], true); err != nil {
		t.Fatal(err)
	}
	if pl, _ := db.GetPlaylist(db.All, a.ID["public playlist"]); len(pl.Tracks) != 1 {
		t.Errorf("normal playlist keeps the moved track: %d", len(pl.Tracks))
	}
}

func TestNavHidesNormalOnlyPages(t *testing.T) {
	a := newModeApp(t)
	for _, c := range []struct {
		secret bool
		link   string
		want   bool
	}{{false, `href="/releases"`, true}, {false, `href="/inbox"`, true}, {true, `href="/releases"`, false}, {true, `href="/inbox"`, false},
		{true, `href="/tagging"`, false}, {true, `href="/downloads"`, false}, {true, `href="/tracks"`, true}, {true, `href="/shows"`, false}, {true, `href="/artists"`, true}} {
		body := a.do(c.secret, "GET", "/tracks", nil).Body.String()
		if strings.Contains(body, c.link) != c.want {
			t.Errorf("secret=%v: %s present = %v, want %v", c.secret, c.link, !c.want, c.want)
		}
	}
}

func TestSecretTrackEditForm(t *testing.T) {
	a := newModeApp(t)
	id := a.ID["secret track"]
	form := func(extra url.Values) url.Values {
		f := url.Values{"title": {"ZZSECRET Song"}, "kind": {"music"}, "artist_id": {a.ID["secret artist"]}}
		for k, v := range extra {
			f[k] = v
		}
		return f
	}
	if rec := a.do(true, "POST", "/ui/tracks/"+id, form(url.Values{"description": {" Line one\nLine two "}, "tags": {"Rain, asmr, rain,  "}})); rec.Code != http.StatusNoContent {
		t.Fatalf("save = %d", rec.Code)
	}
	tr, _ := db.GetTrack(db.Secret, id)
	if tr.Description != "Line one\nLine two" || strings.Join(tr.Tags, "|") != "asmr|Rain" {
		t.Fatalf("saved %q %v", tr.Description, tr.Tags)
	}
	// a save without the fields leaves them alone
	a.do(true, "POST", "/ui/tracks/"+id, form(nil))
	if tr, _ := db.GetTrack(db.Secret, id); tr.Description == "" || len(tr.Tags) != 2 {
		t.Fatal("a save without the fields cleared them")
	}
	// and a normal track takes neither
	pid := a.ID["public track"]
	a.do(false, "POST", "/ui/tracks/"+pid, url.Values{"title": {"ZZPUBLIC Song 1"}, "kind": {"music"}, "artist_id": {a.ID["public artist"]}, "description": {"x"}, "tags": {"y"}})
	if tr, _ := db.GetTrack(db.Normal, pid); tr.Description != "" || len(tr.Tags) != 0 {
		t.Fatalf("normal track got %q %v", tr.Description, tr.Tags)
	}
	// the edit page of a secret track has no release or cover-of parts
	b := a.do(true, "GET", "/tracks/"+id+"/edit", nil).Body.String()
	for _, want := range []string{`name="description"`, `name="tags"`, `name="image"`} {
		if !strings.Contains(b, want) {
			t.Errorf("edit page lacks %s", want)
		}
	}
	if strings.Contains(b, "cover_mode") {
		t.Error("a secret track can't be a cover")
	}
	// the list shows them
	b = a.do(true, "GET", "/tracks", nil).Body.String()
	if !strings.Contains(b, "Line one") || !strings.Contains(b, ">Rain<") {
		t.Error("the list lacks the description or tags")
	}
}

func TestSecretTrackPicture(t *testing.T) {
	a := newModeApp(t)
	id := a.ID["secret track"]
	png, err := os.ReadFile("../../assets/img/default_secret.png")
	if err != nil {
		t.Fatal(err)
	}
	upload := func(secret bool, data []byte) *httptest.ResponseRecorder {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("image", "x.png")
		fw.Write(data)
		mw.Close()
		req := httptest.NewRequest("POST", "/ui/tracks/"+id+"/image", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		if secret {
			req.AddCookie(a.v.Cookie(time.Now(), false))
		}
		rec := httptest.NewRecorder()
		a.h.ServeHTTP(rec, req)
		return rec
	}
	if rec := upload(false, png); rec.Code != http.StatusNotFound {
		t.Fatalf("upload from normal mode = %d", rec.Code)
	}
	if rec := upload(true, []byte("not an image")); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad image = %d", rec.Code)
	}
	if rec := upload(true, png); rec.Code != http.StatusNoContent {
		t.Fatalf("upload = %d", rec.Code)
	}
	tr, _ := db.GetTrack(db.Secret, id)
	if !strings.HasPrefix(tr.ImgURL, "/track-images/"+id) {
		t.Fatalf("img = %s", tr.ImgURL)
	}
	rec := a.do(true, "GET", tr.ImgURL, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "private, no-store" || !bytes.Equal(rec.Body.Bytes(), png) {
		t.Fatalf("picture = %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := a.do(false, "GET", tr.ImgURL, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("picture in normal mode = %d", rec.Code)
	}
	// deleting the track removes the file
	files, _ := filepath.Glob(filepath.Join(db.TrackImagesDir, id+"-*"))
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	if rec := a.do(true, "POST", "/ui/tracks/"+id+"/delete", url.Values{}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if files, _ := filepath.Glob(filepath.Join(db.TrackImagesDir, id+"-*")); len(files) != 0 {
		t.Fatalf("picture left behind: %v", files)
	}
}

func TestAccentAndBadge(t *testing.T) {
	a := newModeApp(t)
	n := a.do(false, "GET", "/tracks", nil).Body.String()
	s := a.do(true, "GET", "/tracks", nil).Body.String()
	if !strings.Contains(n, `data-mode="normal"`) || strings.Contains(n, ">Secret<") {
		t.Error("normal mode page marks itself secret")
	}
	if !strings.Contains(s, `data-mode="secret"`) || !strings.Contains(s, ">Secret<") || !strings.Contains(s, ">Lock<") {
		t.Error("secret mode page lacks the mode, badge or lock")
	}
}

// upload posts a multipart form to path as one of the modes.
func (a *modeApp) upload(secret bool, path string, fields url.Values, file []byte) *httptest.ResponseRecorder {
	a.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, vs := range fields {
		for _, v := range vs {
			mw.WriteField(k, v)
		}
	}
	fw, _ := mw.CreateFormFile("track", "x.mp3")
	fw.Write(file)
	mw.Close()
	req := httptest.NewRequest("POST", path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if secret {
		req.AddCookie(a.v.Cookie(time.Now(), false))
	}
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	return rec
}

func TestUploadsFollowTheMode(t *testing.T) {
	a := newModeApp(t)
	mp3, err := os.ReadFile("../tags/testdata/tagged.mp3")
	if err != nil {
		t.Fatal(err)
	}
	jobs := func() (n int) {
		db.DB.QueryRow(`SELECT COUNT(*) FROM jobs;`).Scan(&n)
		return n
	}
	find := func(v db.Viewer, title string) string {
		tracks, _ := db.ListTracks(v)
		for _, tr := range tracks {
			if tr.Title == title {
				return tr.ID.String()
			}
		}
		return ""
	}

	// secret mode: a secret track, whatever the form says about releases and secrecy
	before := jobs()
	rec := a.upload(true, "/ui/tracks", url.Values{
		"title": {"ZZSECRET Upload"}, "artist_id": {a.ID["secret artist"]},
		"secret": {"0"}, "release_id": {a.ID["public release"]}, "disc": {"1"}, "position": {"9"}, "date": {"2001"},
		"description": {"About it"}, "tags": {"one, two"},
	}, mp3)
	if rec.Code != http.StatusOK {
		t.Fatalf("secret upload = %d %s", rec.Code, rec.Body)
	}
	id := find(db.Secret, "ZZSECRET Upload")
	if id == "" {
		t.Fatal("no secret track")
	}
	tr, _ := db.GetTrack(db.Secret, id)
	if !tr.Secret || len(tr.Releases) != 0 || tr.Description != "About it" || len(tr.Tags) != 2 {
		t.Fatalf("track = %+v", tr)
	}
	if rel, _ := db.GetRelease(db.All, a.ID["public release"]); len(rel.Tracks) != 2 {
		t.Fatalf("the secret track got onto a release: %d tracks", len(rel.Tracks))
	}
	if jobs() != before {
		t.Error("a secret track was queued for tagging")
	}

	// normal mode: a normal track, whatever the form says
	rec = a.upload(false, "/ui/tracks", url.Values{
		"title": {"ZZPUBLIC Upload"}, "artist_id": {a.ID["public artist"]}, "secret": {"1"}, "description": {"x"}, "tags": {"y"},
	}, append(mp3, 1)) // another file
	if rec.Code != http.StatusOK {
		t.Fatalf("normal upload = %d %s", rec.Code, rec.Body)
	}
	id = find(db.Normal, "ZZPUBLIC Upload")
	if id == "" {
		t.Fatal("no normal track")
	}
	if tr, _ := db.GetTrack(db.Normal, id); tr.Secret || tr.Description != "" || len(tr.Tags) != 0 || len(tr.Releases) != 1 {
		t.Fatalf("normal track = %+v", tr)
	}
	if jobs() == before {
		t.Error("a normal track wasn't queued for tagging")
	}

	// the same file from the other side is refused without saying why
	rec = a.upload(false, "/ui/tracks", url.Values{"title": {"Again"}, "artist_id": {a.ID["public artist"]}}, mp3)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "upload failed") {
		t.Fatalf("duplicate of a secret file = %d %s", rec.Code, rec.Body)
	}
}

func TestSecretModeHidesReleaseControls(t *testing.T) {
	a := newModeApp(t)
	for _, c := range []struct {
		path    string
		gone    []string
		present []string
	}{
		{"/add", []string{`name="date"`, `name="secret"`, "YouTube", "new-release-form"}, []string{`name="description"`, `name="tags"`, "Podcast episode"}},
		{"/upload/batch", []string{`name="secret"`, "Release for all tracks", "Disc</span>"}, []string{"Creators"}},
		{"/upload/import", []string{`import-secret`}, []string{"Each file becomes a secret track"}},
		{"/artists", []string{"Add from MusicBrainz"}, nil},
	} {
		secret := a.do(true, "GET", c.path, nil).Body.String()
		normal := a.do(false, "GET", c.path, nil).Body.String()
		for _, g := range c.gone {
			if strings.Contains(secret, g) {
				t.Errorf("%s in secret mode still has %q", c.path, g)
			}
		}
		for _, p := range c.present {
			if !strings.Contains(secret, p) {
				t.Errorf("%s in secret mode lacks %q", c.path, p)
			}
		}
		if c.path == "/artists" && !strings.Contains(normal, "Add from MusicBrainz") {
			t.Errorf("/artists in normal mode lacks the MusicBrainz box")
		}
		if strings.Contains(normal, `name="secret"`) || strings.Contains(normal, "import-secret") {
			t.Errorf("%s in normal mode has a secret checkbox", c.path)
		}
	}
}

func TestEpisodeUploadFollowsTheMode(t *testing.T) {
	a := newModeApp(t)
	mp3, err := os.ReadFile("../tags/testdata/tagged.mp3")
	if err != nil {
		t.Fatal(err)
	}
	// a new show named in secret mode is secret; the form's flag is not read
	rec := a.upload(true, "/ui/episodes", url.Values{"title": {"ZZSECRET Ep"}, "show_name": {"ZZSECRET New Show"}, "secret": {"0"}}, mp3)
	if rec.Code != http.StatusOK {
		t.Fatalf("secret episode = %d %s", rec.Code, rec.Body)
	}
	shows, _ := db.ListShows(db.Secret)
	found := false
	for _, s := range shows {
		found = found || (s.Title == "ZZSECRET New Show" && s.Count == 1)
	}
	if !found {
		t.Fatal("the secret show doesn't list its episode")
	}
	for _, s := range must(db.ListShows(db.Normal)) {
		if strings.Contains(s.Title, "ZZSECRET") {
			t.Fatal("normal mode lists the secret show")
		}
	}
	// the same file from normal mode is refused without saying why
	rec = a.upload(false, "/ui/episodes", url.Values{"title": {"Again"}, "show_name": {"Some Show"}}, mp3)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "upload failed") {
		t.Fatalf("duplicate = %d %s", rec.Code, rec.Body)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestSecretSourceDownload(t *testing.T) {
	a := newModeApp(t)
	fake, src := secrettest.New(t)
	m4a, err := os.ReadFile("../audio/testdata/chapters.m4a")
	if err != nil {
		t.Fatal(err)
	}
	fake.Files[secrettest.MediaHost+"/sounds/one.m4a"] = m4a
	pageURL := fake.Page("/items/someone/one", `<html><head><title>Fallback</title></head><body>
		<div class="jp-title">The Title</div>
		<div class="jp-description"><p>Line one<br>Line two</p></div>
		<script>$("#p").jPlayer("setMedia", { m4a: "https:\/\/media.example.test\/sounds\/one.m4a" })</script></body></html>`)
	fake.Page("/items/someone/copy", `<script>x("https://media.example.test/sounds/one.m4a")</script>`)

	// off: no source configured
	secret.SetDefault(nil)
	t.Cleanup(func() { secret.SetDefault(nil) })
	for _, route := range SecretOnly {
		method, path, _ := strings.Cut(route, " ")
		if rec := a.do(true, method, path, url.Values{}); rec.Code != http.StatusNotFound {
			t.Errorf("%s without a source = %d", route, rec.Code)
		}
	}
	secret.SetDefault(src)
	// and secret mode only
	for _, route := range SecretOnly {
		method, path, _ := strings.Cut(route, " ")
		if rec := a.do(false, method, path, url.Values{}); rec.Code != http.StatusNotFound {
			t.Errorf("%s in normal mode = %d", route, rec.Code)
		}
	}
	if len(SecretOnly) != 4 {
		t.Fatalf("secret-only routes = %v", SecretOnly)
	}

	// the old page forwards to the Add page, with the address in the box
	if b := a.do(true, "GET", "/upload/secret-source?url="+url.QueryEscape(pageURL), nil); b.Code != http.StatusFound || b.Header().Get("Location") != "/add?q="+url.QueryEscape(pageURL) {
		t.Fatalf("old page = %d %s", b.Code, b.Header().Get("Location"))
	}
	if b := a.do(true, "GET", "/upload/secret-source", nil); b.Header().Get("Location") != "/add" {
		t.Fatalf("old page without an address = %d %s", b.Code, b.Header().Get("Location"))
	}
	if b := a.do(true, "GET", "/add?q="+url.QueryEscape(pageURL), nil); b.Code != http.StatusOK ||
		!strings.Contains(b.Body.String(), "/ui/add/lookup") || !strings.Contains(b.Body.String(), `value="`+pageURL+`"`) || !strings.Contains(b.Body.String(), `hx-trigger="load, submit"`) {
		t.Fatalf("add page = %d", b.Code)
	}
	for _, mode := range []bool{true, false} {
		b := a.do(mode, "GET", "/add", nil).Body.String()
		if !strings.Contains(b, "Or upload a file") || strings.Contains(b, `hx-trigger="load, submit"`) {
			t.Errorf("add page (secret %v) lacks the upload part or runs a lookup on load", mode)
		}
		if got := strings.Contains(b, "secret-source"); got != mode {
			t.Errorf("add page (secret %v) mentions the source: %v", mode, got)
		}
		if strings.Contains(b, secrettest.PageHost) {
			t.Errorf("add page (secret %v) names the source's host", mode)
		}
	}

	// Check
	check := func(raw string) *httptest.ResponseRecorder {
		return a.do(true, "POST", "/ui/secret-source/check", url.Values{"url": {raw}})
	}
	if rec := check("https://evil.example.test/items/a/b"); rec.Code != http.StatusBadRequest {
		t.Errorf("wrong host = %d", rec.Code)
	}
	if rec := check("https://pages.example.test/items/a/nothing"); rec.Code != http.StatusBadGateway {
		t.Errorf("missing page = %d", rec.Code)
	}
	rec := check(pageURL + "/?utm=1")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `value="The Title"`) || !strings.Contains(body, "Line one\nLine two") ||
		!strings.Contains(body, `name="url" value="`+pageURL+`"`) || strings.Contains(body, "one.m4a") {
		t.Fatalf("check = %d %s", rec.Code, body)
	}
	if strings.Count(body, " checked") != 1 || !strings.Contains(body, `name="new_creator" value="someone"`) {
		t.Error("the uploader isn't suggested as the one new creator")
	}
	if got := len(fake.Requests()); got != 2 { // the missing page, and this one
		t.Errorf("requests = %v", fake.Requests())
	}

	// Download: creators by hand, and a title
	post := func(extra url.Values) *httptest.ResponseRecorder {
		f := url.Values{"url": {pageURL}, "title": {"The Title"}, "description": {"Line one"}, "tags": {"a, b"}}
		for k, v := range extra {
			f[k] = v
		}
		return a.do(true, "POST", "/ui/secret-source/download", f)
	}
	if rec := post(nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "at least one creator") {
		t.Errorf("no creators = %d %s", rec.Code, rec.Body)
	}
	if rec := post(url.Values{"artist_id": {a.ID["public artist"]}}); rec.Code != http.StatusBadRequest {
		t.Errorf("a creator of the other mode = %d", rec.Code)
	}
	if rec := post(url.Values{"artist_id": {a.ID["secret artist"]}, "title": {" "}}); rec.Code != http.StatusBadRequest {
		t.Errorf("no title = %d", rec.Code)
	}
	if n := countJobs(downloads.JobSecretDownload); n != 0 {
		t.Fatalf("%d jobs queued by failed requests", n)
	}
	two := url.Values{"artist_id": {a.ID["secret artist"], a.ID["shared artist"]}}
	if rec := post(two); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "/ui/secret-source/status?url=") {
		t.Fatalf("download = %d %s", rec.Code, rec.Body)
	}
	if got := len(fake.Requests()); got != 2 {
		t.Errorf("the download asked the page again: %v", fake.Requests())
	}
	if rec := a.do(true, "GET", "/ui/secret-source/status?url="+url.QueryEscape(pageURL), nil); !strings.Contains(rec.Body.String(), "Downloading") {
		t.Errorf("status = %s", rec.Body)
	}

	// the downloads lane does it
	downloads.Register()
	if ran, err := jobs.RunOnce(context.Background(), jobs.Lane{Only: []string{downloads.JobSecretDownload}}); err != nil || !ran {
		t.Fatalf("run = %v, %v", ran, err)
	}
	id, _ := db.TrackBySourceURL(pageURL)
	tr, err := db.GetTrack(db.Secret, id)
	if err != nil || !tr.Secret || tr.Title != "The Title" || tr.Description != "Line one" || len(tr.Tags) != 2 || len(tr.Artists) != 2 || len(tr.Releases) != 0 || tr.Duration <= 0 {
		t.Fatalf("track = %+v, %v", tr, err)
	}
	if _, err := db.GetTrack(db.Normal, id); err == nil {
		t.Error("the track is visible in normal mode")
	}
	if got := fake.Requests(); len(got) != 3 || got[2] != secrettest.MediaHost+"/sounds/one.m4a" {
		t.Errorf("requests = %v, want one page and one file", got)
	}
	if got := fake.Referers[secrettest.MediaHost+"/sounds/one.m4a"]; got != pageURL {
		t.Errorf("referer = %q", got)
	}
	rec = a.do(true, "GET", "/ui/secret-source/status?url="+url.QueryEscape(pageURL), nil)
	if !strings.Contains(rec.Body.String(), "Done") || !strings.Contains(rec.Body.String(), "/tracks/"+id+"/edit") {
		t.Errorf("done status = %s", rec.Body)
	}

	// the same page again is reported before anything is fetched
	if rec := check(pageURL); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Already in the library") {
		t.Errorf("second check = %d %s", rec.Code, rec.Body)
	}
	if rec := post(two); rec.Code != http.StatusConflict {
		t.Errorf("second download = %d", rec.Code)
	}
	if got := len(fake.Requests()); got != 3 {
		t.Errorf("requests = %v", fake.Requests())
	}

	// the uploader's page lists their other pages, the downloaded one folded away
	listURL := fake.Page("/items/someone", `<a href="/items/someone/one">The one</a> <a href="/items/someone/copy">A copy</a> <a href="/items/other/x">Not theirs</a>`)
	uploaderLink := "/add?q=" + url.QueryEscape(listURL)
	if rec := check(pageURL); !strings.Contains(rec.Body.String(), uploaderLink) {
		t.Errorf("the duplicate message doesn't offer the uploader's page: %s", rec.Body)
	}
	rec = check("www." + secrettest.PageHost + "/items/someone/")
	body = rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "1 not in the library, 1 already downloaded") ||
		!strings.Contains(body, `value="https://`+secrettest.PageHost+`/items/someone/copy"`) ||
		!strings.Contains(body, "/tracks/"+id+"/edit") || strings.Contains(body, "Not theirs") {
		t.Fatalf("listing = %d %s", rec.Code, body)
	}
	if got := fake.Requests(); got[len(got)-1] != secrettest.PageHost+"/items/someone" {
		t.Errorf("requests = %v", got)
	}
	if b := a.do(true, "GET", "/tracks/"+id+"/edit", nil).Body.String(); !strings.Contains(b, uploaderLink) {
		t.Error("the edit page doesn't link the uploader's page")
	}
	if b := a.do(true, "GET", "/artists/"+a.ID["secret artist"], nil).Body.String(); !strings.Contains(b, uploaderLink) {
		t.Error("the creator's page doesn't link the uploader's page")
	}
	if b := a.do(false, "GET", "/artists/"+a.ID["shared artist"], nil).Body.String(); strings.Contains(b, "Other pages from this uploader") {
		t.Error("normal mode links the uploader's page")
	}
	if b := a.do(true, "GET", uploaderLink, nil).Body.String(); !strings.Contains(b, `hx-trigger="load, submit"`) || !strings.Contains(b, `value="`+listURL+`"`) {
		t.Error("a prefilled address isn't checked on load")
	}

	// another page with the same file: the usual duplicate check refuses it
	copyURL := "https://" + secrettest.PageHost + "/items/someone/copy"
	if rec := a.do(true, "POST", "/ui/secret-source/check", url.Values{"url": {copyURL}}); rec.Code != http.StatusOK {
		t.Fatalf("check copy = %d", rec.Code)
	}
	rec = a.do(true, "POST", "/ui/secret-source/download", url.Values{"url": {copyURL}, "title": {"Copy"}, "artist_id": {a.ID["secret artist"]}})
	if rec.Code != http.StatusOK {
		t.Fatalf("download copy = %d %s", rec.Code, rec.Body)
	}
	jobs.RunOnce(context.Background(), jobs.Lane{Only: []string{downloads.JobSecretDownload}})
	if id, _ := db.TrackBySourceURL(copyURL); id != "" {
		t.Error("a duplicate file became a track")
	}
	rec = a.do(true, "GET", "/ui/secret-source/status?url="+url.QueryEscape(copyURL), nil)
	if !strings.Contains(rec.Body.String(), "failed") {
		t.Errorf("duplicate status = %s", rec.Body)
	}

	// it never shows on the Downloads page, nor does a badge count it
	body = a.do(false, "GET", "/downloads", nil).Body.String()
	if strings.Contains(strings.ToLower(body), "the title") || strings.Contains(body, "someone/one") {
		t.Error("the Downloads page lists it")
	}
}

func countJobs(typ string) (n int) {
	db.DB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = ?;`, typ).Scan(&n)
	return n
}

func TestQueuePanelInLayout(t *testing.T) {
	a := newModeApp(t)
	for _, secret := range []bool{false, true} {
		body := a.do(secret, "GET", "/tracks", nil).Body.String()
		if !strings.Contains(body, `<aside id="queue-panel" hx-preserve`) {
			t.Errorf("secret=%v: no preserved #queue-panel in the layout", secret)
		}
	}
}

func TestVaultButton(t *testing.T) {
	a := newModeApp(t)
	if body := a.do(false, "GET", "/tracks", nil).Body.String(); !strings.Contains(body, `href="/vault"`) {
		t.Error("normal mode with a vault has no Vault link")
	}
	if rec := a.do(false, "GET", "/vault", nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), `id="vault-passphrase"`) {
		t.Errorf("vault page = %d", rec.Code)
	}
	body := a.do(true, "GET", "/tracks", nil).Body.String()
	if !strings.Contains(body, `action="/vault/lock"`) {
		t.Error("secret mode has no Lock form")
	}
	if strings.Contains(body, `href="/vault"`) {
		t.Error("secret mode links to /vault")
	}

	dbtest.New(t)
	mux := http.NewServeMux()
	Register(mux, nil)
	rec := httptest.NewRecorder()
	WithViewer(mux).ServeHTTP(rec, httptest.NewRequest("GET", "/tracks", nil))
	if strings.Contains(rec.Body.String(), `href="/vault"`) {
		t.Error("normal mode without a vault links to /vault")
	}
}

func TestTagsPage(t *testing.T) {
	a := newModeApp(t)
	for _, route := range SecretModeOnly {
		method, path, _ := strings.Cut(route, " ")
		if rec := a.do(false, method, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s in normal mode = %d", route, rec.Code)
		}
	}
	if len(SecretModeOnly) != 1 {
		t.Fatalf("secret-mode routes = %v", SecretModeOnly)
	}
	body := a.do(true, "GET", "/tags", nil).Body.String()
	if !strings.Contains(body, `id="tag-lens"`) || !strings.Contains(body, `href="/tags"`) || !strings.Contains(body, "<html") {
		t.Errorf("/tags page = %q", body)
	}
	// a region swap gets just the region; a boosted visit the whole page
	req := func(h map[string]string) string {
		r := httptest.NewRequest("GET", "/tags?in=x", nil)
		r.AddCookie(a.v.Cookie(time.Now(), false))
		for k, v := range h {
			r.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		a.h.ServeHTTP(rec, r)
		return rec.Body.String()
	}
	if b := req(map[string]string{"HX-Request": "true"}); strings.Contains(b, "<html") || !strings.Contains(b, `id="tag-lens"`) {
		t.Errorf("region = %q", b)
	}
	if b := req(map[string]string{"HX-Request": "true", "HX-Boosted": "true"}); !strings.Contains(b, "<html") {
		t.Error("boosted visit got a fragment")
	}
	if !strings.Contains(a.do(true, "GET", "/tracks", nil).Body.String(), `href="/tags"`) {
		t.Error("secret nav has no Tags link")
	}
	if strings.Contains(a.do(false, "GET", "/tracks", nil).Body.String(), `href="/tags"`) {
		t.Error("normal nav links to Tags")
	}
}

func TestSecretSourceCreatorFromURL(t *testing.T) {
	a := newModeApp(t)
	fake, src := secrettest.New(t)
	secret.SetDefault(src)
	t.Cleanup(func() { secret.SetDefault(nil) })
	m4a, err := os.ReadFile("../audio/testdata/chapters.m4a")
	if err != nil {
		t.Fatal(err)
	}
	fake.Files[secrettest.MediaHost+"/sounds/one.m4a"] = m4a
	const media = `<script>x("https://media.example.test/sounds/one.m4a")</script>`
	// pages are served by their decoded path; the addresses keep it escaped
	page := func(escaped string) string {
		p, _ := url.PathUnescape(escaped)
		fake.Page(p, "<title>t</title>"+media)
		return "https://" + secrettest.PageHost + escaped
	}
	linkedURL := page("/items/linkedup/new")
	namedURL := page("/items/zzsecret%20ARTIST/new")
	freshURL := page("/items/Fresh%20Face/new")
	rootURL := page("/solo")

	// an earlier track of the secret artist came from the same uploader
	res, err := db.AddTrack(db.Secret, []string{a.ID["secret artist"]}, &audio.Track{Title: "Earlier"}, bytes.NewReader([]byte("earlier")), ".mp3", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetSourceURL(res.TrackID, "https://"+secrettest.PageHost+"/items/linkedup/old"); err != nil {
		t.Fatal(err)
	}

	check := func(u string) string {
		t.Helper()
		rec := a.do(true, "POST", "/ui/secret-source/check", url.Values{"url": {u}})
		if rec.Code != http.StatusOK {
			t.Fatalf("check %s = %d %s", u, rec.Code, rec.Body)
		}
		return rec.Body.String()
	}
	checked := func(body, id string) bool {
		return regexp.MustCompile(`value="` + id + `"[^>]*checked`).MatchString(body)
	}

	// linked by an earlier track's URL
	if body := check(linkedURL); !checked(body, a.ID["secret artist"]) || strings.Contains(body, "new_creator") {
		t.Errorf("linked creator not pre-checked: %s", body)
	}
	// matched by name, ignoring case
	if body := check(namedURL); !checked(body, a.ID["secret artist"]) || strings.Contains(body, "new_creator") {
		t.Errorf("name match not pre-checked: %s", body)
	}
	// no parent: no suggestion
	if body := check(rootURL); strings.Contains(body, " checked") || strings.Contains(body, "new_creator") {
		t.Errorf("suggestion without an uploader: %s", body)
	}
	// an unknown uploader is offered as a new creator, unescaped
	body := check(freshURL)
	if !strings.Contains(body, `name="new_creator" value="Fresh Face"`) || !strings.Contains(body, "New: Fresh Face") {
		t.Fatalf("new creator not offered: %s", body)
	}
	if _, err := db.CreatorForUploader(db.Secret, "", "Fresh Face"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.CreatorForUploader(db.Secret, "", "Fresh Face"); got != nil {
		t.Fatal("checking created the creator")
	}

	post := func(u string, extra url.Values) *httptest.ResponseRecorder {
		f := url.Values{"url": {u}, "title": {"T"}}
		for k, v := range extra {
			f[k] = v
		}
		return a.do(true, "POST", "/ui/secret-source/download", f)
	}
	// nothing picked, nothing checked
	if rec := post(freshURL, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("no creator = %d", rec.Code)
	}
	if got, _ := db.CreatorForUploader(db.Secret, "", "Fresh Face"); got != nil {
		t.Fatal("a refused download created the creator")
	}
	// submitting the new creator creates it, once
	if rec := post(freshURL, url.Values{"new_creator": {"Fresh Face"}}); rec.Code != http.StatusOK {
		t.Fatalf("download with a new creator = %d %s", rec.Code, rec.Body)
	}
	fresh, err := db.CreatorForUploader(db.Secret, "", "fresh face")
	if err != nil || fresh == nil {
		t.Fatalf("new creator = %v, %v", fresh, err)
	}
	other := page("/items/Fresh%20Face/second")
	if rec := post(other, url.Values{"new_creator": {"FRESH FACE"}}); rec.Code != http.StatusOK {
		t.Fatalf("second download = %d %s", rec.Code, rec.Body)
	}
	all, _ := db.ListArtists(db.Secret)
	n := 0
	for _, x := range all {
		if strings.EqualFold(x.Name, "Fresh Face") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d creators named Fresh Face", n)
	}
	// once it exists, it is suggested as an existing creator
	if body := check(page("/items/Fresh%20Face/third")); !checked(body, fresh.ID.String()) || strings.Contains(body, "new_creator") {
		t.Errorf("existing creator not suggested: %s", body)
	}

	// unchecking the new one and picking another creator works
	if rec := post(rootURL, url.Values{"artist_id": {a.ID["shared artist"]}}); rec.Code != http.StatusOK {
		t.Errorf("another creator = %d %s", rec.Code, rec.Body)
	}
}
