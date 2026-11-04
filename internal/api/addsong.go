package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"audioloader/internal/addsong"
	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/downloads"
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
	return candidateOn(c, library, rec, tagging.PickRelease(rec.Releases))
}

// candidateOn is rec as one search result, on rel (nil: no release).
func candidateOn(c *mb.Client, library []*audio.Track, rec mb.Recording, rel *mb.Release) (components.SongCandidate, error) {
	cand := candidateBase(c, rec, rel)
	var err error
	cand.OwnedTrackID, cand.OwnedReleaseID, cand.OwnedHasFile, err = addsong.Owned(library, addsong.SongTrack(rec))
	return cand, err
}

// candidateBase is rec as one search result, on rel (nil: no release), not
// yet checked against the library.
func candidateBase(c *mb.Client, rec mb.Recording, rel *mb.Release) components.SongCandidate {
	cand := components.SongCandidate{
		RecordingID: rec.ID,
		Title:       rec.Title,
		Artists:     rec.ArtistCredit.String(),
		Length:      time.Duration(rec.Length) * time.Millisecond,
		CoverURL:    "/assets/img/default_track.png",
	}
	if rel != nil {
		cand.ReleaseID, cand.ReleaseTitle = rel.ID, rel.Title
		cand.Year = releaseYear(rel.Date)
		cand.CoverURL = c.CoverArtURL + "/release/" + rel.ID + "/front-250"
	}
	return cand
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
	songs := addsong.ReleaseSongs(rel)
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
		cand.OwnedTrackID, cand.OwnedReleaseID, cand.OwnedHasFile, err = addsong.Owned(library, s.Track)
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

	song, it, serr := songFor(r.Context(), recordingID, releaseID)
	if serr != nil {
		http.Error(w, serr.msg, serr.status)
		return
	}

	library, err := db.ListTracks(db.Normal)
	if err != nil {
		serverError(w, err)
		return
	}
	if _, err := addsong.Queue(library, song, it); errors.Is(err, addsong.ErrAlreadyOwned) {
		http.Error(w, "this song is already in your library", http.StatusConflict)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/downloads")
}

// songError is a failure of songFor, with the status to answer.
type songError struct {
	status int
	msg    string
}

// songFor reads what db.AddSong needs for a recording from MusicBrainz: on
// releaseID's tracklist at the recording's place when given, else as a
// single.
func songFor(ctx context.Context, recordingID, releaseID string) (db.NewSong, db.InboxTrack, *songError) {
	c := mb.Default()
	if releaseID != "" {
		rel, err := c.GetRelease(ctx, releaseID)
		if errors.Is(err, mb.ErrNotFound) {
			return db.NewSong{}, db.InboxTrack{}, &songError{http.StatusBadRequest, "that release isn't on MusicBrainz"}
		}
		if err != nil {
			return db.NewSong{}, db.InboxTrack{}, &songError{http.StatusBadGateway, "couldn't read MusicBrainz: " + err.Error()}
		}
		for _, rs := range addsong.ReleaseSongs(rel) {
			if rs.Song.RecordingMBID == recordingID {
				return rs.Song, rs.Track, nil
			}
		}
		return db.NewSong{}, db.InboxTrack{}, &songError{http.StatusBadRequest, "that recording isn't on this release"}
	}
	song, it, err := addsong.Single(ctx, c, recordingID)
	if errors.Is(err, mb.ErrNotFound) {
		return db.NewSong{}, db.InboxTrack{}, &songError{http.StatusBadRequest, "that recording isn't on MusicBrainz"}
	}
	if err != nil {
		return db.NewSong{}, db.InboxTrack{}, &songError{http.StatusBadGateway, "couldn't read MusicBrainz: " + err.Error()}
	}
	return song, it, nil
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
	songs := addsong.ReleaseSongs(rel)
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
		if _, err := addsong.Queue(library, s.Song, s.Track); err != nil && !errors.Is(err, addsong.ErrAlreadyOwned) {
			serverError(w, err)
			return
		}
	}
	renderRelease(w, r, rel)
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
