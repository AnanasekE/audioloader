package db

import (
	"testing"

	"audioloader/internal/audio"
)

// addYouTubeTrack adds a track as the YouTube import does: its own single.
func addYouTubeTrack(t *testing.T, title, artistID, sourceURL string) string {
	t.Helper()
	id := addTestTrack(t, title, title, artistID)
	if sourceURL != "" {
		if err := SetSourceURL(id, sourceURL); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// applyOn stores an applied match for the track on a release.
func applyOn(t *testing.T, trackID, releaseMBID string) {
	t.Helper()
	m := Match{Source: "search", Score: 0.95, RecordingID: "rec-" + trackID, Title: "x", ReleaseID: releaseMBID, ReleaseTitle: "Album", ReleaseType: "album", ReleaseDate: "2019-05-01"}
	if err := SaveMatches(trackID, []Match{m}, TagReview); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyMatch(trackID, m); err != nil {
		t.Fatal(err)
	}
}

func TestMoveSuggestions(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(All, "Artist")
	aid := a.ID.String()

	yt := addYouTubeTrack(t, "Song", aid, "https://www.youtube.com/watch?v=1")
	applyOn(t, yt, "rel-album")
	// the match is on the release the track already is on
	same := addYouTubeTrack(t, "Same", aid, "https://www.youtube.com/watch?v=2")
	DB.Exec(`UPDATE releases SET title = 'Album' WHERE uuid IN (SELECT release_id FROM release_tracks WHERE track_id = ?)`, same)
	applyOn(t, same, "rel-same") // the single takes the MBID: the titles match
	// not from YouTube
	other := addYouTubeTrack(t, "Other", aid, "https://example.com/a")
	applyOn(t, other, "rel-other")
	// no match applied
	addYouTubeTrack(t, "Plain", aid, "https://www.youtube.com/watch?v=4")

	list, err := MoveSuggestions(All)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].TrackID != yt {
		t.Fatalf("suggestions = %+v", list)
	}
	s := list[0]
	if s.TargetID != "rel-album" || s.TargetTitle != "Album" || s.TargetDate != "2019-05-01" || s.FromTitle != "Song" || !s.FromByImport || s.FromTracks != 1 || s.TargetInLibrary {
		t.Fatalf("suggestion = %+v", s)
	}

	// a library release with the MBID is reused, and the track stays listed
	r, _ := CreateRelease("Album", audio.ReleaseAlbum, "", []string{aid})
	DB.Exec(`UPDATE releases SET mbid = 'rel-album' WHERE uuid = ?`, r.ID.String())
	if list, _ = MoveSuggestions(All); len(list) != 1 || !list[0].TargetInLibrary {
		t.Fatalf("in library = %+v", list)
	}

	// dismissed for this target, but a different target shows again
	if err := DismissMove(yt, "rel-album"); err != nil {
		t.Fatal(err)
	}
	if list, _ = MoveSuggestions(All); len(list) != 0 {
		t.Fatalf("dismissed = %+v", list)
	}
	if err := DismissMove(yt, "rel-else"); err != nil {
		t.Fatal(err)
	}
	if list, _ = MoveSuggestions(All); len(list) != 1 {
		t.Fatalf("other target = %+v", list)
	}
	if err := DismissMove("nope", "x"); err != ErrNotFound {
		t.Fatalf("dismiss unknown = %v", err)
	}

	// once the track is on the target it is no longer suggested
	if err := PlaceTrack(yt, ReleaseLink{ReleaseID: r.ID.String(), Disc: 1, Position: 1}); err != nil {
		t.Fatal(err)
	}
	DB.Exec(`UPDATE tracks SET move_dismissed_release = NULL WHERE uuid = ?`, yt)
	if list, _ = MoveSuggestions(All); len(list) != 0 {
		t.Fatalf("on target = %+v", list)
	}
}
