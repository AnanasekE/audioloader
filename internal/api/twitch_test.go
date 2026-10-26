package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/twitch"
)

func TestParseTwitchLogin(t *testing.T) {
	for in, want := range map[string]string{
		"CottontailVA":                         "cottontailva",
		"  some_user1 ":                        "some_user1",
		"https://www.twitch.tv/CottontailVA":   "cottontailva",
		"twitch.tv/cottontailva/videos":        "cottontailva",
		"https://m.twitch.tv/cottontailva?x=1": "cottontailva",
		"https://evil.example/cottontailva":    "",
		"https://twitch.tv.evil.example/abc":   "",
		"https://twitch.tv/":                   "",
		"ab":                                   "",
		"has space":                            "",
		"toolongtoolongtoolongtoolong":         "",
		"bad-dash":                             "",
	} {
		if got := parseTwitchLogin(in); got != want {
			t.Errorf("parseTwitchLogin(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeTwitch serves Twitch's auth, Helix and image hosts and makes it the
// API's client for the test. The picture served for "known" is pic.
func fakeTwitch(t *testing.T, pic *atomic.Value) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/id/oauth2/token":
			w.Write([]byte(`{"access_token": "tok", "expires_in": 3600}`))
		case "/helix/users":
			if r.URL.Query().Get("login") == "known" {
				w.Write([]byte(`{"data": [{"login": "known", "profile_image_url": "http://` + r.Host + `/pic.png"}]}`))
				return
			}
			w.Write([]byte(`{"data": []}`))
		case "/pic.png":
			w.Write(pic.Load().([]byte))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	c := twitch.NewWith("cid", "secret")
	c.AuthURL, c.APIURL, c.ImageHost, c.ImageScheme = srv.URL+"/id", srv.URL+"/helix", u.Host, "http"
	old := twitchClient
	twitchClient = func() *twitch.Client { return c }
	t.Cleanup(func() { twitchClient = old })
}

var (
	pngA = []byte("\x89PNG\r\n\x1a\n0000A")
	pngB = []byte("\x89PNG\r\n\x1a\n0000B")
)

func TestArtistTwitch(t *testing.T) {
	dbtest.New(t)
	var pic atomic.Value
	pic.Store(pngA)
	fakeTwitch(t, &pic)
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)

	a, _ := db.AddArtist("Cottontail")
	path := "/ui/artists/" + a.ID.String() + "/twitch"
	current := func() *struct{ login, img string } {
		got, _ := db.GetArtist(db.All, a.ID.String())
		return &struct{ login, img string }{got.TwitchLogin, got.ImgURL}
	}

	if rec := postForm(h, path, "twitch=not+valid!"); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid = %d", rec.Code)
	}

	// an unknown channel is saved, the picture stays the default
	rec := postForm(h, path, "twitch=https://twitch.tv/Nobody")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "no channel") {
		t.Fatalf("unknown = %d %s", rec.Code, rec.Body.String())
	}
	if c := current(); c.login != "nobody" || strings.HasPrefix(c.img, "/artist-images/") {
		t.Fatalf("after unknown: %+v", c)
	}

	// a known one fetches the picture for an artist with the default one
	if rec := postForm(h, path, "twitch=Known"); rec.Code != http.StatusNoContent {
		t.Fatalf("known = %d %s", rec.Code, rec.Body.String())
	}
	c := current()
	if c.login != "known" || !strings.HasPrefix(c.img, "/artist-images/") {
		t.Fatalf("after known: %+v", c)
	}
	file, err := db.ArtistImagePath(a.ID)
	if err != nil || !fileHas(t, file, pngA) {
		t.Fatalf("picture file %q, %v", file, err)
	}

	// saving again leaves the picture alone, even when Twitch has a new one
	pic.Store(pngB)
	if rec := postForm(h, path, "twitch=known"); rec.Code != http.StatusNoContent {
		t.Fatalf("save again = %d", rec.Code)
	}
	if file, _ := db.ArtistImagePath(a.ID); !fileHas(t, file, pngA) {
		t.Fatal("saving replaced the picture")
	}
	// only the button does
	if rec := postForm(h, path+"/picture", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	if file, _ := db.ArtistImagePath(a.ID); !fileHas(t, file, pngB) {
		t.Fatal("update kept the old picture")
	}

	// a failed update keeps the picture
	postForm(h, path, "twitch=nobody")
	if rec := postForm(h, path+"/picture", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("update unknown = %d", rec.Code)
	}
	if file, _ := db.ArtistImagePath(a.ID); !fileHas(t, file, pngB) {
		t.Fatal("failed update changed the picture")
	}

	// empty removes the channel
	if rec := postForm(h, path, "twitch="); rec.Code != http.StatusNoContent || current().login != "" {
		t.Fatalf("remove = %d, %+v", rec.Code, current())
	}
}

func TestArtistTwitchWithoutClient(t *testing.T) {
	dbtest.New(t)
	old := twitchClient
	twitchClient = func() *twitch.Client { return nil }
	defer func() { twitchClient = old }()
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)

	a, _ := db.AddArtist("Cottontail")
	// the field can still be saved
	if rec := postForm(h, "/ui/artists/"+a.ID.String()+"/twitch", "twitch=known"); rec.Code != http.StatusNoContent {
		t.Fatalf("save = %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := db.GetArtist(db.All, a.ID.String()); got.TwitchLogin != "known" {
		t.Fatalf("login = %q", got.TwitchLogin)
	}
	if rec := postForm(h, "/ui/artists/"+a.ID.String()+"/twitch/picture", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("update = %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/artists/"+a.ID.String(), nil))
	if !strings.Contains(rec.Body.String(), "https://www.twitch.tv/known") || !strings.Contains(rec.Body.String(), "TWITCH_CLIENT_ID") {
		t.Fatalf("page = %d", rec.Code)
	}
}

func TestTwitchHiddenWhileLocked(t *testing.T) {
	dbtest.New(t)
	ids := secretLibrary(t)
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)
	for _, p := range []string{"/twitch", "/twitch/picture", "/link"} {
		if rec := postForm(h, "/ui/artists/"+ids["hidden artist"]+p, "twitch=known&mbid=10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s = %d", p, rec.Code)
		}
	}
}
