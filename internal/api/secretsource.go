package api

import (
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"audioloader/internal/audio"
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

// uploaderPages are the pages one level above the pages the artist's tracks
// came from, such as the uploaders' pages; none outside secret mode.
func uploaderPages(r *http.Request, artistID string) ([]string, error) {
	src := secret.Default()
	if src == nil || !viewer(r).IsSecret() {
		return nil, nil
	}
	urls, err := db.ArtistSourceURLs(viewer(r), artistID)
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	for _, u := range urls {
		if _, err := src.Normalize(u); err != nil {
			continue
		}
		if p := src.Parent(u); p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// secretSourceError writes the message for a failed lookup of a page.
func secretSourceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, secret.ErrBadURL):
		http.Error(w, "That isn't a page of the secret source.", http.StatusBadRequest)
	case errors.Is(err, secret.ErrNoAudio):
		http.Error(w, "No audio or recordings found on that page. Some uploaders' pages are empty on the site itself.", http.StatusUnprocessableEntity)
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
	checkSecret(w, r, r.FormValue("url"))
}

// checkSecret is checkSecretSource for a link given as a value.
func checkSecret(w http.ResponseWriter, r *http.Request, rawURL string) {
	src := secret.Default()
	pageURL, err := src.Normalize(rawURL)
	if err != nil {
		secretSourceError(w, err)
		return
	}
	if t, err := existingSecretTrack(r, pageURL); err != nil {
		serverError(w, err)
		return
	} else if t != nil {
		renderStatus(w, r, http.StatusConflict, components.SecretSourceExists(t, src.Parent(pageURL)))
		return
	}
	page, listing, err := src.Look(r.Context(), pageURL)
	if err != nil {
		secretSourceError(w, err)
		return
	}
	if listing != nil {
		entries := make([]components.ListingEntry, len(listing.Entries))
		for i, e := range listing.Entries {
			entries[i].Entry = e
			if entries[i].Have, err = existingSecretTrack(r, e.URL); err != nil {
				serverError(w, err)
				return
			}
		}
		render(w, r, components.SecretSourceListing(listing, entries))
		return
	}
	rememberPage(page)
	artists, err := db.ListArtists(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	var selected []string
	var newCreator string
	if name := src.Uploader(page.URL); name != "" {
		a, err := db.CreatorForUploader(viewer(r), src.Parent(page.URL), name)
		if err != nil {
			serverError(w, err)
			return
		}
		if a != nil {
			selected = []string{a.ID.String()}
		} else if len([]rune(name)) <= maxCreatorName {
			newCreator = name
		}
	}
	render(w, r, components.SecretSourceDetails(page, artists, src.Parent(page.URL), selected, newCreator))
}

// maxCreatorName is the longest creator name suggested from an address.
const maxCreatorName = 200

// creatorForName is the artist v can see with that name, ignoring case, or a
// new one.
func creatorForName(v db.Viewer, name string) (*audio.Artist, error) {
	a, err := db.CreatorForUploader(v, "", name)
	if err == nil && a == nil {
		if a, err = db.AddArtist(v, name); errors.Is(err, db.ErrArtistExists) {
			a, err = db.CreatorForUploader(v, "", name)
		}
	}
	if err != nil || a == nil {
		return nil, errors.Join(err, errors.New("could not create the creator"))
	}
	return a, nil
}

// downloadFromSecretSource queues the download. It needs at least one
// creator, each one the viewer can see, or a new_creator that is still
// checked, which is created (or found by name) just before queueing.
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
	newName := ""
	if names := r.Form["new_creator"]; len(names) > 0 {
		if newName = strings.TrimSpace(names[0]); newName == "" || len([]rune(newName)) > maxCreatorName {
			http.Error(w, "Invalid creator.", http.StatusBadRequest)
			return
		}
	}
	if len(artistIDs) == 0 && newName == "" {
		http.Error(w, "Pick at least one creator.", http.StatusBadRequest)
		return
	}
	if t, err := existingSecretTrack(r, pageURL); err != nil {
		serverError(w, err)
		return
	} else if t != nil {
		renderStatus(w, r, http.StatusConflict, components.SecretSourceExists(t, src.Parent(pageURL)))
		return
	}

	page := rememberedPage(pageURL)
	if page == nil {
		if page, err = src.Inspect(r.Context(), pageURL); err != nil {
			secretSourceError(w, err)
			return
		}
	}
	if newName != "" {
		a, err := creatorForName(viewer(r), newName)
		if err != nil {
			serverError(w, err)
			return
		}
		if id := a.ID.String(); !slices.Contains(artistIDs, id) {
			artistIDs = append(artistIDs, id)
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
