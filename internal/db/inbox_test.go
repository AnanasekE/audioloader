package db

import (
	"bytes"
	"database/sql"
	"errors"
	"testing"
	"time"

	"audioloader/internal/audio"
)

func TestConfirmInboxRelease(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(All, "Massive Attack")
	owned := addTestTrack(t, "Teardrop", "teardrop bytes", a.ID.String())
	if err := SetFollow(a.ID.String(), "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", DefaultFilters()); err != nil {
		t.Fatal(err)
	}
	library, _ := ListTracks(All)
	credit := []MatchArtist{{ID: "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", Name: "Massive Attack"}}
	tracks := []InboxTrack{
		{Disc: 1, Position: 1, Title: "Angel", RecordingMBID: "r1", LengthMs: 380000, Artists: credit, SpotifyURL: "https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb"},
		{Disc: 1, Position: 2, Title: "Teardrop", RecordingMBID: "r3", LengthMs: 61000, Artists: credit},
		{Disc: 1, Position: 3, Title: "Group Four", LengthMs: 490000, Artists: append(credit, MatchArtist{ID: "x", Name: "Elizabeth Fraser"})},
	}
	for i := range tracks {
		tracks[i].OwnedTrackID = OwnedTrack(library, tracks[i])
	}
	if tracks[1].OwnedTrackID != owned || tracks[0].OwnedTrackID != "" {
		t.Fatalf("owned = %+v", tracks)
	}
	err := AddInboxRelease(InboxRelease{
		ReleaseGroupMBID: "rg1", ArtistID: a.ID.String(), ReleaseMBID: "rel1", Title: "Mezzanine", Type: "album", Date: "1998-04-20",
		Data: InboxData{Artists: credit, Tracks: tracks, SpotifyURL: "https://open.spotify.com/album/0sNOF9WDwhWunNAHPD3Baj"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if known, _ := InboxKnows("rg1"); !known {
		t.Fatal("inbox doesn't know rg1")
	}
	if n, _ := InboxCount(); n != 1 {
		t.Fatalf("inbox count = %d", n)
	}

	relID, placeholders, err := ConfirmInboxRelease("rg1")
	if err != nil {
		t.Fatal(err)
	}
	if len(placeholders) != 2 {
		t.Fatalf("placeholders = %v", placeholders)
	}
	rel, _ := GetRelease(All, relID)
	if rel.MBID != "rel1" || rel.ReleaseGroupMBID != "rg1" || len(rel.Tracks) != 3 {
		t.Fatalf("release = %+v", rel)
	}
	if rel.Tracks[1].Track.ID.String() != owned || !rel.Tracks[1].Track.HasFile || rel.Tracks[0].Track.HasFile {
		t.Fatalf("tracklist = %+v", rel.Tracks)
	}
	var albumLink, trackLink, noLink sql.NullString
	DB.QueryRow(`SELECT spotify_url FROM releases WHERE uuid = ?;`, relID).Scan(&albumLink)
	DB.QueryRow(`SELECT spotify_url FROM tracks WHERE uuid = ?;`, rel.Tracks[0].Track.ID.String()).Scan(&trackLink)
	DB.QueryRow(`SELECT spotify_url FROM tracks WHERE uuid = ?;`, rel.Tracks[2].Track.ID.String()).Scan(&noLink)
	if albumLink.String != "https://open.spotify.com/album/0sNOF9WDwhWunNAHPD3Baj" ||
		trackLink.String != "https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb" || noLink.Valid {
		t.Fatalf("spotify links = %v, %v, %v", albumLink, trackLink, noLink)
	}
	if len(rel.Tracks[2].Track.Artists) != 2 {
		t.Fatalf("featured artist missing: %+v", rel.Tracks[2].Track.Artists)
	}
	if n, _ := InboxCount(); n != 0 {
		t.Fatalf("inbox count after confirm = %d", n)
	}
	if _, _, err := ConfirmInboxRelease("rg1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("confirming twice err = %v", err)
	}
	downloads, err := ListDownloads(All, DownloadQueued)
	if err != nil || len(downloads) != 2 || downloads[0].Release.Title != "Mezzanine" {
		t.Fatalf("downloads = %+v, %v", downloads, err)
	}

	// the same release group now counts as in the library
	if id, _ := LibraryReleaseFor(a.ID.String(), "rg1", "whatever"); id != relID {
		t.Fatalf("LibraryReleaseFor = %q", id)
	}
}

func TestMergePlaceholder(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "rel", audio.ReleaseAlbum)
	existing := addTestTrack(t, "song", "same file")
	ph := addTestPlaceholder(t, r, "song", 1)
	DB.Exec(`UPDATE tracks SET mbid = 'rec' WHERE uuid = ?;`, ph)

	got, err := AttachFile(ph, bytes.NewReader([]byte("same file")), ".mp3", time.Second)
	if !errors.Is(err, ErrExists) || got != existing {
		t.Fatalf("AttachFile = %q, %v", got, err)
	}
	if err := MergePlaceholder(ph, existing); err != nil {
		t.Fatal(err)
	}
	rel, _ := GetRelease(All, r.ID.String())
	if len(rel.Tracks) != 1 || rel.Tracks[0].Track.ID.String() != existing || rel.Tracks[0].Track.MBID != "rec" {
		t.Fatalf("release tracks = %+v", rel.Tracks[0].Track)
	}
	if _, err := GetTrack(All, ph); !errors.Is(err, ErrNotFound) {
		t.Fatalf("placeholder still there: %v", err)
	}
}

func TestFollows(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(All, "A")
	b, _ := AddArtist(All, "B")
	if err := SetFollow(a.ID.String(), "mb-a", DefaultFilters()); err != nil {
		t.Fatal(err)
	}
	if err := SetFollow(b.ID.String(), "mb-a", DefaultFilters()); !errors.Is(err, ErrMBIDTaken) {
		t.Fatalf("duplicate mbid err = %v", err)
	}
	due, _ := FollowsDue(24 * time.Hour)
	if len(due) != 1 {
		t.Fatalf("due = %v", due)
	}
	MarkFollowChecked(a.ID.String())
	if due, _ := FollowsDue(24 * time.Hour); len(due) != 0 {
		t.Fatalf("due after check = %v", due)
	}
	f, err := GetFollow(a.ID.String())
	if err != nil || f.Artist.MBID != "mb-a" || len(f.Filters.Types) == 0 {
		t.Fatalf("follow = %+v, %v", f, err)
	}
}

func TestInboxOwnedCounts(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(All, "Artist")
	if err := SetFollow(a.ID.String(), "mb-a", DefaultFilters()); err != nil {
		t.Fatal(err)
	}
	add := func(rg string, owned ...bool) {
		var tracks []InboxTrack
		for i, o := range owned {
			tr := InboxTrack{Position: i + 1, Title: "t"}
			if o {
				tr.OwnedTrackID = "some-track"
			}
			tracks = append(tracks, tr)
		}
		if err := AddInboxRelease(InboxRelease{ReleaseGroupMBID: rg, ArtistID: a.ID.String(), Title: rg, Type: "album", Data: InboxData{Tracks: tracks}}); err != nil {
			t.Fatal(err)
		}
	}
	add("none", false, false)
	add("some", true, false, false)
	add("all", true, true)

	got := map[string][2]int{}
	list, err := ListInbox(InboxFilter{Status: "new"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range list {
		got[r.ReleaseGroupMBID] = [2]int{r.Owned(), len(r.Data.Tracks)}
	}
	want := map[string][2]int{"none": {0, 2}, "some": {1, 3}, "all": {2, 2}}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s owned/total = %v, want %v", k, got[k], w)
		}
	}
}

func TestRestoreInboxRelease(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(All, "Artist")
	r := InboxRelease{ReleaseGroupMBID: "rg", ArtistID: a.ID.String(), Title: "Album", Type: "album"}
	if err := AddInboxRelease(r); err != nil {
		t.Fatal(err)
	}
	if err := RestoreInboxRelease("rg"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restoring a new release err = %v", err)
	}
	if err := IgnoreInboxRelease("rg"); err != nil {
		t.Fatal(err)
	}
	if n, ig, _ := InboxCounts(); n != 0 || ig != 1 {
		t.Fatalf("counts = %d, %d", n, ig)
	}
	if err := RestoreInboxRelease("rg"); err != nil {
		t.Fatal(err)
	}
	// the daily check finds it again: still one row, and it knows it
	if known, _ := InboxKnows("rg"); !known {
		t.Fatal("inbox forgot the release")
	}
	if err := AddInboxRelease(r); err != nil {
		t.Fatal(err)
	}
	if n, ig, _ := InboxCounts(); n != 1 || ig != 0 {
		t.Fatalf("counts after restore = %d, %d", n, ig)
	}
}

func TestListInboxFilter(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(All, "Massive Attack")
	b, _ := AddArtist(All, "Portishead")
	for _, id := range []string{a.ID.String(), b.ID.String()} {
		if err := SetFollow(id, "mb-"+id, DefaultFilters()); err != nil {
			t.Fatal(err)
		}
	}
	owned := []InboxTrack{{Position: 1, Title: "x", OwnedTrackID: "t"}}
	for _, r := range []InboxRelease{
		{ReleaseGroupMBID: "1", ArtistID: a.ID.String(), Title: "Mezzanine", Type: "album", Date: "1998"},
		{ReleaseGroupMBID: "2", ArtistID: a.ID.String(), Title: "Teardrop", Type: "single", Date: "1998-04"},
		{ReleaseGroupMBID: "3", ArtistID: b.ID.String(), Title: "Dummy 100%", Type: "album", Date: "1994", Data: InboxData{Tracks: owned}},
		{ReleaseGroupMBID: "4", ArtistID: b.ID.String(), Title: "Undated", Type: "ep"},
	} {
		if err := AddInboxRelease(r); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(f InboxFilter) string {
		f.Status = "new"
		list, err := ListInbox(f)
		if err != nil {
			t.Fatal(err)
		}
		s := ""
		for _, r := range list {
			s += r.ReleaseGroupMBID
		}
		return s
	}
	for _, c := range []struct {
		f    InboxFilter
		want string
	}{
		{InboxFilter{}, "2134"},
		{InboxFilter{Sort: "oldest"}, "3124"},
		{InboxFilter{Type: "album"}, "13"},
		{InboxFilter{Type: "album", NoneOwned: true}, "1"},
		{InboxFilter{Q: "portis"}, "34"},
		{InboxFilter{Q: "tear"}, "2"},
		{InboxFilter{Q: "100%"}, "3"},
		{InboxFilter{Q: "%"}, "3"},
		{InboxFilter{Q: "nothing"}, ""},
	} {
		if got := ids(c.f); got != c.want {
			t.Errorf("%+v = %q, want %q", c.f, got, c.want)
		}
	}
}
