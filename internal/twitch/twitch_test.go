package twitch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// png is the smallest file http.DetectContentType calls image/png.
var png = []byte("\x89PNG\r\n\x1a\n0000")

// fakeTwitch stands in for both Twitch hosts and the image CDN.
type fakeTwitch struct {
	tokens   atomic.Int32
	lookups  atomic.Int32
	validTok atomic.Value // the token Helix accepts
	expires  int          // expires_in of new tokens
	client   *Client
	now      time.Time
}

func newFake(t *testing.T) *fakeTwitch {
	t.Helper()
	f := &fakeTwitch{expires: 3600, now: time.Unix(1_000_000, 0)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/id/oauth2/token":
			r.ParseForm()
			if r.Method != "POST" || r.PostForm.Get("grant_type") != "client_credentials" ||
				r.PostForm.Get("client_id") != "cid" || r.PostForm.Get("client_secret") != "secret" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			n := f.tokens.Add(1)
			tok := "token" + strconv.Itoa(int(n))
			f.validTok.Store(tok)
			w.Write([]byte(`{"access_token": "` + tok + `", "expires_in": ` + strconv.Itoa(f.expires) + `}`))
		case r.URL.Path == "/helix/users":
			f.lookups.Add(1)
			if r.Header.Get("Client-Id") != "cid" || r.Header.Get("Authorization") != "Bearer "+f.validTok.Load().(string) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			switch r.URL.Query().Get("login") {
			case "ninja":
				w.Write([]byte(`{"data": [{"login": "ninja", "display_name": "Ninja", "profile_image_url": "http://` + r.Host + `/img/ok.png"}]}`))
			case "evil":
				w.Write([]byte(`{"data": [{"login": "evil", "profile_image_url": "http://other.example/x.png"}]}`))
			default:
				w.Write([]byte(`{"data": []}`))
			}
		case r.URL.Path == "/img/ok.png":
			w.Write(png)
		case r.URL.Path == "/img/big.png":
			w.Write(append(append([]byte{}, png...), make([]byte, MaxImageSize)...))
		case r.URL.Path == "/img/text.png":
			w.Write([]byte("<html>not an image</html>"))
		case r.URL.Path == "/img/redirect.png":
			http.Redirect(w, r, "http://other.example/x.png", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	f.validTok.Store("")
	u, _ := url.Parse(srv.URL)
	c := NewWith("cid", "secret")
	c.AuthURL, c.APIURL = srv.URL+"/id", srv.URL+"/helix"
	c.ImageHost, c.ImageScheme = u.Host, "http"
	c.Now = func() time.Time { return f.now }
	f.client = c
	return f
}

func TestUserAndImage(t *testing.T) {
	f := newFake(t)
	ctx := context.Background()
	u, err := f.client.User(ctx, "ninja")
	if err != nil || u.Login != "ninja" || u.DisplayName != "Ninja" {
		t.Fatalf("user = %+v, %v", u, err)
	}
	data, err := f.client.Image(ctx, u.ProfileImageURL)
	if err != nil || len(data) != len(png) {
		t.Fatalf("image = %d bytes, %v", len(data), err)
	}
}

func TestUnknownChannel(t *testing.T) {
	f := newFake(t)
	if _, err := f.client.User(context.Background(), "nobody"); !errors.Is(err, ErrUnknownChannel) {
		t.Fatalf("err = %v", err)
	}
}

func TestTokenCachedUntilExpiry(t *testing.T) {
	f := newFake(t)
	ctx := context.Background()
	for range 3 {
		if _, err := f.client.User(ctx, "ninja"); err != nil {
			t.Fatal(err)
		}
	}
	if f.tokens.Load() != 1 {
		t.Fatalf("token requests = %d, want 1", f.tokens.Load())
	}
	// inside the safety margin before expiry the token is replaced
	f.now = f.now.Add(time.Hour - 30*time.Second)
	if _, err := f.client.User(ctx, "ninja"); err != nil {
		t.Fatal(err)
	}
	if f.tokens.Load() != 2 {
		t.Fatalf("token requests after expiry = %d, want 2", f.tokens.Load())
	}
}

func TestRevokedTokenRetriedOnce(t *testing.T) {
	f := newFake(t)
	ctx := context.Background()
	if _, err := f.client.User(ctx, "ninja"); err != nil {
		t.Fatal(err)
	}
	// the token stops working: the next lookup gets a 401, then works with a new one
	f.validTok.Store("revoked-elsewhere")
	if _, err := f.client.User(ctx, "ninja"); err != nil {
		t.Fatal(err)
	}
	if f.tokens.Load() != 2 || f.lookups.Load() != 3 {
		t.Fatalf("tokens = %d, lookups = %d", f.tokens.Load(), f.lookups.Load())
	}
}

func TestAlwaysUnauthorizedGivesUp(t *testing.T) {
	f := newFake(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			w.Write([]byte(`{"access_token": "t", "expires_in": 3600}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	f.client.AuthURL, f.client.APIURL = srv.URL, srv.URL
	if _, err := f.client.User(context.Background(), "ninja"); err == nil || errors.Is(err, ErrUnknownChannel) {
		t.Fatalf("err = %v", err)
	}
}

func TestImageIsCheckedBeforeAndAfterDownload(t *testing.T) {
	f := newFake(t)
	ctx := context.Background()
	host := f.client.ImageHost
	for name, raw := range map[string]string{
		"other host":    "http://other.example/x.png",
		"wrong scheme":  "https://" + host + "/img/ok.png",
		"host suffix":   "http://" + host + ".evil.example/img/ok.png",
		"too large":     "http://" + host + "/img/big.png",
		"not an image":  "http://" + host + "/img/text.png",
		"redirect away": "http://" + host + "/img/redirect.png",
		"missing":       "http://" + host + "/img/none.png",
	} {
		if _, err := f.client.Image(ctx, raw); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestNoClientWithoutCredentials(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "id")
	t.Setenv("TWITCH_CLIENT_SECRET", "")
	if New() != nil {
		t.Fatal("client without a secret")
	}
	t.Setenv("TWITCH_CLIENT_SECRET", "s")
	if New() == nil {
		t.Fatal("no client with both")
	}
}
