// Package vault unlocks secret tracks. A viewer who enters the passphrase
// (its bcrypt hash is AUDIOLOADER_SECRET) gets a signed cookie; requests
// carrying a valid one see secret tracks. Without AUDIOLOADER_SECRET the
// feature is off and nothing is ever unlocked.
package vault

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// CookieName is the vault session cookie.
const CookieName = "al_vault"

// Lifetime is how long an unlock lasts.
const Lifetime = 12 * time.Hour

// Vault checks passphrases and signs and verifies session cookies.
type Vault struct {
	hash []byte // bcrypt hash of the passphrase
	key  []byte // HMAC key for cookies
}

// FromEnv returns the vault configured by AUDIOLOADER_SECRET, with its
// signing key in keyPath (created on first use), or nil when the feature
// is off.
func FromEnv(keyPath string) (*Vault, error) {
	hash := os.Getenv("AUDIOLOADER_SECRET")
	if hash == "" {
		return nil, nil
	}
	if _, err := bcrypt.Cost([]byte(hash)); err != nil {
		return nil, errors.New("AUDIOLOADER_SECRET must be a bcrypt hash; make one with: audioloader -hash-secret")
	}
	key, err := loadKey(keyPath)
	if err != nil {
		return nil, err
	}
	return &Vault{hash: []byte(hash), key: key}, nil
}

// New makes a vault from a bcrypt hash and key, for tests.
func New(hash, key []byte) *Vault { return &Vault{hash: hash, key: key} }

// Hash returns the bcrypt hash of a passphrase, for AUDIOLOADER_SECRET.
func Hash(passphrase string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(passphrase), bcrypt.DefaultCost)
	return string(h), err
}

func loadKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil && len(key) >= 32 {
		return key, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return key, os.WriteFile(path, key, 0o600)
}

// Check reports whether passphrase is the vault's.
func (v *Vault) Check(passphrase string) bool {
	return bcrypt.CompareHashAndPassword(v.hash, []byte(passphrase)) == nil
}

func (v *Vault) sign(payload string) string {
	m := hmac.New(sha256.New, v.key)
	m.Write([]byte(payload))
	return hex.EncodeToString(m.Sum(nil))
}

// Cookie returns a session cookie valid until now+Lifetime.
func (v *Vault) Cookie(now time.Time, secure bool) *http.Cookie {
	exp := strconv.FormatInt(now.Add(Lifetime).Unix(), 10)
	return &http.Cookie{
		Name:     CookieName,
		Value:    exp + "|" + v.sign(exp),
		Path:     "/",
		Expires:  now.Add(Lifetime),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
}

// Valid reports whether the request carries an unexpired, untampered session.
func (v *Vault) Valid(r *http.Request, now time.Time) bool {
	if v == nil {
		return false
	}
	c, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	exp, sig, ok := strings.Cut(c.Value, "|")
	if !ok || !hmac.Equal([]byte(sig), []byte(v.sign(exp))) {
		return false
	}
	unix, err := strconv.ParseInt(exp, 10, 64)
	return err == nil && now.Unix() < unix
}

// ClearCookie expires the session cookie.
func ClearCookie() *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode}
}
