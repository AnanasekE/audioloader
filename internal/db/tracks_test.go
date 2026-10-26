package db

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"
	"uuid"

	"audioloader/internal/audio"
)

// addTestPlaceholder adds a track without a file on release r.
func addTestPlaceholder(t *testing.T, r *audio.Release, title string, position int) string {
	t.Helper()
	id := "00000000-0000-0000-0000-00000000000" + string(rune('0'+position))
	if _, err := DB.Exec(`INSERT INTO tracks(uuid, title, duration_seconds, has_file) VALUES(?, ?, 60, 0);`, id, title); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO release_tracks(release_id, track_id, disc, position) VALUES(?, ?, 1, ?);`, r.ID.String(), id, position); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPlaceholderTracks(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "rel", audio.ReleaseAlbum)
	id := addTestPlaceholder(t, r, "missing", 1)

	tracks, err := ListTracks(All)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 0 {
		t.Fatalf("placeholder listed in library: %v", tracks)
	}
	rel, err := GetRelease(All, r.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(rel.Tracks) != 1 || rel.Tracks[0].Track.HasFile {
		t.Fatalf("release tracks = %+v", rel.Tracks)
	}

	if _, err := AttachFile(id, bytes.NewReader([]byte("audio")), ".mp3", 90*time.Second); err != nil {
		t.Fatal(err)
	}
	tracks, _ = ListTracks(All)
	if len(tracks) != 1 || !tracks[0].HasFile || tracks[0].Duration != 90*time.Second {
		t.Fatalf("after attach: %+v", tracks)
	}
	if _, err := AttachFile(id, bytes.NewReader([]byte("other")), ".mp3", time.Second); !errors.Is(err, ErrHasFile) {
		t.Fatalf("second attach err = %v", err)
	}
}

func TestAttachFileDuplicate(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "rel", audio.ReleaseAlbum)
	existing := addTestTrack(t, "song", "same")
	id := addTestPlaceholder(t, r, "song", 1)

	got, err := AttachFile(id, bytes.NewReader([]byte("same")), ".mp3", time.Second)
	if !errors.Is(err, ErrExists) || got != existing {
		t.Fatalf("got %q, %v; want %q, ErrExists", got, err, existing)
	}
}

func TestUpdateTrack(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(All, "Old Artist")
	b, _ := AddArtist(All, "New Artist")
	id := addTestTrack(t, "Old Title", "audio", a.ID.String())

	err := UpdateTrack(All, id, TrackEdit{Title: "Brand New Title", Kind: audio.KindMusic, ArtistIDs: []string{b.ID.String()}})
	if err != nil {
		t.Fatal(err)
	}
	tr, _ := GetTrack(All, id)
	if tr.Title != "Brand New Title" || len(tr.Artists) != 1 || tr.Artists[0].ID != b.ID {
		t.Fatalf("track = %+v", tr)
	}
	res, err := Search(All, "brand new", 5)
	if err != nil || len(res.Tracks) != 1 {
		t.Fatalf("search new title = %+v, %v", res, err)
	}
	if res, _ := Search(All, "old title", 5); len(res.Tracks) != 0 {
		t.Fatalf("search old title = %+v", res.Tracks)
	}

	// a song can't become an episode, a bad artist changes nothing
	if err := UpdateTrack(All, id, TrackEdit{Title: "x", Kind: audio.KindEpisode}); !errors.Is(err, ErrBadEdit) {
		t.Fatalf("to episode = %v", err)
	}
	err = UpdateTrack(All, id, TrackEdit{Title: "x", Kind: audio.KindMusic, ArtistIDs: []string{"nope"}})
	if !errors.Is(err, ErrBadReference) {
		t.Fatalf("bad artist = %v", err)
	}
	if tr, _ := GetTrack(All, id); tr.Title != "Brand New Title" || len(tr.Artists) != 1 {
		t.Fatalf("failed edit changed the track: %+v", tr)
	}

	// normal mode can't edit, or even find, a secret track
	SetSecret(id, true)
	if err := UpdateTrack(Normal, id, TrackEdit{Title: "x", Kind: audio.KindMusic}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("normal mode edit = %v", err)
	}
	off := false
	if err := UpdateTrack(Secret, id, TrackEdit{Title: "Brand New Title", Kind: audio.KindMusic, ArtistIDs: []string{b.ID.String()}, Secret: &off}); err != nil {
		t.Fatal(err)
	}
	if tr, _ := GetTrack(Normal, id); tr == nil {
		t.Fatal("still secret")
	}
}

func TestDeleteTrack(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(All, "Artist")
	r, _ := CreateRelease("Album", audio.ReleaseAlbum, "", []string{a.ID.String()})
	one := addTestTrack(t, "One", "one", a.ID.String())
	two := addTestTrack(t, "Two", "two", a.ID.String())
	three := addTestTrack(t, "Three", "three", a.ID.String())
	if err := SetReleaseTracks(All, r.ID.String(), []ReleaseTrackInput{{one, 1}, {two, 1}, {three, 1}}); err != nil {
		t.Fatal(err)
	}
	p, _ := CreatePlaylist(All, "Mix")
	if err := SetPlaylistTracks(All, p.ID.String(), []string{two, three}); err != nil {
		t.Fatal(err)
	}
	SavePosition(two, 5, false)
	SaveMatches(two, []Match{{Source: "search", Score: 0.7, RecordingID: "rec", Title: "Two"}}, TagReview)
	path, err := TrackFilePath(uuid.MustParse(two))
	if err != nil {
		t.Fatal(err)
	}

	if err := DeleteTrack(All, two); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	if _, err := GetTrack(All, two); !errors.Is(err, ErrNotFound) {
		t.Fatalf("track = %v", err)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM release_tracks WHERE track_id = '` + two + `'`,
		`SELECT COUNT(*) FROM playlist_tracks WHERE track_id = '` + two + `'`,
		`SELECT COUNT(*) FROM playback_positions WHERE track_id = '` + two + `'`,
		`SELECT COUNT(*) FROM tag_matches WHERE track_id = '` + two + `'`,
		`SELECT COUNT(*) FROM track_artist_map WHERE track_id = '` + two + `'`,
	} {
		var n int
		if err := DB.QueryRow(q).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s = %d, %v", q, n, err)
		}
	}

	// the others keep their places; the release stays
	rel, _ := GetRelease(All, r.ID.String())
	if len(rel.Tracks) != 2 || rel.Tracks[0].Position != 1 || rel.Tracks[1].Position != 3 {
		t.Fatalf("release tracks = %+v", rel.Tracks)
	}
	if err := DeleteTrack(All, two); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
	// a locked viewer can't delete a secret track
	SetSecret(three, true)
	if err := DeleteTrack(Normal, three); !errors.Is(err, ErrNotFound) {
		t.Fatalf("locked delete = %v", err)
	}
}
