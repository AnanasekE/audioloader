package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
	"audioloader/internal/sources/ytdlp"
	"audioloader/internal/tagging"
)

// searchSongsMusicBrainz lists MusicBrainz's 5 best recordings for what was
// typed, marking the ones the library already has. A MusicBrainz release or
// recording link lists exactly what it points to instead.
func searchSongsMusicBrainz(w http.ResponseWriter, r *http.Request) {
	listMusicBrainz(w, r, r.URL.Query().Get("q"))
}

const msgMBLink = "Paste a release or recording link."

// listMusicBrainz is searchSongsMusicBrainz for q given as a value.
func listMusicBrainz(w http.ResponseWriter, r *http.Request, q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		http.Error(w, "type an artist and a title", http.StatusBadRequest)
		return
	}
	c := mb.Default()
	if kind, id, ok := mb.ParseLink(q); ok {
		switch kind {
		case "release":
			listRelease(w, r, id)
		case "recording":
			listRecording(w, r, id)
		default:
			http.Error(w, msgMBLink, http.StatusBadRequest)
		}
		return
	}
	recs, err := c.SearchSongs(r.Context(), q, 5)
	if err != nil {
		http.Error(w, "couldn't search MusicBrainz: "+err.Error(), http.StatusBadGateway)
		return
	}
	library, err := db.ListTracks(db.Normal)
	if err != nil {
		serverError(w, err)
		return
	}
	found := make([]components.SongCandidate, 0, len(recs))
	for _, rec := range recs {
		cand, err := recordingCandidate(c, library, rec)
		if err != nil {
			serverError(w, err)
			return
		}
		found = append(found, cand)
	}
	render(w, r, components.SongCandidates(q, nil, found))
}

// recordingCandidate is rec as one search result, on the release
// tagging.PickRelease picks for it.
func recordingCandidate(c *mb.Client, library []*audio.Track, rec mb.Recording) (components.SongCandidate, error) {
	cand := components.SongCandidate{
		RecordingID: rec.ID,
		Title:       rec.Title,
		Artists:     rec.ArtistCredit.String(),
		Length:      time.Duration(rec.Length) * time.Millisecond,
		CoverURL:    "/assets/img/default_track.png",
	}
	if rel := tagging.PickRelease(rec.Releases); rel != nil {
		cand.ReleaseID, cand.ReleaseTitle = rel.ID, rel.Title
		cand.Year = releaseYear(rel.Date)
		cand.CoverURL = c.CoverArtURL + "/release/" + rel.ID + "/front-250"
	}
	var err error
	cand.OwnedTrackID, cand.OwnedReleaseID, cand.OwnedHasFile, err = ownedSong(library, songTrack(rec))
	return cand, err
}

func releaseYear(date string) string {
	if len(date) > 4 {
		return date[:4]
	}
	return date
}

// listRecording shows the one recording a link points to.
func listRecording(w http.ResponseWriter, r *http.Request, id string) {
	c := mb.Default()
	rec, err := c.GetRecording(r.Context(), id)
	if errors.Is(err, mb.ErrNotFound) {
		http.Error(w, "that recording isn't on MusicBrainz", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "couldn't read MusicBrainz: "+err.Error(), http.StatusBadGateway)
		return
	}
	library, err := db.ListTracks(db.Normal)
	if err != nil {
		serverError(w, err)
		return
	}
	cand, err := recordingCandidate(c, library, *rec)
	if err != nil {
		serverError(w, err)
		return
	}
	render(w, r, components.SongCandidates("", nil, []components.SongCandidate{cand}))
}

// listRelease shows every track of the release a link points to, each
// added to that very release.
func listRelease(w http.ResponseWriter, r *http.Request, id string) {
	rel, err := mb.Default().GetRelease(r.Context(), id)
	if errors.Is(err, mb.ErrNotFound) {
		http.Error(w, "that release isn't on MusicBrainz", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "couldn't read MusicBrainz: "+err.Error(), http.StatusBadGateway)
		return
	}
	renderRelease(w, r, rel)
}

// renderRelease answers rel's header and tracklist, with what the library
// has marked as owned.
func renderRelease(w http.ResponseWriter, r *http.Request, rel *mb.Release) {
	c := mb.Default()
	library, err := db.ListTracks(db.Normal)
	if err != nil {
		serverError(w, err)
		return
	}
	songs := releaseSongs(rel)
	head := &components.SongRelease{
		ID:       rel.ID,
		Title:    rel.Title,
		Artists:  rel.ArtistCredit.String(),
		Year:     releaseYear(rel.Date),
		CoverURL: c.CoverArtURL + "/release/" + rel.ID + "/front-250",
		Tracks:   len(songs),
	}
	found := make([]components.SongCandidate, 0, len(songs))
	for _, s := range songs {
		cand := components.SongCandidate{
			RecordingID: s.Track.RecordingMBID,
			Title:       s.Track.Title,
			Artists:     s.Artists,
			Length:      time.Duration(s.Track.LengthMs) * time.Millisecond,
			CoverURL:    head.CoverURL,
			ReleaseID:   rel.ID,
			Disc:        s.Disc,
			Position:    s.Position,
		}
		cand.OwnedTrackID, cand.OwnedReleaseID, cand.OwnedHasFile, err = ownedSong(library, s.Track)
		if err != nil {
			serverError(w, err)
			return
		}
		if !cand.Owned() {
			head.Missing++
		}
		found = append(found, cand)
	}
	render(w, r, components.SongCandidates("", head, found))
}

// releaseSong is one track of a MusicBrainz release, ready to add to the
// library on that release.
type releaseSong struct {
	Song           db.NewSong
	Track          db.InboxTrack // what the library's ownership rule compares
	Artists        string
	Disc, Position int
}

// releaseSongs lists rel's tracks in disc and position order.
func releaseSongs(rel *mb.Release) []releaseSong {
	var out []releaseSong
	for _, m := range rel.Media {
		for _, t := range m.Tracks {
			it := songTrack(t.Recording)
			it.Title, it.LengthMs = t.Title, firstPositive(t.Length, t.Recording.Length)
			out = append(out, releaseSong{
				Track:    it,
				Artists:  t.Recording.ArtistCredit.String(),
				Disc:     max(m.Position, 1),
				Position: t.Position,
				Song: db.NewSong{
					RecordingMBID: t.Recording.ID,
					Title:         it.Title,
					ISRC:          it.ISRC,
					LengthMs:      it.LengthMs,
					SpotifyURL:    mb.SpotifyURL(t.Recording.Relations, "track"),
					Artists:       it.Artists,
					CoverWorkID:   coverWorkID(t.Recording),
					Release: &db.NewSongRelease{
						MBID:             rel.ID,
						ReleaseGroupMBID: rel.ReleaseGroup.ID,
						Title:            rel.Title,
						Type:             tagging.ReleaseType(rel.ReleaseGroup),
						Date:             rel.Date,
						SpotifyURL:       mb.SpotifyURL(rel.Relations, "album"),
						Artists:          tagging.MatchArtists(rel.ArtistCredit),
						Disc:             max(m.Position, 1),
						Position:         t.Position,
					},
				},
			})
		}
	}
	return out
}

func coverWorkID(rec mb.Recording) string {
	if w := rec.CoverWork(); w != nil {
		return w.ID
	}
	return ""
}

// songTrack is the recording as an inbox tracklist entry, which is what the
// library's ownership rule compares.
func songTrack(rec mb.Recording) db.InboxTrack {
	it := db.InboxTrack{
		Title:         rec.Title,
		RecordingMBID: rec.ID,
		LengthMs:      rec.Length,
		Artists:       tagging.MatchArtists(rec.ArtistCredit),
	}
	if len(rec.ISRCs) > 0 {
		it.ISRC = rec.ISRCs[0]
	}
	return it
}

// ownedSong finds the library's track for t, or its queued placeholder. The
// release is the one the track is on; "" for both when the library has none.
func ownedSong(library []*audio.Track, t db.InboxTrack) (trackID, releaseID string, hasFile bool, err error) {
	if id := db.OwnedTrack(library, t); id != "" {
		for _, lt := range library {
			if lt.ID.String() == id && len(lt.Releases) > 0 {
				releaseID = lt.Releases[0].ID.String()
			}
		}
		return id, releaseID, true, nil
	}
	if t.RecordingMBID == "" {
		return "", "", false, nil
	}
	trackID, releaseID, err = db.QueuedSong(t.RecordingMBID)
	return trackID, releaseID, false, err
}

// addSongFromMusicBrainz adds a picked recording as a placeholder on its
// release (or as a single without one) and queues its download.
func addSongFromMusicBrainz(w http.ResponseWriter, r *http.Request) {
	recordingID := validMBID(r.FormValue("recording_id"))
	if recordingID == "" {
		http.Error(w, "invalid recording id", http.StatusBadRequest)
		return
	}
	releaseID := ""
	if r.FormValue("release_id") != "" {
		if releaseID = validMBID(r.FormValue("release_id")); releaseID == "" {
			http.Error(w, "invalid release id", http.StatusBadRequest)
			return
		}
	}

	c := mb.Default()
	var song db.NewSong
	var it db.InboxTrack
	if releaseID != "" {
		rel, err := c.GetRelease(r.Context(), releaseID)
		if errors.Is(err, mb.ErrNotFound) {
			http.Error(w, "that release isn't on MusicBrainz", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "couldn't read MusicBrainz: "+err.Error(), http.StatusBadGateway)
			return
		}
		found := false
		for _, rs := range releaseSongs(rel) {
			if !found && rs.Song.RecordingMBID == recordingID {
				found, song, it = true, rs.Song, rs.Track
			}
		}
		if !found {
			http.Error(w, "that recording isn't on this release", http.StatusBadRequest)
			return
		}
	} else {
		rec, err := c.GetRecording(r.Context(), recordingID)
		if errors.Is(err, mb.ErrNotFound) {
			http.Error(w, "that recording isn't on MusicBrainz", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "couldn't read MusicBrainz: "+err.Error(), http.StatusBadGateway)
			return
		}
		it = songTrack(*rec)
		song = db.NewSong{
			RecordingMBID: rec.ID,
			Title:         rec.Title,
			ISRC:          it.ISRC,
			LengthMs:      rec.Length,
			SpotifyURL:    mb.SpotifyURL(rec.Relations, "track"),
			Artists:       it.Artists,
			CoverWorkID:   coverWorkID(*rec),
		}
	}

	library, err := db.ListTracks(db.Normal)
	if err != nil {
		serverError(w, err)
		return
	}
	if err := queueSong(library, song, it); errors.Is(err, errAlreadyOwned) {
		http.Error(w, "this song is already in your library", http.StatusConflict)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/downloads")
}

// maxAddAll caps how many tracks "Add all" queues at once.
const maxAddAll = 100

// addReleaseFromMusicBrainz adds every track of a release the library
// doesn't have yet, all onto that release, and answers the refreshed list.
func addReleaseFromMusicBrainz(w http.ResponseWriter, r *http.Request) {
	releaseID := validMBID(r.FormValue("release_id"))
	if releaseID == "" {
		http.Error(w, "invalid release id", http.StatusBadRequest)
		return
	}
	rel, err := mb.Default().GetRelease(r.Context(), releaseID)
	if errors.Is(err, mb.ErrNotFound) {
		http.Error(w, "that release isn't on MusicBrainz", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "couldn't read MusicBrainz: "+err.Error(), http.StatusBadGateway)
		return
	}
	songs := releaseSongs(rel)
	if len(songs) > maxAddAll {
		http.Error(w, fmt.Sprintf("this release has %d tracks; add at most %d at once", len(songs), maxAddAll), http.StatusBadRequest)
		return
	}
	library, err := db.ListTracks(db.Normal)
	if err != nil {
		serverError(w, err)
		return
	}
	for _, s := range songs {
		if err := queueSong(library, s.Song, s.Track); err != nil && !errors.Is(err, errAlreadyOwned) {
			serverError(w, err)
			return
		}
	}
	renderRelease(w, r, rel)
}

var errAlreadyOwned = errors.New("the library already has this song")

// queueSong adds song (it is the same song as the library's ownership rule
// sees it) as a placeholder and queues its download, unless the library has
// it or has it queued.
func queueSong(library []*audio.Track, song db.NewSong, it db.InboxTrack) error {
	// another tab may have added it since the search
	if id, _, _, err := ownedSong(library, it); err != nil {
		return err
	} else if id != "" {
		return errAlreadyOwned
	}
	trackID, newReleaseID, newRelease, err := db.AddSong(song)
	if err != nil {
		return err
	}
	if err := downloads.Enqueue(trackID); err != nil {
		return err
	}
	if song.CoverWorkID != "" {
		payload := map[string]string{"TrackID": trackID, "WorkID": song.CoverWorkID, "RecordingID": song.RecordingMBID}
		if err := jobs.EnqueueAt(tagging.JobCoverOfApply, trackID, payload, time.Time{}); err != nil {
			return err
		}
	}
	if newRelease {
		return tagging.EnqueueCover(newReleaseID)
	}
	return nil
}

func firstPositive(a, b int) int {
	if a > 0 {
		return a
	}
	return b
}

// searchSongsYouTube lists the top 5 YouTube results for what was typed,
// with the title and artist each would get.
func searchSongsYouTube(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "type an artist and a title", http.StatusBadRequest)
		return
	}
	d := downloads.Default()
	if d.YT == nil {
		http.Error(w, d.YTErr.Error(), http.StatusServiceUnavailable)
		return
	}
	found, err := d.YT.Search(r.Context(), q, 5)
	if err != nil {
		http.Error(w, "couldn't search YouTube: "+err.Error(), http.StatusBadGateway)
		return
	}
	titles, artists, owned := make([]string, len(found)), make([]string, len(found)), make([]string, len(found))
	covers := make([]downloads.CoverGuess, len(found))
	for i, e := range found {
		covers[i] = downloads.GuessCover(e.Title)
		artists[i] = downloads.GuessArtist(e)
		titles[i] = downloads.CleanTitle(e.Title, []string{artists[i]})
		id, err := db.TrackBySourceURL(ytdlp.VideoURL(e.ID))
		if err != nil {
			serverError(w, err)
			return
		}
		if id != "" {
			owned[i] = "/tracks"
			if t, err := db.GetTrack(db.Normal, id); err == nil && len(t.Releases) > 0 {
				owned[i] = "/releases/" + t.Releases[0].ID.String()
			}
		}
	}
	render(w, r, components.SongYouTubeCandidates(found, titles, artists, owned, covers))
}

// importCover reads the "This is a cover" part of a YouTube add form. It
// returns nil when the box isn't ticked, and writes a 400 for bad values.
func importCover(w http.ResponseWriter, r *http.Request) (*db.CoverEdit, bool) {
	if r.FormValue("is_cover") == "" {
		return nil, true
	}
	return coverFromForm(w, r)
}

// addSongFromYouTube imports a picked video as a single, with the title and
// artist the person confirmed, and the original if it is a cover.
func addSongFromYouTube(w http.ResponseWriter, r *http.Request) {
	ref, err := ytdlp.Parse(ytdlp.VideoURL(r.FormValue("video_id")))
	if err != nil {
		http.Error(w, "invalid video id", http.StatusBadRequest)
		return
	}
	title, artist := strings.TrimSpace(r.FormValue("title")), strings.TrimSpace(r.FormValue("artist"))
	if title == "" || artist == "" {
		http.Error(w, "a song needs a title and an artist", http.StatusBadRequest)
		return
	}
	if id, err := db.TrackBySourceURL(ytdlp.VideoURL(ref.Video)); err != nil {
		serverError(w, err)
		return
	} else if id != "" {
		http.Error(w, "this video is already in your library", http.StatusConflict)
		return
	}
	cover, ok := importCover(w, r)
	if !ok {
		return
	}
	ids, err := db.ResolveArtists(db.Normal, []string{artist})
	if err != nil {
		serverError(w, err)
		return
	}
	imp := downloads.YouTubeImport{VideoID: ref.Video, Title: title, ArtistIDs: []string{ids[artist]}, Cover: cover}
	if err := downloads.EnqueueImport(imp); err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/add")
}
