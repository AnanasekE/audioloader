package db

import (
	"errors"
	"testing"
)

func TestAppleImport(t *testing.T) {
	newTestDB(t)
	id, err := CreateAppleImport("https://music.apple.com/us/playlist/x/pl.abc")
	if err != nil {
		t.Fatal(err)
	}
	imp, err := GetAppleImport(id)
	if err != nil || imp.Status != ImportQueued || imp.Name != "" || imp.PlaylistID != "" {
		t.Fatalf("new import = %+v, %v", imp, err)
	}
	if _, err := GetAppleImport(id + 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing import: %v", err)
	}

	songs := []AppleImportTrack{
		{Title: "One", Artist: "A", Album: "X", DurationMs: 1000},
		{Title: "Two", Artist: "B & C", DurationMs: 2000},
	}
	if err := StartAppleImport(id, "Mine", 5, songs); err != nil {
		t.Fatal(err)
	}
	// a second start keeps what the first stored
	if err := StartAppleImport(id, "Other", 9, songs[:1]); err != nil {
		t.Fatal(err)
	}
	imp, _ = GetAppleImport(id)
	tracks, _ := AppleImportTracks(id)
	if imp.Name != "Mine" || imp.TrackCount != 5 || len(tracks) != 2 || tracks[1].Position != 2 || tracks[1].Title != "Two" || tracks[0].State != AppleSongPending {
		t.Fatalf("import = %+v, tracks = %+v", imp, tracks)
	}

	m := *tracks[0]
	m.RecordingID, m.ReleaseID, m.MatchTitle, m.MatchArtist, m.ReleaseTitle, m.Score, m.State = "rec", "rel", "One!", "A", "X", 0.93, AppleSongMatched
	m.Title = "ignored"
	if err := SetAppleTrackMatch(m); err != nil {
		t.Fatal(err)
	}
	if err := SetAppleTrackState(id, 2, AppleSongNotFound); err != nil {
		t.Fatal(err)
	}
	if err := SetAppleTrackState(id, 3, AppleSongAdded); !errors.Is(err, ErrNotFound) {
		t.Fatalf("state of a song that isn't there: %v", err)
	}
	tracks, _ = AppleImportTracks(id)
	got := tracks[0]
	if got.Title != "One" || got.RecordingID != "rec" || got.ReleaseID != "rel" || got.MatchTitle != "One!" || got.Score != 0.93 || got.State != AppleSongMatched {
		t.Fatalf("matched song = %+v", got)
	}
	counts, _ := AppleImportCounts(id)
	if counts[AppleSongMatched] != 1 || counts[AppleSongNotFound] != 1 || counts[AppleSongPending] != 0 {
		t.Fatalf("counts = %v", counts)
	}

	if err := SetAppleImportStatus(id, ImportFailed, "boom"); err != nil {
		t.Fatal(err)
	}
	p, _ := CreatePlaylist(Normal, "Mine")
	if err := SetAppleImportPlaylist(id, p.ID.String()); err != nil {
		t.Fatal(err)
	}
	id2, _ := CreateAppleImport("https://music.apple.com/us/playlist/y/pl.def")
	list, err := ListAppleImports(5)
	if err != nil || len(list) != 2 || list[0].ID != id2 || list[1].Status != ImportFailed || list[1].Error != "boom" || list[1].PlaylistID != p.ID.String() {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if list, _ := ListAppleImports(1); len(list) != 1 {
		t.Fatalf("limit: %d imports", len(list))
	}

	// deleting the playlist leaves the import
	if err := DeletePlaylist(Normal, p.ID.String()); err != nil {
		t.Fatal(err)
	}
	if imp, _ := GetAppleImport(id); imp.PlaylistID != "" {
		t.Errorf("playlist_id = %q after the playlist was deleted", imp.PlaylistID)
	}
}
