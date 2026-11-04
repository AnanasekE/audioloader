package tagging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/mb"
)

// moveRelease is the album the fake MusicBrainz serves as "rel1".
var moveRelease = mb.Release{
	ID: "rel1", Title: "Album", Date: "2019-05-01",
	ReleaseGroup: mb.ReleaseGroup{ID: "rg1", PrimaryType: "Album"},
	ArtistCredit: mb.Credits{{Name: "Artist", Artist: mb.Artist{ID: "art1", Name: "Artist"}}},
	Media: []mb.Medium{{Position: 1, Tracks: []mb.Track{
		{Position: 1, Title: "Opener", Recording: mb.Recording{ID: "rec0"}},
		{Position: 2, Title: "Song", Recording: mb.Recording{ID: "rec1"}},
	}}},
}

func moveClient(t *testing.T) *mb.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/2/release/rel1" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(moveRelease)
	}))
	t.Cleanup(srv.Close)
	c := mb.New()
	c.MusicBrainzURL, c.CoverArtURL = srv.URL+"/ws/2", srv.URL+"/caa"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

// youtubeImport is a YouTube import whose applied match is on rel1. Its
// release is the single made at import.
func youtubeImport(t *testing.T) (trackID, singleID string, artistID string) {
	t.Helper()
	a, err := db.AddArtist(db.All, "Artist")
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.AddTrack(db.All, []string{a.ID.String()}, &audio.Track{Title: "Song", Duration: time.Minute},
		bytes.NewReader([]byte("song")), ".mp3", nil, "2018-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetSourceURL(res.TrackID, "https://www.youtube.com/watch?v=abc"); err != nil {
		t.Fatal(err)
	}
	m := db.Match{Source: "search", Score: 0.95, RecordingID: "rec1", Title: "Song", ReleaseID: "rel1", ReleaseGroupID: "rg1",
		ReleaseTitle: "Album", ReleaseType: "album", ReleaseDate: "2019-05-01"}
	if err := db.SaveMatches(res.TrackID, []db.Match{m}, db.TagReview); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ApplyMatch(res.TrackID, m); err != nil {
		t.Fatal(err)
	}
	tr, err := db.GetTrack(db.All, res.TrackID)
	if err != nil || len(tr.Releases) != 1 {
		t.Fatalf("track = %+v, %v", tr, err)
	}
	return res.TrackID, tr.Releases[0].ID.String(), a.ID.String()
}

// placement is the track's release, disc and position, after checking it is on one release.
func placement(t *testing.T, trackID string) (*audio.Release, int, int) {
	t.Helper()
	tr, err := db.GetTrack(db.All, trackID)
	if err != nil || len(tr.Releases) != 1 {
		t.Fatalf("track = %+v, %v", tr, err)
	}
	rel, err := db.GetRelease(db.All, tr.Releases[0].ID.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range rel.Tracks {
		if rt.Track.ID.String() == trackID {
			return rel, rt.Disc, rt.Position
		}
	}
	t.Fatalf("track not on its release %+v", rel)
	return nil, 0, 0
}

func TestMoveCreatesReleaseAndRemovesSingle(t *testing.T) {
	dbtest.New(t)
	trackID, singleID, _ := youtubeImport(t)

	if err := Move(context.Background(), moveClient(t), trackID); err != nil {
		t.Fatal(err)
	}
	rel, disc, pos := placement(t, trackID)
	if rel.MBID != "rel1" || rel.ReleaseGroupMBID != "rg1" || rel.Title != "Album" || rel.Type != audio.ReleaseAlbum || rel.Date != "2019-05-01" || disc != 1 || pos != 2 {
		t.Fatalf("release = %+v disc %d pos %d", rel, disc, pos)
	}
	if len(rel.Tracks) != 1 {
		t.Fatalf("the other tracks aren't added: %+v", rel.Tracks)
	}
	if _, err := db.GetRelease(db.All, singleID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("the single is still there: %v", err)
	}
	if list, _ := db.MoveSuggestions(db.All); len(list) != 0 {
		t.Fatalf("still suggested: %+v", list)
	}
	if err := Move(context.Background(), moveClient(t), trackID); !errors.Is(err, ErrNoMove) {
		t.Fatalf("second move = %v", err)
	}
}

func TestMoveReusesReleaseAndMergesPlaceholder(t *testing.T) {
	dbtest.New(t)
	trackID, _, _ := youtubeImport(t)
	artists := []db.MatchArtist{{ID: "art1", Name: "Artist"}}
	target := &db.NewSongRelease{MBID: "rel1", ReleaseGroupMBID: "rg1", Title: "Album", Type: "album", Artists: artists, Disc: 1, Position: 1}
	_, releaseID, _, err := db.AddSong(db.NewSong{RecordingMBID: "rec0", Title: "Opener", Artists: artists, Release: target})
	if err != nil {
		t.Fatal(err)
	}
	// a placeholder for the very recording, on its own slot
	target2 := *target
	target2.Position = 2
	placeholder, _, _, err := db.AddSong(db.NewSong{RecordingMBID: "rec1", Title: "Song", Artists: artists, Release: &target2})
	if err != nil {
		t.Fatal(err)
	}
	pl, _ := db.CreatePlaylist(db.All, "Mix")
	if err := db.SetPlaylistTracks(db.All, pl.ID.String(), []string{placeholder}); err != nil {
		t.Fatal(err)
	}

	if err := Move(context.Background(), moveClient(t), trackID); err != nil {
		t.Fatal(err)
	}
	rel, _, pos := placement(t, trackID)
	if rel.ID.String() != releaseID || pos != 2 || len(rel.Tracks) != 2 {
		t.Fatalf("release = %+v pos %d", rel, pos)
	}
	if _, err := db.GetTrack(db.All, placeholder); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("placeholder is still there: %v", err)
	}
	got, err := db.GetPlaylist(db.All, pl.ID.String())
	if err != nil || len(got.Tracks) != 1 || got.Tracks[0].ID.String() != trackID {
		t.Fatalf("playlist = %+v, %v", got, err)
	}
}

func TestMoveKeepsOldReleaseWithOtherTracks(t *testing.T) {
	dbtest.New(t)
	trackID, singleID, artistID := youtubeImport(t)
	res, err := db.AddTrack(db.All, []string{artistID}, &audio.Track{Title: "B-side", Duration: time.Minute},
		bytes.NewReader([]byte("b")), ".mp3", &db.ReleaseLink{ReleaseID: singleID, Disc: 1, Position: 2}, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := Move(context.Background(), moveClient(t), trackID); err != nil {
		t.Fatal(err)
	}
	old, err := db.GetRelease(db.All, singleID)
	if err != nil || len(old.Tracks) != 1 || old.Tracks[0].Track.ID.String() != res.TrackID {
		t.Fatalf("old release = %+v, %v", old, err)
	}
	if rel, _, _ := placement(t, trackID); rel.MBID != "rel1" {
		t.Fatalf("release = %+v", rel)
	}
}

func TestMoveRecordingNotOnRelease(t *testing.T) {
	dbtest.New(t)
	trackID, singleID, _ := youtubeImport(t)
	moveRelease.Media[0].Tracks[1].Recording.ID = "other"
	t.Cleanup(func() { moveRelease.Media[0].Tracks[1].Recording.ID = "rec1" })

	if err := Move(context.Background(), moveClient(t), trackID); !errors.Is(err, ErrNotOnRelease) {
		t.Fatalf("move = %v", err)
	}
	if _, err := db.GetRelease(db.All, singleID); err != nil {
		t.Fatalf("nothing should change: %v", err)
	}
}
