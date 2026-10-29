package match

import (
	"regexp"
	"strings"
)

var (
	// "(Official Video)", "[Official Audio]", "(Lyrics)", "(HD)", ...
	noise = regexp.MustCompile(`(?i)\s*[\(\[][^\)\]]*\b(official|video|audio|lyrics?|visuali[sz]er|hd|hq|4k|mv)\b[^\)\]]*[\)\]]`)
	dash  = regexp.MustCompile(`\s+[-–—|]\s+`)
	// any bracketed part, e.g. "(Remastered 2019)"
	parens = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)
)

// CleanTitle turns a video title into a track title: it drops "(Official
// Video)" and the like, and an "Artist - " prefix naming one of artists.
func CleanTitle(videoTitle string, artists []string) string {
	t := strings.TrimSpace(noise.ReplaceAllString(videoTitle, ""))
	if loc := dash.FindStringIndex(t); loc != nil {
		prefix := t[:loc[0]]
		for _, a := range artists {
			if Same(prefix, a) || strings.Contains(Normalize(prefix), Normalize(a)) && a != "" {
				return strings.TrimSpace(t[loc[1]:])
			}
		}
	}
	return t
}

// TitlePrefix is the "Artist" of a video title shaped "Artist - Title",
// without "(Official Video)" and the like; "" if it has no such prefix.
func TitlePrefix(videoTitle string) string {
	t := strings.TrimSpace(noise.ReplaceAllString(videoTitle, ""))
	if loc := dash.FindStringIndex(t); loc != nil {
		return t[:loc[0]]
	}
	return ""
}

// TitleScore is how well a video or song title fits a track title, 0..1:
// the cleaned title against the track's, with and without bracketed parts
// ("Song (Remastered)" is still the song).
func TitleScore(candidate, title string, artists []string) float64 {
	c := CleanTitle(candidate, artists)
	return max(Similarity(c, title), Similarity(parens.ReplaceAllString(c, ""), parens.ReplaceAllString(title, "")))
}
