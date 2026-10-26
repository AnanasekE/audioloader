package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/jobs"
	"audioloader/internal/sources/secret"
)

// JobSecretDownload downloads one page of the secret source as a secret
// track. It runs in the downloads lane but is not a download: it has no
// placeholder track, so the Downloads page never lists it.
const JobSecretDownload = "secret_download"

// SecretImport is one secret source download.
type SecretImport struct {
	// PageURL is the normalized page URL, which becomes the track's source URL.
	PageURL string
	// AudioURL is the file the page names, found by secret.Source.Inspect.
	AudioURL    string
	Title       string
	Description string
	Tags        []string
	ArtistIDs   []string
}

// EnqueueSecret queues the download. A second request for a page that is
// still queued or running does nothing.
func EnqueueSecret(imp SecretImport) error {
	return jobs.EnqueueAt(JobSecretDownload, imp.PageURL, imp, time.Time{})
}

func registerSecret() {
	jobs.Register(JobSecretDownload, func(ctx context.Context, payload json.RawMessage) error {
		var imp SecretImport
		if err := json.Unmarshal(payload, &imp); err != nil {
			return jobs.Permanent(err)
		}
		return ImportSecret(ctx, secret.Default(), imp)
	})
}

// ImportSecret fetches the page's audio file and adds it as a secret track
// by the given creators, with the page URL as its source URL. The usual
// duplicate check on the file applies. Failures that a retry can't fix are
// permanent.
func ImportSecret(ctx context.Context, src *secret.Source, imp SecretImport) error {
	if src == nil {
		return jobs.Permanent(secret.ErrOff)
	}
	pageURL, err := src.Normalize(imp.PageURL)
	if err != nil {
		return jobs.Permanent(err)
	}
	existing, err := db.TrackBySourceURL(pageURL)
	if err != nil {
		return err
	}
	if existing != "" {
		if _, err := db.GetTrack(db.Secret, existing); err == nil {
			return nil // downloaded before
		}
	}

	dir, err := os.MkdirTemp("", "audioloader-secret-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	file, err := src.Fetch(ctx, &secret.Page{URL: pageURL, AudioURL: imp.AudioURL}, dir)
	if err != nil {
		if ctx.Err() != nil {
			return err // shutting down; the job runs again
		}
		switch {
		case errors.Is(err, secret.ErrAudioHost), errors.Is(err, secret.ErrOffHost), errors.Is(err, secret.ErrTooLarge):
			return jobs.Permanent(err)
		}
		return err
	}
	f, err := os.Open(file.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	track := &audio.Track{Title: imp.Title, Duration: file.Duration, Description: imp.Description, Tags: db.NormalizeTags(imp.Tags)}
	res, err := db.AddTrack(db.Secret, imp.ArtistIDs, track, f, file.Ext, nil, "")
	if err != nil {
		return permanentIfBad(err)
	}
	return db.SetSourceURL(res.TrackID, pageURL)
}
