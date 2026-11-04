package db

import "strings"

// OwnedSong is the library's track for a MusicBrainz recording.
type OwnedSong struct {
	TrackID   string
	ReleaseID string // "" when the track is on no release
	HasFile   bool   // false: a placeholder waiting for its download
}

// inList is "?, ?, ?" for n values and the values as query arguments.
func inList(ids []string) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ","), args
}

// OwnedRecordings maps each of the recording MBIDs the normal library has
// (as a track or as a queued placeholder) to its track. A downloaded track
// wins over a placeholder of the same recording.
func OwnedRecordings(ids []string) (map[string]OwnedSong, error) {
	out := map[string]OwnedSong{}
	if len(ids) == 0 {
		return out, nil
	}
	in, args := inList(ids)
	rows, err := DB.Query(`
		SELECT t.mbid, t.uuid, t.has_file,
			COALESCE((SELECT release_id FROM release_tracks WHERE track_id = t.uuid LIMIT 1), '')
		FROM tracks t
		WHERE t.mbid IN (`+in+`) AND t.secret = 0 AND t.kind = 'music'
		ORDER BY t.has_file ASC;`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rec string
		var o OwnedSong
		if err := rows.Scan(&rec, &o.TrackID, &o.HasFile, &o.ReleaseID); err != nil {
			return nil, err
		}
		out[rec] = o // downloaded tracks come last and replace placeholders
	}
	return out, rows.Err()
}

// ReleaseCount is how much of a release group the library has: Have tracks
// downloaded out of the Total on its release.
type ReleaseCount struct {
	Have, Total int
}

// OwnedReleaseCounts maps each release group MBID the normal library has a
// release of to its track counts.
func OwnedReleaseCounts(releaseGroupIDs []string) (map[string]ReleaseCount, error) {
	out := map[string]ReleaseCount{}
	if len(releaseGroupIDs) == 0 {
		return out, nil
	}
	in, args := inList(releaseGroupIDs)
	rows, err := DB.Query(`
		SELECT r.release_group_mbid, COUNT(t.uuid), COALESCE(SUM(t.has_file), 0)
		FROM releases r
		JOIN release_tracks rt ON rt.release_id = r.uuid
		JOIN tracks t ON t.uuid = rt.track_id AND t.secret = 0
		WHERE r.release_group_mbid IN (`+in+`)
		GROUP BY r.release_group_mbid;`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var c ReleaseCount
		if err := rows.Scan(&id, &c.Total, &c.Have); err != nil {
			return nil, err
		}
		out[id] = c
	}
	return out, rows.Err()
}

// FollowedArtists returns the MusicBrainz artist IDs, among mbids, that a
// library artist follows.
func FollowedArtists(mbids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(mbids) == 0 {
		return out, nil
	}
	in, args := inList(mbids)
	rows, err := DB.Query(`
		SELECT a.mbid FROM artist_follows f JOIN artists a ON a.uuid = f.artist_id
		WHERE a.mbid IN (`+in+`);`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
