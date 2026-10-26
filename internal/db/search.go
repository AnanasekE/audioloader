package db

import (
	"strings"
	"unicode"

	"audioloader/internal/audio"
)

// SearchResults are the matches of a search, best first within each kind.
type SearchResults struct {
	Tracks   []*audio.Track
	Releases []*audio.Release
	Artists  []*audio.Artist
	Shows    []*audio.Show
}

func (r *SearchResults) Empty() bool {
	return len(r.Tracks)+len(r.Releases)+len(r.Artists)+len(r.Shows) == 0
}

// Search finds tracks, releases and artists matching every word of q, the
// last word as a prefix (so results show while typing). Accents and case
// don't matter.
func Search(v Viewer, q string, limit int) (*SearchResults, error) {
	match := searchQuery(q)
	res := &SearchResults{}
	if match == "" {
		return res, nil
	}
	if err := refreshSearch(); err != nil {
		return nil, err
	}

	ids := map[string][]string{}
	for _, kind := range []string{"track", "release", "artist", "show"} {
		rows, err := DB.Query(`
			SELECT ref FROM search WHERE search MATCH ? AND kind = ? AND (secret = 0 OR ?)
			ORDER BY bm25(search, 0, 0, 0, 10, 3, 1) LIMIT ?;`, match, kind, v.Secret, limit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids[kind] = append(ids[kind], id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	for _, id := range ids["track"] {
		t, err := GetTrack(v, id)
		if err != nil {
			return nil, err
		}
		res.Tracks = append(res.Tracks, t)
	}
	for _, id := range ids["release"] {
		r, err := scanRelease(DB.QueryRow(selectRelease+` WHERE uuid = ?;`, id))
		if err != nil {
			return nil, err
		}
		res.Releases = append(res.Releases, r)
	}
	if err := attachReleaseArtists(res.Releases); err != nil {
		return nil, err
	}
	for _, id := range ids["artist"] {
		a, err := GetArtist(v, id)
		if err != nil {
			return nil, err
		}
		res.Artists = append(res.Artists, a)
	}
	for _, id := range ids["show"] {
		s, err := GetShow(v, id)
		if err != nil {
			return nil, err
		}
		s.Episodes = nil
		res.Shows = append(res.Shows, s)
	}
	return res, nil
}

// searchQuery turns user input into an FTS5 query: each word quoted (so
// FTS5 syntax in the input is just text), the last one a prefix.
func searchQuery(q string) string {
	words := strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for i, w := range words {
		words[i] = `"` + w + `"`
	}
	if len(words) == 0 {
		return ""
	}
	words[len(words)-1] += "*"
	return strings.Join(words, " ")
}

// refreshSearch rebuilds the index if anything changed since the last build.
// At this library's size a rebuild takes milliseconds, and it is simpler to
// get right than updating rows across the artist map tables.
func refreshSearch() error {
	var dirty bool
	if err := DB.QueryRow(`SELECT dirty FROM search_state;`).Scan(&dirty); err != nil {
		return err
	}
	if !dirty {
		return nil
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`DELETE FROM search;`,
		// original: what a cover is of, so searching the original finds covers. A
		// secret original in the library is left out, or its name would find the cover.
		`INSERT INTO search(kind, ref, secret, title, artists, original)
			SELECT 'track', t.uuid, t.secret, t.title, COALESCE(group_concat(a.name, ' '), ''),
				CASE WHEN t.original_track_id IS NOT NULL THEN COALESCE((
					SELECT o.title || ' ' || COALESCE((SELECT group_concat(oa.name, ' ') FROM track_artist_map om JOIN artists oa ON oa.uuid = om.artist_id WHERE om.track_id = o.uuid), '')
					FROM tracks o WHERE o.uuid = t.original_track_id AND o.secret = 0), '')
				ELSE trim(COALESCE(t.original_title, '') || ' ' || COALESCE(t.original_artists, '')) END
			FROM tracks t
			LEFT JOIN track_artist_map m ON m.track_id = t.uuid
			LEFT JOIN artists a ON a.uuid = m.artist_id
			WHERE ` + libraryTracks + `
			GROUP BY t.uuid;`,
		`INSERT INTO search(kind, ref, secret, title, artists)
			SELECT 'release', r.uuid, NOT (` + Public.releaseCond("r") + `), r.title, COALESCE(group_concat(a.name, ' '), '')
			FROM releases r
			LEFT JOIN release_artist_map m ON m.release_id = r.uuid
			LEFT JOIN artists a ON a.uuid = m.artist_id
			GROUP BY r.uuid;`,
		`INSERT INTO search(kind, ref, secret, title, artists)
			SELECT 'artist', a.uuid, NOT (` + Public.artistCond("a") + `), a.name, '' FROM artists a;`,
		`INSERT INTO search(kind, ref, secret, title, artists)
			SELECT 'show', s.uuid, NOT (` + Public.showCond("s") + `), s.title, COALESCE(s.author, '') FROM shows s;`,
		// the rebuild itself doesn't touch the watched tables, so this sticks
		`UPDATE search_state SET dirty = 0;`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}
