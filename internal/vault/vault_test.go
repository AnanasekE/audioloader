package vault

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestVault(t *testing.T) {
	hash, err := Hash("open sesame")
	if err != nil {
		t.Fatal(err)
	}
	v := New([]byte(hash), []byte("0123456789abcdef0123456789abcdef"))
	if !v.Check("open sesame") || v.Check("wrong") {
		t.Fatal("passphrase check")
	}

	now := time.Now()
	c := v.Cookie(now, false)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(c)
	if !v.Valid(req, now) {
		t.Fatal("fresh cookie rejected")
	}
	if v.Valid(req, now.Add(Lifetime+time.Second)) {
		t.Fatal("expired cookie accepted")
	}

	tampered := *c
	tampered.Value = "9999999999|" + c.Value[len("9999999999|"):]
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&tampered)
	if v.Valid(req, now) {
		t.Fatal("tampered expiry accepted")
	}

	other := New([]byte(hash), []byte("another key, another key, another"))
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(c)
	if other.Valid(req, now) {
		t.Fatal("cookie signed with another key accepted")
	}

	var off *Vault
	if off.Valid(req, now) {
		t.Fatal("disabled vault unlocked")
	}
}

func TestFromEnvOff(t *testing.T) {
	t.Setenv("AUDIOLOADER_SECRET", "")
	v, err := FromEnv(t.TempDir() + "/key")
	if v != nil || err != nil {
		t.Fatalf("v = %v, err = %v", v, err)
	}
	t.Setenv("AUDIOLOADER_SECRET", "not a hash")
	if _, err := FromEnv(t.TempDir() + "/key"); err == nil {
		t.Fatal("accepted a plain passphrase")
	}
}
