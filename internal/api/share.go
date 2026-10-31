package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"log"
	"net/http"
	"strings"

	"audioloader/internal/sources/secret"
)

// minShareToken is the shortest share token accepted; shorter ones are
// too easy to guess, so the feature stays off.
const minShareToken = 32

// theShareToken is the secret that lets a request queue secret tracks
// without the vault cookie; "" when the feature is off.
var theShareToken string

// SetShareToken sets the share token. A token shorter than minShareToken
// characters is refused (and logged), which leaves the feature off.
func SetShareToken(token string) {
	theShareToken = ""
	if token == "" {
		return
	}
	if len(token) < minShareToken {
		log.Printf("sharing is off: AUDIOLOADER_SHARE_TOKEN is shorter than %d characters", minShareToken)
		return
	}
	theShareToken = token
}

// shareEnabled is true when a token is set and what a share needs is there:
// the vault, and a secret source.
func shareEnabled() bool {
	return theShareToken != "" && theVault != nil && secret.Default() != nil
}

// shareToken tells whether the request carries the share token as a bearer
// token. The hashes are compared, so the token's length doesn't leak.
func shareToken(r *http.Request) bool {
	if theShareToken == "" {
		return false
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || got == "" {
		return false
	}
	a, b := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(theShareToken))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
