package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"audioloader/internal/dbtest"
	"audioloader/internal/vault"
)

// v1Server is the app with a vault whose passphrase is "pass", or without
// one. get and send answer as the normal viewer unless secret is set.
type v1Server struct {
	t      *testing.T
	h      http.Handler
	vault  *vault.Vault
	secret bool
}

func newV1(t *testing.T, withVault bool) *v1Server {
	t.Helper()
	dbtest.New(t)
	s := &v1Server{t: t}
	if withVault {
		hash, _ := vault.Hash("pass")
		s.vault = vault.New([]byte(hash), []byte("0123456789abcdef0123456789abcdef"))
	}
	mux := http.NewServeMux()
	Register(mux, s.vault)
	s.h = WithViewer(mux)
	return s
}

func (s *v1Server) do(method, u, ctype, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, u, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if s.secret {
		req.AddCookie(s.vault.Cookie(time.Now(), false))
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	return rec
}

func (s *v1Server) get(u string) *httptest.ResponseRecorder {
	return s.do("GET", u, "", "")
}

// json decodes a 200 JSON answer into a generic map or fails.
func (s *v1Server) json(u string) map[string]any {
	s.t.Helper()
	rec := s.get(u)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		s.t.Fatalf("GET %s = %d %s %q", u, rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		s.t.Fatalf("GET %s: %v in %s", u, err, rec.Body.String())
	}
	return out
}

func TestV1Me(t *testing.T) {
	s := newV1(t, true)
	if got := s.json("/api/v1/me"); got["secret"] != false || got["vault"] != true || got["secretSource"] != false {
		t.Fatalf("normal mode: %v", got)
	}
	s.secret = true
	if got := s.json("/api/v1/me"); got["secret"] != true || got["vault"] != true {
		t.Fatalf("secret mode: %v", got)
	}

	off := newV1(t, false)
	if got := off.json("/api/v1/me"); got["secret"] != false || got["vault"] != false {
		t.Fatalf("vault off: %v", got)
	}
}

func TestV1Vault(t *testing.T) {
	s := newV1(t, true)
	const jsonType = "application/json"
	const formType = "application/x-www-form-urlencoded"

	rec := s.do("POST", "/api/v1/vault", jsonType, `{"passphrase": "pass"}`)
	if rec.Code != http.StatusNoContent || len(rec.Result().Cookies()) != 1 {
		t.Fatalf("unlock = %d, cookies %v", rec.Code, rec.Result().Cookies())
	}
	// the cookie it sets unlocks secret mode
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.AddCookie(rec.Result().Cookies()[0])
	me := httptest.NewRecorder()
	s.h.ServeHTTP(me, req)
	if !strings.Contains(me.Body.String(), `"secret":true`) {
		t.Fatalf("me with the cookie: %s", me.Body.String())
	}
	if rec := s.do("POST", "/api/v1/vault", formType, "passphrase=pass"); rec.Code != http.StatusNoContent {
		t.Fatalf("form unlock = %d", rec.Code)
	}

	start := time.Now()
	rec = s.do("POST", "/api/v1/vault", jsonType, `{"passphrase": "nope"}`)
	if rec.Code != http.StatusUnauthorized || len(rec.Result().Cookies()) != 0 || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("wrong passphrase = %d %s %v", rec.Code, rec.Body.String(), rec.Result().Cookies())
	}
	if time.Since(start) < time.Second {
		t.Error("wrong passphrase wasn't slowed down")
	}
	if rec := s.do("POST", "/api/v1/vault", jsonType, `{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad JSON = %d", rec.Code)
	}

	rec = s.do("POST", "/api/v1/vault/lock", "", "")
	cookies := rec.Result().Cookies()
	if rec.Code != http.StatusNoContent || len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatalf("lock = %d, cookies %v", rec.Code, cookies)
	}

	off := newV1(t, false)
	if rec := off.do("POST", "/api/v1/vault", jsonType, `{"passphrase": "pass"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unlock with the vault off = %d", rec.Code)
	}
}

func TestPageParams(t *testing.T) {
	for _, c := range []struct {
		query         string
		limit, offset int
	}{
		{"", 100, 0}, {"limit=20&offset=5", 20, 5}, {"limit=9999", 500, 0},
		{"limit=0&offset=-3", 100, 0}, {"limit=x&offset=y", 100, 0},
	} {
		limit, offset := pageParams(httptest.NewRequest("GET", "/?"+c.query, nil))
		if limit != c.limit || offset != c.offset {
			t.Errorf("%q = %d, %d; want %d, %d", c.query, limit, offset, c.limit, c.offset)
		}
	}
}
