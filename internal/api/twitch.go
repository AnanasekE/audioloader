package api

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"uuid"

	"audioloader/internal/db"
	"audioloader/internal/twitch"
)

// twitchClient returns the Twitch client, nil when it isn't configured.
// Tests replace it.
var twitchClient = twitch.Default

var twitchLoginPattern = regexp.MustCompile(`^[a-z0-9_]{3,25}$`)

// parseTwitchLogin reads a channel from a login or a twitch.tv/<login> URL
// and returns it lowercase; "" if it is neither or isn't a valid login.
func parseTwitchLogin(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if strings.Contains(text, "/") || strings.Contains(text, ".") {
		if !strings.Contains(text, "://") {
			text = "https://" + text
		}
		u, err := url.Parse(text)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return ""
		}
		if h := u.Hostname(); h != "twitch.tv" && h != "www.twitch.tv" && h != "m.twitch.tv" {
			return ""
		}
		text, _, _ = strings.Cut(strings.Trim(u.Path, "/"), "/")
	}
	if !twitchLoginPattern.MatchString(text) {
		return ""
	}
	return text
}

// Why a picture couldn't be fetched, in words for the artist page.
var (
	errTwitchOff     = errors.New("Twitch isn't set up: set TWITCH_CLIENT_ID and TWITCH_CLIENT_SECRET.")
	errTwitchUnknown = errors.New("Twitch has no channel with that login.")
	errTwitchFailed  = errors.New("Couldn't fetch the picture from Twitch.")
)

// fetchTwitchPicture replaces the artist's picture with the channel's
// current profile picture. The error is one of the messages above, fit to
// show; the picture is left alone on any error.
func fetchTwitchPicture(ctx context.Context, artistID uuid.UUID, login string) error {
	tc := twitchClient()
	if tc == nil {
		return errTwitchOff
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	user, err := tc.User(ctx, login)
	if errors.Is(err, twitch.ErrUnknownChannel) {
		return errTwitchUnknown
	}
	var data []byte
	if err == nil {
		data, err = tc.Image(ctx, user.ProfileImageURL)
	}
	if err == nil {
		err = db.SaveArtistImage(artistID, bytes.NewReader(data))
	}
	if err != nil {
		log.Println("twitch picture:", err)
		return errTwitchFailed
	}
	return nil
}

// setArtistTwitch saves the artist's Twitch channel from "twitch" (a login
// or URL; empty removes it). A new channel gives an artist who still has the
// default picture the channel's picture, if Twitch is set up.
func setArtistTwitch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok || !visibleArtist(w, r, id.String()) {
		return
	}
	login := ""
	if text := strings.TrimSpace(r.FormValue("twitch")); text != "" {
		if login = parseTwitchLogin(text); login == "" {
			http.Error(w, "that isn't a Twitch login (3 to 25 letters, digits or underscores) or channel URL", http.StatusBadRequest)
			return
		}
	}
	a, err := db.GetArtist(viewer(r), id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	if err := db.SetArtistTwitch(id.String(), login); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		serverError(w, err)
		return
	}
	// an uploaded or earlier fetched picture is served from /artist-images/
	hasDefaultPicture := !strings.HasPrefix(a.ImgURL, "/artist-images/")
	if login != "" && hasDefaultPicture && twitchClient() != nil {
		if err := fetchTwitchPicture(r.Context(), id, login); err != nil {
			// the login is saved; only the picture failed
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("Saved. " + err.Error()))
			return
		}
	}
	hxLocation(w, "/artists/"+id.String())
}

// updateArtistTwitchPicture replaces the picture with the saved channel's current one.
func updateArtistTwitchPicture(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "artist_id")
	if !ok || !visibleArtist(w, r, id.String()) {
		return
	}
	a, err := db.GetArtist(viewer(r), id.String())
	if err != nil {
		serverError(w, err)
		return
	}
	if a.TwitchLogin == "" {
		http.Error(w, "save a Twitch channel first", http.StatusBadRequest)
		return
	}
	switch err := fetchTwitchPicture(r.Context(), id, a.TwitchLogin); {
	case errors.Is(err, errTwitchOff), errors.Is(err, errTwitchUnknown):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	hxLocation(w, "/artists/"+id.String())
}
