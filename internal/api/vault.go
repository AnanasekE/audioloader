package api

import (
	"errors"
	"net/http"
	"time"

	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/vault"
)

// theVault unlocks secret tracks; nil when the feature is off.
var theVault *vault.Vault

// WithViewer puts the request's viewer into its context: unlocked when it
// carries a valid vault cookie.
func WithViewer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := db.Normal
		if theVault.Valid(r, time.Now()) {
			v = db.Secret
		}
		next.ServeHTTP(w, r.WithContext(db.WithViewer(r.Context(), v)))
	})
}

func getVaultPage(w http.ResponseWriter, r *http.Request) {
	if theVault == nil {
		http.NotFound(w, r)
		return
	}
	render(w, r, components.VaultPage(r.URL.Query().Has("wrong")))
}

func unlockVault(w http.ResponseWriter, r *http.Request) {
	if theVault == nil {
		http.NotFound(w, r)
		return
	}
	if !theVault.Check(r.FormValue("passphrase")) {
		// slows down guessing, on top of bcrypt's cost
		time.Sleep(time.Second)
		http.Redirect(w, r, "/vault?wrong", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, theVault.Cookie(time.Now(), r.TLS != nil))
	// a full page load, so the player and pages start over unlocked
	http.Redirect(w, r, "/tracks", http.StatusSeeOther)
}

func lockVault(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, vault.ClearCookie())
	http.Redirect(w, r, "/tracks", http.StatusSeeOther)
}

// setTrackSecret moves a track to the other mode, see db.SetSecret. A viewer
// moves only tracks of the mode they are in, so a missing vault, or a track
// that is already there, is a 404.
func setTrackSecret(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}
	if theVault == nil || !visibleTrack(w, r, id.String()) {
		if theVault == nil {
			http.NotFound(w, r)
		}
		return
	}
	err := db.SetSecret(id.String(), !viewer(r).IsSecret())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, backTo(r, "/tracks"))
}

// visibleTrack writes a 404 and returns false if the viewer can't see the
// track, so actions on secret tracks look like actions on missing ones.
func visibleTrack(w http.ResponseWriter, r *http.Request, id string) bool {
	_, err := db.GetTrack(viewer(r), id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return false
	}
	if err != nil {
		serverError(w, err)
		return false
	}
	return true
}
