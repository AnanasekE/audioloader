package api

import (
	"encoding/json"
	"mime"
	"net/http"
	"strconv"
	"time"

	"audioloader/internal/sources/secret"
	"audioloader/internal/vault"
)

// The JSON API for the mobile app, under /api/v1/. See docs/api.md.

// Paging of list endpoints: ?limit= (default 100, at most 500) and ?offset=.
const (
	defaultLimit = 100
	maxLimit     = 500
)

// registerV1 adds the /api/v1/ routes; see the table in docs/api.md.
func registerV1(mux *http.ServeMux) {
	handle(mux, "GET /api/v1/me", getMe)
	handle(mux, "POST /api/v1/vault", unlockVaultJSON)
	handle(mux, "POST /api/v1/vault/lock", lockVaultJSON)
	handle(mux, "GET /api/v1/tracks", getTracksJSON)
	handle(mux, "GET /api/v1/tracks/{track_id}", getTrackJSON)
	handle(mux, "GET /api/v1/search", getSearchJSON)
}

// writeJSON sends v as JSON with the given status. It encodes into a buffer
// first, like render, so a failure is a clean 500.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(append(body, '\n'))
}

// jsonError sends {"error": msg}. Internal errors go through serverError
// instead, which logs them and keeps the details out of the response.
func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// pageParams reads ?limit= and ?offset=; bad or out-of-range values fall back
// to the default limit, the maximum, or zero.
func pageParams(r *http.Request) (limit, offset int) {
	limit = defaultLimit
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = min(n, maxLimit)
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && n > 0 {
		offset = n
	}
	return limit, offset
}

// getMe tells the app which mode it is in and what to show. secretSource is
// true only in secret mode, like the page for it.
func getMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Secret       bool `json:"secret"`
		Vault        bool `json:"vault"`
		SecretSource bool `json:"secretSource"`
	}{viewer(r).IsSecret(), theVault != nil, viewer(r).IsSecret() && secret.Default() != nil})
}

// unlockVaultJSON is unlockVault for the app: the passphrase comes as JSON
// ({"passphrase": "..."}) or a form, and the answers are status codes.
func unlockVaultJSON(w http.ResponseWriter, r *http.Request) {
	if theVault == nil {
		http.NotFound(w, r)
		return
	}
	var pass string
	if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct == "application/json" {
		var req struct {
			Passphrase string `json:"passphrase"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		pass = req.Passphrase
	} else {
		pass = r.FormValue("passphrase")
	}
	if !checkPassphrase(pass) {
		jsonError(w, http.StatusUnauthorized, "wrong passphrase")
		return
	}
	http.SetCookie(w, theVault.Cookie(time.Now(), r.TLS != nil))
	w.WriteHeader(http.StatusNoContent)
}

func lockVaultJSON(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, vault.ClearCookie())
	w.WriteHeader(http.StatusNoContent)
}
