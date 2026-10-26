package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
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
		{true, `href="/tagging"`, false}, {true, `href="/downloads"`, false}, {true, `href="/tracks"`, true}, {true, `href="/shows"`, true}} {
		body := a.do(c.secret, "GET", "/tracks", nil).Body.String()
		if strings.Contains(body, c.link) != c.want {
			t.Errorf("secret=%v: %s present = %v, want %v", c.secret, c.link, !c.want, c.want)
		}
	}
}

func TestMovingTracksBetweenModes(t *testing.T) {
	a := newModeApp(t)
	// the question names the releases the track would leave
	b := a.do(false, "GET", "/tracks", nil).Body.String()
	if !strings.Contains(b, "It will leave ZZPUBLIC Album") {
		t.Error("the confirm text doesn't name the release")
	}
	id := a.ID["public track"]
	if rec := a.do(false, "POST", "/ui/tracks/"+id+"/secret", url.Values{}); rec.Code != http.StatusNoContent {
		t.Fatalf("make secret = %d", rec.Code)
	}
	tr, err := db.GetTrack(db.Secret, id)
	if err != nil || len(tr.Releases) != 0 {
		t.Fatalf("moved track = %+v, %v", tr, err)
	}
	// the same request from normal mode no longer finds it
	if rec := a.do(false, "POST", "/ui/tracks/"+id+"/secret", url.Values{}); rec.Code != http.StatusNotFound {
		t.Errorf("second move = %d", rec.Code)
	}
	if rec := a.do(true, "POST", "/ui/tracks/"+id+"/secret", url.Values{}); rec.Code != http.StatusNoContent {
		t.Fatalf("make public = %d", rec.Code)
	}
	if _, err := db.GetTrack(db.Normal, id); err != nil {
		t.Fatalf("back in normal mode: %v", err)
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
		{"/upload", []string{`name="date"`, `name="secret"`, "From YouTube", "new-release-form"}, []string{`name="description"`, `name="tags"`, "Podcast episode"}},
		{"/upload/batch", []string{`name="secret"`, "Release for all tracks", "Disc</span>"}, []string{"Creators"}},
		{"/upload/import", []string{`import-secret`}, []string{"Each file becomes a secret track"}},
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
