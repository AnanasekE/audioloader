package mb

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	spotifyID   = regexp.MustCompile(`^[A-Za-z0-9]{22}$`)
	spotifyIntl = regexp.MustCompile(`^intl-[a-z]{2,3}(-[A-Za-z]{2,4})?$`)
)

// SpotifyURL returns the canonical Spotify link of the given kind ("album"
// or "track") among the relations, or "". Links that aren't an
// open.spotify.com album or track are ignored; "?si=" and an "intl-xx"
// path prefix are dropped.
func SpotifyURL(rels []Relation, kind string) string {
	for _, r := range rels {
		if u := ParseSpotifyURL(r.URL.Resource); u != "" && strings.Contains(u, "/"+kind+"/") {
			return u
		}
	}
	return ""
}

// ParseSpotifyURL returns the canonical form of an open.spotify.com album
// or track URL, or "" for anything else.
func ParseSpotifyURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.EqualFold(u.Hostname(), "open.spotify.com") {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 && spotifyIntl.MatchString(parts[0]) {
		parts = parts[1:]
	}
	if len(parts) != 2 || (parts[0] != "album" && parts[0] != "track") || !spotifyID.MatchString(parts[1]) {
		return ""
	}
	return "https://open.spotify.com/" + parts[0] + "/" + parts[1]
}
