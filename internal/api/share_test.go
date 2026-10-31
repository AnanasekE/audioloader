package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"audioloader/internal/dbtest"
	"audioloader/internal/sources/secret"
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
