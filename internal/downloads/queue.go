package downloads

import (
	"time"

	"audioloader/internal/jobs"
)

// JobDownload finds and downloads the file of one placeholder track.
const JobDownload = "download"

// Enqueue queues the download of a placeholder track.
func Enqueue(trackID string) error {
	return jobs.EnqueueAt(JobDownload, trackID, map[string]string{"TrackID": trackID}, time.Time{})
}
