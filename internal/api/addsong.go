package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/components"
	"audioloader/internal/db"
	"audioloader/internal/downloads"
	"audioloader/internal/mb"
	"audioloader/internal/sources/ytdlp"
	"audioloader/internal/tagging"
)

func getAddSongPage(w http.ResponseWriter, r *http.Request) {
	render(w, r, components.AddSongPage(strings.TrimSpace(r.URL.Query().Get("q"))))
}

// searchSongsMusicBrainz lists MusicBrainz's 5 best recordings for what was
// typed, marking the ones the library already has.
func searchSongsMusicBrainz(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "type an artist and a title", http.StatusBadRequest)
		return
	}
	c := mb.Default()
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
		cand := components.SongCandidate{
			RecordingID: rec.ID,
			Title:       rec.Title,
			Artists:     rec.ArtistCredit.String(),
			Length:      time.Duration(rec.Length) * time.Millisecond,
			CoverURL:    "/assets/img/default_track.png",
		}
		if rel := tagging.PickRelease(rec.Releases); rel != nil {
			cand.ReleaseID, cand.ReleaseTitle = rel.ID, rel.Title
			cand.Year = rel.Date
			if len(cand.Year) > 4 {
				cand.Year = cand.Year[:4]
			}
			cand.CoverURL = c.CoverArtURL + "/release/" + rel.ID + "/front-250"
		}
		cand.OwnedTrackID, cand.OwnedReleaseID, cand.OwnedHasFile, err = ownedSong(library, songTrack(rec))
		if err != nil {
			serverError(w, err)
			return
		}
		found = append(found, cand)
	}
	render(w, r, components.SongCandidates(q, found))
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
		for _, m := range rel.Media {
			for _, t := range m.Tracks {
				if found || t.Recording.ID != recordingID {
					continue
				}
				found = true
				it = songTrack(t.Recording)
				it.Title, it.LengthMs = t.Title, firstPositive(t.Length, t.Recording.Length)
				song = db.NewSong{
					RecordingMBID: recordingID,
					Title:         it.Title,
					ISRC:          it.ISRC,
					LengthMs:      it.LengthMs,
					SpotifyURL:    mb.SpotifyURL(t.Recording.Relations, "track"),
					Artists:       it.Artists,
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
				}
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
		}
	}

	// another tab may have added it since the search
	library, err := db.ListTracks(db.Normal)
	if err != nil {
		serverError(w, err)
		return
	}
	if id, _, _, err := ownedSong(library, it); err != nil {
		serverError(w, err)
		return
	} else if id != "" {
		http.Error(w, "this song is already in your library", http.StatusConflict)
		return
	}

	trackID, newReleaseID, newRelease, err := db.AddSong(song)
	if err != nil {
		serverError(w, err)
		return
	}
	if err := downloads.Enqueue(trackID); err != nil {
		serverError(w, err)
		return
	}
	if newRelease {
		if err := tagging.EnqueueCover(newReleaseID); err != nil {
			serverError(w, err)
			return
		}
	}
	hxLocation(w, "/downloads")
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
	for i, e := range found {
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
	render(w, r, components.SongYouTubeCandidates(found, titles, artists, owned))
}

// addSongFromYouTube imports a picked video as a single, with the title and
// artist the person confirmed.
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
	ids, err := db.ResolveArtists(db.Normal, []string{artist})
	if err != nil {
		serverError(w, err)
		return
	}
	imp := downloads.YouTubeImport{VideoID: ref.Video, Title: title, ArtistIDs: []string{ids[artist]}}
	if err := downloads.EnqueueImport(imp); err != nil {
		serverError(w, err)
		return
	}
	hxLocation(w, "/upload/youtube")
}
