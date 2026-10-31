package api

import (
	"net/http"
	"net/url"
	"strings"

	"audioloader/internal/db"
	"audioloader/internal/mb"
	"audioloader/internal/sources/secret"
	"audioloader/internal/sources/ytdlp"
)

// addTarget is where the Add page's box sends what was typed.
type addTarget int

const (
	addSearch       addTarget = iota // text: MusicBrainz song search, then YouTube
	addMusicBrainz                   // a musicbrainz.org link
	addYouTube                       // a YouTube or YouTube Music video or playlist link
	addSecretSource                  // a link the secret source takes (secret mode only)
	addRefused                       // answer 400 with the route's Message
)

// addRoute is the result of classifying the box: where to send Value, or
// why not.
type addRoute struct {
	Target  addTarget
	Value   string // the text, or the link with a scheme
	Message string // for addRefused
}

const (
	msgBadSite       = "Audioloader can't add from that site."
	msgSecretNoText  = "Paste a link from the secret source, or upload a file."
	msgSecretNoLinks = "Upload a file."
)

// classifyAddInput decides what the box's q is for viewer v. A secret source
// link only counts in secret mode; in normal mode it is any unknown site,
// so the page never says the source exists.
func classifyAddInput(v db.Viewer, q string) addRoute {
	q = strings.TrimSpace(q)
	link, host, isLink := parseAddLink(q)
	if !isLink {
		if v.IsSecret() {
			if secret.Default() == nil {
				return addRoute{Target: addRefused, Message: msgSecretNoLinks}
			}
			return addRoute{Target: addRefused, Message: msgSecretNoText}
		}
		return addRoute{Target: addSearch, Value: q}
	}
	if v.IsSecret() {
		if src := secret.Default(); src != nil {
			if _, err := src.Normalize(link); err == nil {
				return addRoute{Target: addSecretSource, Value: link}
			}
		}
		return addRoute{Target: addRefused, Message: msgBadSite}
	}
	if host == "musicbrainz.org" || strings.HasSuffix(host, ".musicbrainz.org") {
		return addRoute{Target: addMusicBrainz, Value: link}
	}
	if _, err := ytdlp.Parse(link); err == nil {
		return addRoute{Target: addYouTube, Value: link}
	}
	return addRoute{Target: addRefused, Message: msgBadSite}
}

// parseAddLink tells a link from search text: no spaces, and either a scheme
// or a host with a dot followed by a path ("AC/DC" and "St.Vincent" are
// text). link has a scheme and host is the lowercase host without "www." or "m.".
func parseAddLink(q string) (link, host string, ok bool) {
	if q == "" || strings.ContainsAny(q, " \t\r\n") {
		return "", "", false
	}
	link = q
	if !strings.Contains(q, "://") {
		slash := strings.IndexAny(q, "/?#")
		if slash < 0 || !strings.Contains(q[:slash], ".") {
			return "", "", false
		}
		link = "https://" + q
	}
	u, err := url.Parse(link)
	if err != nil || u.Hostname() == "" {
		return link, "", true // still a link; the target will refuse it
	}
	host = strings.ToLower(u.Hostname())
	if u.Scheme != "http" && u.Scheme != "https" {
		return link, "", true
	}
	for _, p := range []string{"www.", "m."} {
		host = strings.TrimPrefix(host, p)
	}
	return link, host, true
}

// lookupAdd is the Add page's box: it classifies q and answers what the
// matching handler would.
func lookupAdd(w http.ResponseWriter, r *http.Request) {
	rt := classifyAddInput(viewer(r), r.FormValue("q"))
	switch rt.Target {
	case addMusicBrainz:
		// a MusicBrainz page that isn't an entity link would be searched as text
		if _, _, ok := mb.ParseLink(rt.Value); !ok {
			http.Error(w, msgMBLink, http.StatusBadRequest)
			return
		}
		listMusicBrainz(w, r, rt.Value)
	case addSearch:
		listMusicBrainz(w, r, rt.Value)
	case addYouTube:
		previewYouTube(w, r, rt.Value)
	case addSecretSource:
		checkSecret(w, r, rt.Value)
	default:
		http.Error(w, rt.Message, http.StatusBadRequest)
	}
}
