package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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
