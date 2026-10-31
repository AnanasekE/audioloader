package db

import (
	"sort"
	"strconv"
	"strings"

	"audioloader/internal/audio"
)

// TagQuery selects tracks by their tags. Include is all-of when Any is
// false, any-of when true; Exclude always removes.
type TagQuery struct {
	Include, Exclude []string
	Any              bool
	Sort             string // "added" (default), "creator", "tag"
}

// where returns the SQL conditions (each starting with AND) and their
// arguments for q, on a tracks table aliased t. Tags match ignoring case.
func (q TagQuery) where() (string, []any) {
	include, exclude := NormalizeTags(q.Include), NormalizeTags(q.Exclude)
	var sql strings.Builder
	var args []any
	list := func(tags []string) string {
		for _, t := range tags {
			args = append(args, t)
		}
		return placeholders(len(tags))
	}
	if len(include) > 0 {
		if q.Any {
			sql.WriteString(` AND EXISTS (SELECT 1 FROM track_tags g WHERE g.track_id = t.uuid AND g.tag IN (` + list(include) + `))`)
		} else {
			sql.WriteString(` AND (SELECT COUNT(DISTINCT g.tag) FROM track_tags g WHERE g.track_id = t.uuid AND g.tag IN (` + list(include) + `)) = ` + strconv.Itoa(len(include)))
		}
	}
	if len(exclude) > 0 {
		sql.WriteString(` AND NOT EXISTS (SELECT 1 FROM track_tags g WHERE g.track_id = t.uuid AND g.tag IN (` + list(exclude) + `))`)
	}
	return sql.String(), args
}

// TrackTags returns, for the tracks v can see that match q, each tag with
// how many of those tracks have it. With an empty q, that's every tag's count.
func TrackTags(v Viewer, q TagQuery) (map[string]int, error) {
	cond, args := q.where()
	rows, err := DB.Query(`SELECT MIN(g.tag), COUNT(*) FROM track_tags g JOIN tracks t ON t.uuid = g.track_id
		WHERE `+libraryTracks+` AND `+v.trackCond("t")+cond+` GROUP BY g.tag;`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var tag string
		var n int
		if err := rows.Scan(&tag, &n); err != nil {
			return nil, err
		}
		counts[tag] = n
	}
	return counts, rows.Err()
}

// TracksByTags returns the tracks v can see that match q, with their tags.
func TracksByTags(v Viewer, q TagQuery) ([]*audio.Track, error) {
	cond, args := q.where()
	rows, err := DB.Query(selectTrack+` WHERE `+libraryTracks+` AND `+v.trackCond("t")+cond+` ORDER BY t.rowid DESC;`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tracks := make([]*audio.Track, 0)
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	if err := attachArtists(tracks); err != nil {
		return nil, err
	}
	if err := attachReleases(tracks); err != nil {
		return nil, err
	}
	if err := attachCovers(v, tracks); err != nil {
		return nil, err
	}
	if err := attachTags(tracks); err != nil {
		return nil, err
	}
	first := func(t *audio.Track) string {
		switch q.Sort {
		case "creator":
			if len(t.Artists) > 0 {
				return strings.ToLower(t.Artists[0].Name)
			}
		case "tag":
			if len(t.Tags) > 0 {
				return strings.ToLower(t.Tags[0])
			}
		}
		return ""
	}
	if q.Sort == "creator" || q.Sort == "tag" {
		sort.SliceStable(tracks, func(i, j int) bool {
			a, b := first(tracks[i]), first(tracks[j])
			if a == "" || b == "" {
				return a != "" && b == "" // untagged / creatorless last
			}
			if a != b {
				return a < b
			}
			return strings.ToLower(tracks[i].Title) < strings.ToLower(tracks[j].Title)
		})
	}
	return tracks, nil
}
