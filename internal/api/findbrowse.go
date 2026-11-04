package api

import (
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"audioloader/internal/addsong"
	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/mb"
	"audioloader/internal/tracking"
)

// groupTypeOrder is the order of the type sections on an artist's page;
// release groups of any other type come last as "Other".
var groupTypeOrder = []string{"Album", "EP", "Single", "Broadcast"}

// findSections groups release groups by primary type, newest first within a
// type, and marks what the library has of each.
func findSections(c *mb.Client, groups []mb.ReleaseGroup) ([]components.FindSection, error) {
	ids := make([]string, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
	}
	counts, err := db.OwnedReleaseCounts(ids)
	if err != nil {
		return nil, err
	}
	byType := map[string][]components.FindGroup{}
	for _, g := range groups {
		typ := g.PrimaryType
		if !contains(groupTypeOrder, typ) {
			typ = "Other"
		}
		n := counts[g.ID]
		byType[typ] = append(byType[typ], components.FindGroup{
			Group: g, CoverURL: c.CoverArtURL + "/release-group/" + g.ID + "/front-250", Have: n.Have, Total: n.Total,
		})
	}
	var out []components.FindSection
	for _, typ := range append(append([]string{}, groupTypeOrder...), "Other") {
		gs := byType[typ]
		if len(gs) == 0 {
			continue
		}
		// dates sort as text; groups without one go last
		sort.SliceStable(gs, func(i, j int) bool {
			a, b := gs[i].Group.FirstReleaseDate, gs[j].Group.FirstReleaseDate
			if (a == "") != (b == "") {
				return b == ""
			}
			return a > b
		})
		title := typ
		if typ != "Other" && typ != "Broadcast" {
			title += "s"
		}
		out = append(out, components.FindSection{Title: title, Groups: gs})
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// artistGroupsPage reads one page of the artist's release groups.
func artistGroupsPage(r *http.Request, mbid string, offset int) (components.FindArtistPage, error) {
	c := mb.Default()
	p := components.FindArtistPage{MBID: mbid}
	groups, total, err := c.BrowseReleaseGroups(r.Context(), mbid, offset)
	if err != nil {
		return p, err
	}
	p.Total = total
	if offset+len(groups) < total && len(groups) > 0 {
		p.More = offset + len(groups)
	}
	p.Sections, err = findSections(c, groups)
	return p, err
}

// getFindArtist lists an artist's release groups by type, 100 at a time.
func getFindArtist(w http.ResponseWriter, r *http.Request) {
	mbid := validMBID(r.PathValue("mbid"))
	if mbid == "" {
		http.NotFound(w, r)
		return
	}
	p, err := artistGroupsPage(r, mbid, 0)
	if errors.Is(err, mb.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Println("find artist:", err)
		http.Error(w, "couldn't read MusicBrainz: "+err.Error(), http.StatusBadGateway)
		return
	}
	p.Name = strings.TrimSpace(r.URL.Query().Get("name"))
	if p.Name == "" {
		p.Name = "Artist"
	}
	followed, err := db.FollowedArtists([]string{mbid})
	if err != nil {
		serverError(w, err)
		return
	}
	p.Following = followed[mbid]
	render(w, r, components.FindArtistBrowse(p))
}

// getFindArtistGroups answers the More button with the next page.
func getFindArtistGroups(w http.ResponseWriter, r *http.Request) {
	mbid := validMBID(r.PathValue("mbid"))
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if mbid == "" || err != nil || offset < 0 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	p, err := artistGroupsPage(r, mbid, offset)
	if errors.Is(err, mb.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Println("find artist groups:", err)
		http.Error(w, "couldn't read MusicBrainz: "+err.Error(), http.StatusBadGateway)
		return
	}
	render(w, r, components.FindGroups(p))
}

// getFindReleaseGroup shows the tracklist of the release that stands for a
// release group.
func getFindReleaseGroup(w http.ResponseWriter, r *http.Request) {
	id := validMBID(r.PathValue("mbid"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	c := mb.Default()
	releases, _, err := c.BrowseReleases(r.Context(), id, 0)
	if errors.Is(err, mb.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Println("find release group:", err)
		http.Error(w, "couldn't read MusicBrainz: "+err.Error(), http.StatusBadGateway)
		return
	}
	pick := tracking.PickRelease(releases)
	if pick == nil {
		// no official release: any one with a tracklist will do
		for i := range releases {
			if len(releases[i].Media) > 0 {
				pick = &releases[i]
				break
			}
		}
	}
	if pick == nil {
		http.NotFound(w, r)
		return
	}
	// the browse answer has no work relations, which tell a cover from an original
	rel, err := c.GetRelease(r.Context(), pick.ID)
	if errors.Is(err, mb.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Println("find release:", err)
		http.Error(w, "couldn't read MusicBrainz: "+err.Error(), http.StatusBadGateway)
		return
	}
	p, err := findReleasePage(c, rel, id)
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.FindReleaseBrowse(*p))
}

// findReleasePage is rel's tracklist with the library's tracks marked.
func findReleasePage(c *mb.Client, rel *mb.Release, groupID string) (*components.FindReleasePage, error) {
	songs := addsong.ReleaseSongs(rel)
	ids := make([]string, len(songs))
	for i, s := range songs {
		ids[i] = s.Song.RecordingMBID
	}
	owned, err := db.OwnedRecordings(ids)
	if err != nil {
		return nil, err
	}
	p := &components.FindReleasePage{
		GroupID:   groupID,
		ReleaseID: rel.ID,
		Title:     rel.Title,
		Artists:   rel.ArtistCredit.String(),
		Year:      releaseYear(firstNonEmpty(rel.ReleaseGroup.FirstReleaseDate, rel.Date)),
		Type:      rel.ReleaseGroup.PrimaryType,
		CoverURL:  c.CoverArtURL + "/release/" + rel.ID + "/front-250",
	}
	if len(rel.ArtistCredit) > 0 {
		p.ArtistMBID, p.ArtistName = rel.ArtistCredit[0].Artist.ID, rel.ArtistCredit[0].Artist.Name
		followed, err := db.FollowedArtists([]string{p.ArtistMBID})
		if err != nil {
			return nil, err
		}
		p.Following = followed[p.ArtistMBID]
	}
	for _, s := range songs {
		cand := components.SongCandidate{
			RecordingID: s.Song.RecordingMBID,
			Title:       s.Track.Title,
			Artists:     s.Artists,
			Length:      time.Duration(s.Track.LengthMs) * time.Millisecond,
			ReleaseID:   rel.ID,
			Disc:        s.Disc,
			Position:    s.Position,
		}
		if o, ok := owned[cand.RecordingID]; ok {
			cand.OwnedTrackID, cand.OwnedReleaseID, cand.OwnedHasFile = o.TrackID, o.ReleaseID, o.HasFile
		} else {
			p.Missing++
		}
		p.Tracks = append(p.Tracks, cand)
	}
	return p, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
