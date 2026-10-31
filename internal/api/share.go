package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"audioloader/internal/db"
	"audioloader/internal/downloads"
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

// shareFailDelay slows down guessing of the token.
var shareFailDelay = time.Second

// shareLimit is how many shares a minute are accepted in total.
const shareLimit = 30

// shareBucket is a token bucket that refills shareLimit tokens a minute.
var shareBucket = struct {
	sync.Mutex
	tokens float64
	at     time.Time
}{tokens: shareLimit}

func allowShare() bool {
	b := &shareBucket
	b.Lock()
	defer b.Unlock()
	now := time.Now()
	if !b.at.IsZero() {
		b.tokens = min(shareLimit, b.tokens+now.Sub(b.at).Minutes()*shareLimit)
	}
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// shareURL reads the page URL of a share: a form field, or a JSON body.
func shareURL(r *http.Request) string {
	r.Body = http.MaxBytesReader(nil, r.Body, 8<<10)
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "application/json" {
		var body struct {
			URL string `json:"url"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			return ""
		}
		return body.URL
	}
	return r.FormValue("url")
}

// publicBase is the address the app is reached at: AUDIOLOADER_PUBLIC_URL,
// else built from the request.
func publicBase(r *http.Request) string {
	if u := strings.TrimRight(os.Getenv("AUDIOLOADER_PUBLIC_URL"), "/"); u != "" {
		return u
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// cut shortens s to at most n bytes without splitting a character.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// shareSecretPage does what "Download from URL" does for a page, as the
// secret viewer, with the page's own defaults. It answers in one line of
// plain text, for a notification.
func shareSecretPage(w http.ResponseWriter, r *http.Request) {
	if !allowShare() {
		http.Error(w, "Too many shares; try again in a minute.", http.StatusTooManyRequests)
		return
	}
	src := secret.Default()
	pageURL, err := src.Normalize(shareURL(r))
	if err != nil {
		secretSourceError(w, err)
		return
	}
	if t, err := existingSecretTrack(r, pageURL); err != nil {
		serverError(w, err)
		return
	} else if t != nil {
		http.Error(w, "Already in your library: "+t.Title, http.StatusConflict)
		return
	}
	page, listing, err := src.Look(r.Context(), pageURL)
	if err != nil {
		secretSourceError(w, err)
		return
	}
	if listing != nil {
		http.Error(w, "That's a list of recordings; share a single recording.", http.StatusUnprocessableEntity)
		return
	}
	name := src.Uploader(page.URL)
	if name == "" || len([]rune(name)) > maxCreatorName {
		http.Error(w, "No creator in that address. Finish it in the app:\n"+
			publicBase(r)+"/upload/secret-source?url="+url.QueryEscape(page.URL), http.StatusUnprocessableEntity)
		return
	}
	creator, err := db.CreatorForUploader(viewer(r), src.Parent(page.URL), name)
	if err == nil && creator == nil {
		creator, err = creatorForName(viewer(r), name)
	}
	if err != nil {
		serverError(w, err)
		return
	}
	title := strings.TrimSpace(page.Title)
	if title == "" {
		last, _ := url.PathUnescape(path.Base(page.URL))
		title = strings.TrimSpace(last)
	}
	if runes := []rune(title); len(runes) > 200 {
		title = string(runes[:200])
	}
	err = downloads.EnqueueSecret(downloads.SecretImport{
		PageURL:     page.URL,
		AudioURL:    page.AudioURL,
		Title:       title,
		Description: cut(strings.TrimSpace(page.Description), maxDescription),
		Tags:        page.Tags,
		ArtistIDs:   []string{creator.ID.String()},
	})
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, "Queued: %s by %s\n", title, creator.Name)
}
