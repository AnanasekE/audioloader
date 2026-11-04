package api

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/mb"
	"audioloader/internal/tagging"
)

// findLimit is how many hits a Find search shows.
const findLimit = 25

// findForm reads the Find form from the query; an unknown mode is a song search.
func findForm(r *http.Request) components.FindForm {
	q := r.URL.Query()
	f := components.FindForm{
		Mode:   q.Get("mode"),
		Q:      strings.TrimSpace(q.Get("q")),
		Artist: strings.TrimSpace(q.Get("artist")),
		Type:   q.Get("type"),
		Year:   strings.TrimSpace(q.Get("year")),
	}
	if f.Mode != mb.FindRelease && f.Mode != mb.FindArtist {
		f.Mode = mb.FindSong
	}
	return f
}

// getFindPage is the Find page; a search in the address runs on load.
func getFindPage(w http.ResponseWriter, r *http.Request) {
	f := findForm(r)
	var res *components.FindResults
	if f.Q != "" {
		var err error
		if res, err = findSearch(r, f); err != nil {
			serverError(w, err)
			return
		}
	}
	render(w, r, components.FindPage(f, res))
}

// getFindResults answers a search from the form with the results only, and
// puts the page's address (not this one) in the browser's.
func getFindResults(w http.ResponseWriter, r *http.Request) {
	f := findForm(r)
	if f.Q == "" {
		http.Error(w, "type something to search for", http.StatusBadRequest)
		return
	}
	res, err := findSearch(r, f)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("HX-Push-Url", "/add/find?"+findQuery(f))
	render(w, r, components.FindResultList(f, res))
}

// findQuery is f as the page's query string.
func findQuery(f components.FindForm) string {
	v := url.Values{"mode": {f.Mode}, "q": {f.Q}}
	if f.Mode != mb.FindArtist {
		for k, s := range map[string]string{"artist": f.Artist, "type": f.Type, "year": f.Year} {
			if s != "" {
				v.Set(k, s)
			}
		}
	}
	return v.Encode()
}

// findSearch runs the search for f. A MusicBrainz failure comes back as
// Err in the results, any other error as err.
func findSearch(r *http.Request, f components.FindForm) (*components.FindResults, error) {
	c := mb.Default()
	query := mb.FindQuery(f.Mode, mb.FindFields{Text: f.Q, Artist: f.Artist, Type: f.Type, Year: f.Year})
	res := &components.FindResults{}
	fail := func(err error) (*components.FindResults, error) {
		log.Println("find:", err)
		res.Err = "Couldn't search MusicBrainz."
		return res, nil
	}
	switch f.Mode {
	case mb.FindRelease:
		groups, err := c.SearchReleaseGroups(r.Context(), query, findLimit)
		if err != nil {
			return fail(err)
		}
		ids := make([]string, len(groups))
		for i, g := range groups {
			ids[i] = g.ID
		}
		counts, err := db.OwnedReleaseCounts(ids)
		if err != nil {
			return nil, err
		}
		for _, g := range groups {
			n := counts[g.ID]
			res.Groups = append(res.Groups, components.FindGroup{
				Group: g, CoverURL: c.CoverArtURL + "/release-group/" + g.ID + "/front-250", Have: n.Have, Total: n.Total,
			})
		}
	case mb.FindArtist:
		artists, err := c.SearchArtists(r.Context(), query, findLimit)
		if err != nil {
			return fail(err)
		}
		ids := make([]string, len(artists))
		for i, a := range artists {
			ids[i] = a.ID
		}
		followed, err := db.FollowedArtists(ids)
		if err != nil {
			return nil, err
		}
		for _, a := range artists {
			res.Artists = append(res.Artists, components.FindArtist{Artist: a, Following: followed[a.ID]})
		}
	default:
		recs, err := c.SearchRecordings(r.Context(), query, findLimit)
		if err != nil {
			return fail(err)
		}
		ids := make([]string, len(recs))
		for i, rec := range recs {
			ids[i] = rec.ID
		}
		owned, err := db.OwnedRecordings(ids)
		if err != nil {
			return nil, err
		}
		for _, rec := range recs {
			cand := candidateBase(c, rec, tagging.PickRelease(rec.Releases))
			if o, ok := owned[rec.ID]; ok {
				cand.OwnedTrackID, cand.OwnedReleaseID, cand.OwnedHasFile = o.TrackID, o.ReleaseID, o.HasFile
			}
			res.Songs = append(res.Songs, cand)
		}
	}
	return res, nil
}
