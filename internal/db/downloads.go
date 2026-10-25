package db

import (
	"database/sql"
	"errors"
	"time"

	"audioloader/internal/audio"
)

// Download statuses.
const (
	DownloadQueued      = "queued"
	DownloadSearching   = "searching"
	DownloadDownloading = "downloading"
	DownloadDone        = "done"
	DownloadNotFound    = "not_found"
	DownloadFailed      = "failed"
)

// Download is a placeholder track waiting for its file.
type Download struct {
	Track     *audio.Track
	Release   *audio.Release // the release it was confirmed on, without tracks
	Disc      int
	Position  int
	Status    string
	Source    string
	Progress  float64 // 0..1 while downloading
	LastError string
	UpdatedAt string
}

// ListDownloads returns downloads with any of the given statuses, by release.
func ListDownloads(v Viewer, statuses ...string) ([]*Download, error) {
	args := make([]any, len(statuses))
	for i, s := range statuses {
		args[i] = s
	}
	rows, err := DB.Query(`
		SELECT d.track_id, d.status, COALESCE(d.source, ''), COALESCE(d.progress, 0), COALESCE(d.last_error, ''), d.updated_at,
			r.uuid, r.title, r.type, r.img_url, rt.disc, rt.position
		FROM downloads d
		JOIN release_tracks rt ON rt.track_id = d.track_id
		JOIN releases r ON r.uuid = rt.release_id
		JOIN tracks t ON t.uuid = d.track_id
		WHERE d.status IN (`+placeholders(len(statuses))+`) AND `+v.trackCond("t")+`
		GROUP BY d.track_id
		ORDER BY r.title COLLATE NOCASE, r.uuid, rt.disc, rt.position;`, args...)
	if err != nil {
		return nil, err
	}
	var out []*Download
	for rows.Next() {
		d := &Download{Release: &audio.Release{}}
		var trackID string
		err := rows.Scan(&trackID, &d.Status, &d.Source, &d.Progress, &d.LastError, &d.UpdatedAt,
			&d.Release.ID, &d.Release.Title, &d.Release.Type, &d.Release.ImgURL, &d.Disc, &d.Position)
		if err != nil {
			rows.Close()
			return nil, err
		}
		d.Track = &audio.Track{}
		d.Track.ID.UnmarshalText([]byte(trackID))
		out = append(out, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, d := range out {
		t, err := GetTrack(v, d.Track.ID.String())
		if err != nil {
			return nil, err
		}
		d.Track = t
	}
	return out, nil
}

// GetDownload returns the download of a placeholder track.
func GetDownload(v Viewer, trackID string) (*Download, error) {
	var d Download
	err := DB.QueryRow(`
		SELECT status, COALESCE(source, ''), COALESCE(progress, 0), COALESCE(last_error, ''), updated_at
		FROM downloads WHERE track_id = ?;`, trackID).Scan(&d.Status, &d.Source, &d.Progress, &d.LastError, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.Track, err = GetTrack(v, trackID)
	if err != nil {
		return nil, err
	}
	for _, r := range d.Track.Releases {
		d.Release = r
		break
	}
	return &d, nil
}

// SetDownload records a download's state. progress and lastErr are
// cleared when zero.
func SetDownload(trackID, status, source string, progress float64, lastErr string) error {
	var prog any
	if progress > 0 {
		prog = progress
	}
	_, err := DB.Exec(`
		UPDATE downloads SET status = ?, source = COALESCE(?, source), progress = ?, last_error = ?, updated_at = ?
		WHERE track_id = ?;`,
		status, nullIfEmpty(source), prog, nullIfEmpty(lastErr), sqlTime(time.Now()), trackID)
	return err
}

// SetDownloadProgress updates only the progress of a running download.
func SetDownloadProgress(trackID string, progress float64) error {
	_, err := DB.Exec(`UPDATE downloads SET progress = ?, updated_at = ? WHERE track_id = ?;`, progress, sqlTime(time.Now()), trackID)
	return err
}

// DownloadCounts returns the number of downloads per status.
func DownloadCounts() (map[string]int, error) {
	rows, err := DB.Query(`SELECT status, COUNT(*) FROM downloads GROUP BY status;`)
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

// SetSourceURL records where a track's file came from.
func SetSourceURL(trackID, url string) error {
	_, err := DB.Exec(`UPDATE tracks SET source_url = ? WHERE uuid = ?;`, url, trackID)
	return err
}

// TrackBySourceURL returns the track downloaded from url, or "".
func TrackBySourceURL(url string) (string, error) {
	var id string
	err := DB.QueryRow(`SELECT uuid FROM tracks WHERE source_url = ? AND has_file = 1 LIMIT 1;`, url).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
