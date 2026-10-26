package api

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"uuid"

	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/sources/secret"
)

// pages remembers what Check found for a while, so the download that follows
// uses the audio file the server itself saw, never one sent by the browser,
// and the source is asked for each page only once.
var pages = struct {
	sync.Mutex
	m map[string]cachedPage
}{m: map[string]cachedPage{}}

type cachedPage struct {
	page *secret.Page
	at   time.Time
}

const pageTTL = 30 * time.Minute

func rememberPage(p *secret.Page) {
	pages.Lock()
	defer pages.Unlock()
	for url, c := range pages.m {
		if time.Since(c.at) > pageTTL {
			delete(pages.m, url)
		}
	}
	pages.m[p.URL] = cachedPage{p, time.Now()}
}

func rememberedPage(url string) *secret.Page {
	pages.Lock()
	defer pages.Unlock()
	if c, ok := pages.m[url]; ok && time.Since(c.at) <= pageTTL {
		return c.page
	}
	return nil
}

func getSecretSourcePage(w http.ResponseWriter, r *http.Request) {
	render(w, r, components.SecretSourcePage())
}

// secretSourceError writes the message for a failed lookup of a page.
func secretSourceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, secret.ErrBadURL):
		http.Error(w, "That isn't a page of the secret source.", http.StatusBadRequest)
	case errors.Is(err, secret.ErrNoAudio):
		http.Error(w, "No audio found on that page.", http.StatusUnprocessableEntity)
	case errors.Is(err, secret.ErrAudioHost), errors.Is(err, secret.ErrOffHost):
		http.Error(w, "That page points to a host that isn't allowed.", http.StatusUnprocessableEntity)
	case errors.Is(err, secret.ErrTooLarge):
		http.Error(w, "That is larger than allowed.", http.StatusUnprocessableEntity)
	default:
		log.Println("secret source:", err)
		http.Error(w, "Could not load that page.", http.StatusBadGateway)
	}
}

// existingSecretTrack is the track already downloaded from pageURL that the
// viewer can see, or nil.
func existingSecretTrack(r *http.Request, pageURL string) (*components.ExistingTrack, error) {
	id, err := db.TrackBySourceURL(pageURL)
	if err != nil || id == "" {
		return nil, err
	}
	t, err := db.GetTrack(viewer(r), id)
	if errors.Is(err, db.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &components.ExistingTrack{ID: t.ID.String(), Title: t.Title}, nil
}

// checkSecretSource loads the page and answers with the form to confirm.
func checkSecretSource(w http.ResponseWriter, r *http.Request) {
	src := secret.Default()
	pageURL, err := src.Normalize(r.FormValue("url"))
	if err != nil {
		secretSourceError(w, err)
		return
	}
	if t, err := existingSecretTrack(r, pageURL); err != nil {
		serverError(w, err)
		return
	} else if t != nil {
		renderStatus(w, r, http.StatusConflict, components.SecretSourceExists(t))
		return
	}
	page, err := src.Inspect(r.Context(), pageURL)
	if err != nil {
		secretSourceError(w, err)
		return
	}
	rememberPage(page)
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.SecretSourceDetails(page, artists))
}

// downloadFromSecretSource queues the download. The creators are always
// picked by hand: at least one, and each must be a creator the viewer can see.
func downloadFromSecretSource(w http.ResponseWriter, r *http.Request) {
	src := secret.Default()
	pageURL, err := src.Normalize(r.FormValue("url"))
	if err != nil {
		secretSourceError(w, err)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "A title is required.", http.StatusBadRequest)
		return
	}
	if len([]rune(title)) > 200 {
		http.Error(w, "The title is too long.", http.StatusBadRequest)
		return
	}
	description := strings.TrimSpace(r.FormValue("description"))
	if len(description) > maxDescription {
		http.Error(w, "The description is too long.", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form.", http.StatusBadRequest)
		return
	}
	var artistIDs []string
	for _, id := range r.Form["artist_id"] {
		if _, err := uuid.Parse(id); err != nil {
			http.Error(w, "Invalid creator.", http.StatusBadRequest)
			return
		}
		if _, err := db.GetArtist(viewer(r), id); err != nil {
			http.Error(w, "Unknown creator.", http.StatusBadRequest)
			return
		}
		artistIDs = append(artistIDs, id)
	}
	if len(artistIDs) == 0 {
		http.Error(w, "Pick at least one creator.", http.StatusBadRequest)
		return
	}
	if t, err := existingSecretTrack(r, pageURL); err != nil {
		serverError(w, err)
		return
	} else if t != nil {
		renderStatus(w, r, http.StatusConflict, components.SecretSourceExists(t))
		return
	}

	page := rememberedPage(pageURL)
	if page == nil {
		if page, err = src.Inspect(r.Context(), pageURL); err != nil {
			secretSourceError(w, err)
			return
		}
	}
	err = downloads.EnqueueSecret(downloads.SecretImport{
		PageURL:     pageURL,
		AudioURL:    page.AudioURL,
		Title:       title,
		Description: description,
		Tags:        strings.Split(r.FormValue("tags"), ","),
		ArtistIDs:   artistIDs,
	})
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.SecretSourceStatus(pageURL, components.SecretDownloadWorking, "", ""))
}

// getSecretSourceStatus answers the page's polling for a download's progress.
func getSecretSourceStatus(w http.ResponseWriter, r *http.Request) {
	src := secret.Default()
	pageURL, err := src.Normalize(r.URL.Query().Get("url"))
	if err != nil {
		secretSourceError(w, err)
		return
	}
	if t, err := existingSecretTrack(r, pageURL); err != nil {
		serverError(w, err)
		return
	} else if t != nil {
		render(w, r, components.SecretSourceStatus(pageURL, components.SecretDownloadDone, t.ID, ""))
		return
	}
	job, err := jobs.Latest(downloads.JobSecretDownload, pageURL)
	if err != nil {
		serverError(w, err)
		return
	}
	switch {
	case job == nil:
		render(w, r, components.SecretSourceStatus(pageURL, components.SecretDownloadFailed, "", "Nothing is downloading for that page."))
	case job.Status == "queued" || job.Status == "running":
		render(w, r, components.SecretSourceStatus(pageURL, components.SecretDownloadWorking, "", ""))
	case job.Status == "failed":
		render(w, r, components.SecretSourceStatus(pageURL, components.SecretDownloadFailed, "", "The download failed: "+job.LastError))
	default:
		render(w, r, components.SecretSourceStatus(pageURL, components.SecretDownloadDone, "", ""))
	}
}
