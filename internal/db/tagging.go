package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"audioloader/internal/audio"
	"audioloader/internal/match"
)

// Match is a MusicBrainz candidate for a track, as found by the auto-tagger
// and stored in tag_matches until it is applied or rejected.
type Match struct {
	Source string  `json:"source"` // "tags", "acoustid" or "search"
	Score  float64 `json:"score"`  // 0..1

	RecordingID string        `json:"recording_id"`
	Title       string        `json:"title"`
	LengthMs    int           `json:"length_ms"`
	ISRC        string        `json:"isrc,omitempty"`
	Artists     []MatchArtist `json:"artists"`

	ReleaseID      string        `json:"release_id,omitempty"`
	ReleaseGroupID string        `json:"release_group_id,omitempty"`
	ReleaseTitle   string        `json:"release_title,omitempty"`
	ReleaseType    string        `json:"release_type,omitempty"` // our type: album, ep, single, compilation
	ReleaseDate    string        `json:"release_date,omitempty"`
	ReleaseArtists []MatchArtist `json:"release_artists,omitempty"`
}

type MatchArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Tag statuses of a track, after its last auto-tagging run.
const (
	TagMatched = "matched"
	TagReview  = "review"
	TagNone    = "none"
)

// SaveMatches stores the candidates for a track, replacing earlier pending
// ones, and sets its tag status.
func SaveMatches(trackID string, matches []Match, status string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tag_matches WHERE track_id = ? AND status = 'pending';`, trackID); err != nil {
		return err
	}
	for _, m := range matches {
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		// a candidate rejected before stays rejected
		_, err = tx.Exec(`
			INSERT INTO tag_matches(track_id, source, recording_mbid, release_mbid, score, data)
			VALUES(?, ?, ?, ?, ?, ?)
			ON CONFLICT(track_id, recording_mbid) DO NOTHING;`,
			trackID, m.Source, m.RecordingID, nullIfEmpty(m.ReleaseID), m.Score, string(data))
		if err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE tracks SET tag_status = ? WHERE uuid = ?;`, nullIfEmpty(status), trackID); err != nil {
		return err
	}
	return tx.Commit()
}

// RejectedRecordings returns the recording IDs rejected for a track before,
// so the tagger doesn't propose them again.
func RejectedRecordings(trackID string) (map[string]bool, error) {
	rows, err := DB.Query(`SELECT recording_mbid FROM tag_matches WHERE track_id = ? AND status = 'rejected';`, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// PendingMatch is a stored candidate waiting for review.
type PendingMatch struct {
	TrackID string
	Match
}

// PendingMatches returns the candidates waiting for review, best first per track.
func PendingMatches(v Viewer) ([]PendingMatch, error) {
	rows, err := DB.Query(`
		SELECT m.track_id, m.data FROM tag_matches m
		JOIN tracks t ON t.uuid = m.track_id
		WHERE m.status = 'pending' AND ` + v.trackCond("t") + `
		ORDER BY t.title COLLATE NOCASE, m.track_id, m.score DESC;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingMatch
	for rows.Next() {
		var p PendingMatch
		var data string
		if err := rows.Scan(&p.TrackID, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &p.Match); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetMatch returns a stored candidate.
func GetMatch(trackID, recordingID string) (*Match, error) {
	var data string
	err := DB.QueryRow(`SELECT data FROM tag_matches WHERE track_id = ? AND recording_mbid = ?;`, trackID, recordingID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var m Match
	return &m, json.Unmarshal([]byte(data), &m)
}

// RejectMatch marks a candidate as wrong. A track without pending
// candidates left gets the "none" status.
func RejectMatch(trackID, recordingID string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE tag_matches SET status = 'rejected' WHERE track_id = ? AND recording_mbid = ?;`, trackID, recordingID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = tx.Exec(`
		UPDATE tracks SET tag_status = 'none'
		WHERE uuid = ? AND tag_status = 'review'
		AND NOT EXISTS (SELECT 1 FROM tag_matches WHERE track_id = ? AND status = 'pending');`, trackID, trackID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ApplyMatch writes a candidate's MusicBrainz IDs onto the track, its
// artists and the release it matches. It fills in missing values only:
// titles, types and dates the user set are kept. It returns the IDs of
// releases that got an MBID, e.g. to fetch their covers.
func ApplyMatch(trackID string, m Match) (releaseIDs []string, err error) {
	t, err := GetTrack(All, trackID)
	if err != nil {
		return nil, err
	}

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// another track may already have this recording, e.g. a remaster;
	// the recording MBID isn't unique, so that's fine
	_, err = tx.Exec(`UPDATE tracks SET mbid = ?, isrc = COALESCE(isrc, ?), tag_status = 'matched' WHERE uuid = ?;`,
		m.RecordingID, nullIfEmpty(m.ISRC), trackID)
	if err != nil {
		return nil, err
	}
	for _, a := range m.Artists {
		if err := setArtistMBID(tx, a); err != nil {
			return nil, err
		}
	}

	if m.ReleaseID != "" {
		var taken bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM releases WHERE mbid = ?);`, m.ReleaseID).Scan(&taken); err != nil {
			return nil, err
		}
		// the release the match is on: the track's release with the same title
		for _, r := range t.Releases {
			if taken || r.MBID != "" || !match.Same(r.Title, m.ReleaseTitle) {
				continue
			}
			_, err := tx.Exec(`
				UPDATE releases SET mbid = ?, release_group_mbid = ?, release_date = COALESCE(release_date, ?)
				WHERE uuid = ?;`,
				m.ReleaseID, nullIfEmpty(m.ReleaseGroupID), nullIfEmpty(m.ReleaseDate), r.ID.String())
			if err != nil {
				return nil, err
			}
			for _, a := range m.ReleaseArtists {
				if err := setArtistMBID(tx, a); err != nil {
					return nil, err
				}
			}
			releaseIDs = append(releaseIDs, r.ID.String())
			break
		}
	}

	if _, err := tx.Exec(`UPDATE tag_matches SET status = 'applied' WHERE track_id = ? AND recording_mbid = ?;`, trackID, m.RecordingID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM tag_matches WHERE track_id = ? AND status = 'pending';`, trackID); err != nil {
		return nil, err
	}
	return releaseIDs, tx.Commit()
}

// setArtistMBID gives the library artist with a's name the MBID, unless an
// artist already has it or that artist has another one.
func setArtistMBID(tx *sql.Tx, a MatchArtist) error {
	if a.ID == "" {
		return nil
	}
	_, err := tx.Exec(`
		UPDATE artists SET mbid = ?
		WHERE lower(name) = lower(?) AND mbid IS NULL
		AND NOT EXISTS (SELECT 1 FROM artists WHERE mbid = ?);`, a.ID, a.Name, a.ID)
	return err
}

// ArtistSuggestion is a library artist without a MusicBrainz ID that an
// applied match credits under a similar but different name. Nothing is
// linked until the user picks.
type ArtistSuggestion struct {
	Artist       *audio.Artist
	CreditedName string // how the match spells the artist
}

// ArtistSuggestions lists the library artists that applied matches credit
// under a close but not identical name, e.g. "Massive Attak" for "Massive
// Attack". Exact names are linked when the match is applied, so they never
// show up here.
func ArtistSuggestions(v Viewer) ([]ArtistSuggestion, error) {
	rows, err := DB.Query(`
		SELECT a.uuid, a.name, m.data FROM tag_matches m
		JOIN tracks t ON t.uuid = m.track_id
		JOIN track_artist_map tam ON tam.track_id = t.uuid
		JOIN artists a ON a.uuid = tam.artist_id
		WHERE m.status = 'applied' AND a.mbid IS NULL AND ` + v.trackCond("t") + ` AND ` + v.artistCond("a") + `
		ORDER BY a.name COLLATE NOCASE, a.uuid, m.rowid;`)
	if err != nil {
		return nil, err
	}
	type hit struct{ artistID, credited string }
	var hits []hit
	seen := map[string]bool{}
	for rows.Next() {
		var id, name, data string
		if err := rows.Scan(&id, &name, &data); err != nil {
			rows.Close()
			return nil, err
		}
		if seen[id] {
			continue
		}
		var m Match
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			rows.Close()
			return nil, err
		}
		for _, c := range append(m.Artists, m.ReleaseArtists...) {
			if c.ID != "" && !strings.EqualFold(name, c.Name) && match.Close(name, c.Name) {
				seen[id] = true
				hits = append(hits, hit{id, c.Name})
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var out []ArtistSuggestion
	for _, h := range hits {
		a, err := GetArtist(v, h.artistID)
		if err != nil {
			return nil, err
		}
		out = append(out, ArtistSuggestion{Artist: a, CreditedName: h.credited})
	}
	return out, nil
}

// UntaggedTrackIDs returns library tracks that were never auto-tagged or
// had no match last time.
func UntaggedTrackIDs() ([]string, error) {
	rows, err := DB.Query(`SELECT t.uuid FROM tracks t WHERE (t.tag_status IS NULL OR t.tag_status = 'none') AND ` + libraryTracks + `;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// TagCounts returns how many library tracks have each tag status ("" for untagged).
func TagCounts(v Viewer) (map[string]int, error) {
	rows, err := DB.Query(`SELECT COALESCE(t.tag_status, ''), COUNT(*) FROM tracks t WHERE ` + libraryTracks + ` AND ` + v.trackCond("t") + ` GROUP BY 1;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		counts[s] = n
	}
	return counts, rows.Err()
}
