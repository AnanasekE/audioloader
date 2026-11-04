package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/mb"
)

// youtubeImportOn is a YouTube import, on its own single, whose applied
// match is on the album "rel1"; the fake MusicBrainz serves that album.
func youtubeImportOn(t *testing.T) string {
	t.Helper()
	rel := mb.Release{
		ID: "rel1", Title: "The Album", Date: "2019-05-01",
		ReleaseGroup: mb.ReleaseGroup{ID: "rg1", PrimaryType: "Album"},
		ArtistCredit: mb.Credits{{Name: "Artist", Artist: mb.Artist{ID: "art1", Name: "Artist"}}},
		Media:        []mb.Medium{{Position: 1, Tracks: []mb.Track{{Position: 3, Title: "Song", Recording: mb.Recording{ID: "rec1"}}}}},
	}
	b, err := json.Marshal(rel)
	if err != nil {
		t.Fatal(err)
	}
	fakeMusicBrainz(t, map[string]string{"/release/rel1": string(b)})

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
		ReleaseTitle: "The Album", ReleaseType: "album", ReleaseDate: "2019-05-01"}
	if err := db.SaveMatches(res.TrackID, []db.Match{m}, db.TagReview); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ApplyMatch(res.TrackID, m); err != nil {
		t.Fatal(err)
	}
	return res.TrackID
}

func TestTaggingMoveSection(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	id := youtubeImportOn(t)

	body := do(h, "GET", "/tagging", nil).Body.String()
	for _, want := range []string{"On another release", "Single – Song", "Album – The Album (2019)", "/ui/tagging/" + id + "/move", "Keep here"} {
		if !strings.Contains(body, want) {
			t.Errorf("tagging page lacks %q", want)
		}
	}
	if body := do(h, "GET", "/tracks", nil).Body.String(); !strings.Contains(body, "Move to The Album") {
		t.Error("track menu lacks Move to")
	}

	rec := do(h, "POST", "/ui/tagging/"+id+"/move/dismiss", nil)
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Location") == "" {
		t.Fatalf("dismiss = %d", rec.Code)
	}
	if body := do(h, "GET", "/tagging", nil).Body.String(); strings.Contains(body, "On another release") {
		t.Error("dismissed suggestion is still listed")
	}
	if body := do(h, "GET", "/tracks", nil).Body.String(); strings.Contains(body, "Move to The Album") {
		t.Error("dismissed suggestion is still in the menu")
	}
	if rec := do(h, "POST", "/ui/tagging/"+id+"/move/dismiss", nil); rec.Code != http.StatusNotFound {
		t.Errorf("second dismiss = %d, want 404", rec.Code)
	}
}

func TestTaggingMoveRoute(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	id := youtubeImportOn(t)

	rec := do(h, "POST", "/ui/tagging/"+id+"/move", nil)
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Location") == "" {
		t.Fatalf("move = %d %s", rec.Code, rec.Body)
	}
	tr, err := db.GetTrack(db.All, id)
	if err != nil || len(tr.Releases) != 1 || tr.Releases[0].MBID != "rel1" {
		t.Fatalf("track = %+v, %v", tr, err)
	}
	rel, _ := db.GetRelease(db.All, tr.Releases[0].ID.String())
	if rel.Tracks[0].Disc != 1 || rel.Tracks[0].Position != 3 {
		t.Errorf("placed at %+v", rel.Tracks[0])
	}
	if rec := do(h, "POST", "/ui/tagging/"+id+"/move", nil); rec.Code != http.StatusNotFound {
		t.Errorf("second move = %d, want 404", rec.Code)
	}
}

func TestTaggingMoveRecordingGone(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	id := youtubeImportOn(t)
	// MusicBrainz now lists another recording at that spot
	fakeMusicBrainz(t, map[string]string{"/release/rel1": `{"id":"rel1","title":"The Album","media":[{"position":1,"tracks":[{"position":1,"title":"x","recording":{"id":"other"}}]}]}`})

	if rec := do(h, "POST", "/ui/tagging/"+id+"/move", nil); rec.Code != http.StatusConflict {
		t.Errorf("move = %d, want 409", rec.Code)
	}
}
