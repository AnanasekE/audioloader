// Package match compares titles and names from different sources, e.g. a
// file's tags with MusicBrainz, ignoring case, punctuation and "feat." parts.
package match

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	// "Song (feat. X)", "Song feat. X", "Song [ft. X]"
	featPattern = regexp.MustCompile(`(?i)[\(\[]?\s*\b(feat|ft|featuring)\b\.?\s.*$`)
	spaces      = regexp.MustCompile(`\s+`)
)

// Normalize lowercases s, strips accents, punctuation and a trailing
// "feat. ..." part, and collapses whitespace.
func Normalize(s string) string {
	s = featPattern.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&", " and ")
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r): // combining accents left by NFD
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.TrimSpace(spaces.ReplaceAllString(b.String(), " "))
}

// Similarity is 1 for equal normalized strings, falling towards 0 with
// their edit distance.
func Similarity(a, b string) float64 {
	a, b = Normalize(a), Normalize(b)
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
	longest := max(len(ra), len(rb))
	if longest == 0 {
		return 1
	}
	return 1 - float64(levenshtein(ra, rb))/float64(longest)
}

// Same reports whether a and b are equal after normalizing.
func Same(a, b string) bool {
	return Normalize(a) == Normalize(b)
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
