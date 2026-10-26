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
	a, _ := AddArtist("Massive Attack")
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
	a, _ := AddArtist("A")
	b, _ := AddArtist("B")
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
