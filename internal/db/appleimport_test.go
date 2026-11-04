package db

import (
	"errors"
	"testing"
	"time"
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

func TestAppleFollow(t *testing.T) {
	newTestDB(t)
	url := "https://music.apple.com/us/playlist/x/pl.abc"
	id, _ := CreateAppleImport(url)
	if err := StartAppleImport(id, "Mine", 3, []AppleImportTrack{
		{AppleID: "11", Title: "One", Artist: "A"},
		{AppleID: "22", Title: "Two", Artist: "B"},
		{Title: "Old", Artist: "C"}, // from before IDs were stored
	}); err != nil {
		t.Fatal(err)
	}
	tracks, _ := AppleImportTracks(id)
	if tracks[0].AppleID != "11" || tracks[2].AppleID != "" {
		t.Fatalf("apple ids = %+v", tracks)
	}

	f, err := FollowApplePlaylist(id)
	if err != nil || f.Name != "Mine" || f.ImportID != id || f.LastCheckedAt != "" {
		t.Fatalf("follow = %+v, %v", f, err)
	}
	again, err := FollowApplePlaylist(id)
	if err != nil || again.ID != f.ID {
		t.Fatalf("following twice = %+v, %v", again, err)
	}
	if got, err := AppleFollowOfImport(id); err != nil || got.ID != f.ID {
		t.Fatalf("follow of import = %+v, %v", got, err)
	}
	if _, err := FollowApplePlaylist(id + 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("follow of a missing import: %v", err)
	}
	seen, _ := AppleFollowSeen(f.ID)
	if len(seen) != 2 || !seen["11"] || !seen["22"] {
		t.Fatalf("seen = %v", seen)
	}

	due, _ := AppleFollowsDue(time.Hour)
	if len(due) != 1 || due[0] != f.ID {
		t.Fatalf("due before a check = %v", due)
	}
	if err := MarkAppleFollowChecked(f.ID, "Not public any more"); err != nil {
		t.Fatal(err)
	}
	if due, _ = AppleFollowsDue(time.Hour); len(due) != 0 {
		t.Fatalf("due after a check = %v", due)
	}
	if due, _ = AppleFollowsDue(0); len(due) != 1 {
		t.Fatalf("due with every = 0: %v", due)
	}
	f, _ = GetAppleFollow(f.ID)
	if f.LastError != "Not public any more" || f.LastCheckedAt == "" {
		t.Fatalf("checked follow = %+v", f)
	}

	pos, err := AddAppleFollowSongs(f.ID, []AppleImportTrack{{AppleID: "33", Title: "Three", Artist: "D"}})
	if err != nil || len(pos) != 1 || pos[0] != 4 {
		t.Fatalf("positions = %v, %v", pos, err)
	}
	if seen, _ = AppleFollowSeen(f.ID); !seen["33"] {
		t.Fatalf("seen after add = %v", seen)
	}
	if _, err := AddAppleFollowSongs(f.ID+1, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("add to a missing follow: %v", err)
	}
	if err := SetAppleTrackState(id, 4, AppleSongUnsure); err != nil {
		t.Fatal(err)
	}
	if err := SetAppleTrackState(id, 3, AppleSongNotFound); err != nil {
		t.Fatal(err)
	}
	list, _ := ListAppleFollows()
	if len(list) != 1 || list[0].ToPick != 2 {
		t.Fatalf("list = %+v", list)
	}
	if n, err := AppleFollowsToPick(); err != nil || n != 1 {
		t.Fatalf("follows to pick = %d, %v", n, err)
	}

	SetAppleTrackState(id, 4, AppleSongAdded)
	SetAppleTrackState(id, 3, AppleSongOwned)
	if n, _ := AppleFollowsToPick(); n != 0 {
		t.Fatalf("follows to pick with everything picked = %d", n)
	}
	if err := DeleteAppleFollow(f.ID); err != nil {
		t.Fatal(err)
	}
	if seen, _ = AppleFollowSeen(f.ID); len(seen) != 0 {
		t.Fatalf("seen after delete = %v", seen)
	}
	if imp, err := GetAppleImport(id); err != nil || imp.Name != "Mine" {
		t.Fatalf("import after unfollow = %+v, %v", imp, err)
	}
	if err := DeleteAppleFollow(f.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}
