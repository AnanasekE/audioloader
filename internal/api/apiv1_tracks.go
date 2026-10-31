package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
)

// containsWords reports whether text contains every word; no words match all.
func containsWords(text string, words []string) bool {
	text = strings.ToLower(text)
	return !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(text, w) })
}

// getTracksJSON lists the tracks with files. ?sort= is added (the default),
// creator or tag, as on the track page; ?q= keeps the tracks whose title or
// creators contain every word.
func getTracksJSON(w http.ResponseWriter, r *http.Request) {
	tracks, err := db.ListTracks(viewer(r))
	if err != nil {
		serverError(w, err)
		return
	}
	if words := strings.Fields(strings.ToLower(r.URL.Query().Get("q"))); len(words) > 0 {
		tracks = slices.DeleteFunc(tracks, func(t *audio.Track) bool {
			text := t.Title
			for _, a := range t.Artists {
				text += " " + a.Name
			}
			return !containsWords(text, words)
		})
	}
	switch r.URL.Query().Get("sort") {
	case components.SortCreator:
		sortTracksBy(tracks, func(t *audio.Track) string {
			if len(t.Artists) == 0 {
				return ""
			}
			return t.Artists[0].Name
		})
	case components.SortTag:
		sortTracksBy(tracks, func(t *audio.Track) string {
			if len(t.Tags) == 0 {
				return ""
			}
			return t.Tags[0]
		})
	}
	limit, offset := pageParams(r)
	writeJSON(w, http.StatusOK, listOf(tracksJSON(viewer(r), tracks), limit, offset))
}

// sortTracksBy orders tracks by key, ignoring case, those without one (an
// empty key) last. It is stable, so ties keep the order they came in.
func sortTracksBy(tracks []*audio.Track, key func(*audio.Track) string) {
	slices.SortStableFunc(tracks, func(a, b *audio.Track) int {
		ka, kb := strings.ToLower(key(a)), strings.ToLower(key(b))
		switch {
		case ka == kb:
			return 0
		case ka == "":
			return 1
		case kb == "":
			return -1
		}
		return strings.Compare(ka, kb)
	})
}

func getTrackJSON(w http.ResponseWriter, r *http.Request) {
	id, ok := trackID(w, r)
	if !ok {
		return
	}
	t, err := db.GetTrack(viewer(r), id.String())
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	chapters, err := db.GetChapters(t.ID.String())
	if err == nil && t.IsEpisode() {
		err = db.AttachPositions([]*audio.Track{t})
	}
	if err != nil {
		serverError(w, err)
		return
	}
	d := jsonTrackDetail{
		jsonTrack:   trackJSON(viewer(r), t),
		Releases:    releasesJSON(viewer(r), t.Releases),
		Tags:        append([]string{}, t.Tags...),
		Description: t.Description,
		Chapters:    make([]jsonChapter, len(chapters)),
	}
	for i, c := range chapters {
		d.Chapters[i] = jsonChapter{c.Start.Seconds(), c.Title}
	}
	writeJSON(w, http.StatusOK, d)
}

// searchCap is how many of each kind /api/v1/search returns.
const searchCap = 20

func getSearchJSON(w http.ResponseWriter, r *http.Request) {
	v := viewer(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	res, err := db.Search(v, q, searchCap)
	if err != nil {
		serverError(w, err)
		return
	}
	if err := db.AttachPositions(res.Tracks); err != nil {
		serverError(w, err)
		return
	}
	// the search index has no playlists; at their number, names are matched here
	playlists, err := db.ListPlaylists(v)
	if err != nil {
		serverError(w, err)
		return
	}
	words := strings.Fields(strings.ToLower(q))
	playlists = slices.DeleteFunc(playlists, func(p *audio.Playlist) bool {
		return len(words) == 0 || !containsWords(p.Name, words)
	})
	writeJSON(w, http.StatusOK, jsonSearch{
		Tracks:    tracksJSON(v, res.Tracks),
		Artists:   artistsJSON(v, res.Artists),
		Releases:  releasesJSON(v, res.Releases),
		Playlists: playlistsJSON(v, playlists[:min(len(playlists), searchCap)]),
		Shows:     showsJSON(v, res.Shows),
	})
}
