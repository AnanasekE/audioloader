package db

import (
	"bytes"
	"errors"
	"testing"

	"audioloader/internal/audio"
)

func TestSecretTracks(t *testing.T) {
	newTestDB(t)
	pub, _ := AddArtist("Public Artist")
	hid, _ := AddArtist("Hidden Artist")
	r, _ := CreateRelease("Mixed", audio.ReleaseAlbum, "", []string{pub.ID.String()})
	only, _ := CreateRelease("Only Secret", audio.ReleaseSingle, "", []string{hid.ID.String()})
	empty, _ := CreateRelease("Empty", audio.ReleaseEP, "", []string{pub.ID.String()})

	a := addTestTrack(t, "One", "one", pub.ID.String())
	s := addTestTrack(t, "ZZSECRET", "secret", hid.ID.String())
	b := addTestTrack(t, "Three", "three", pub.ID.String())
	if err := SetReleaseTracks(All, r.ID.String(), []ReleaseTrackInput{{a, 1}, {s, 1}, {b, 1}}); err != nil {
		t.Fatal(err)
	}
	if err := SetReleaseTracks(All, only.ID.String(), []ReleaseTrackInput{{s, 1}}); err != nil {
		t.Fatal(err)
	}
	if err := SetSecret(s, true); err != nil {
		t.Fatal(err)
	}

	// the track itself
	if _, err := GetTrack(Public, s); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTrack = %v", err)
	}
	if tr, err := GetTrack(All, s); err != nil || !tr.Secret {
		t.Fatalf("unlocked GetTrack = %+v, %v", tr, err)
	}
	for _, tr := range must(ListTracks(Public)) {
		if tr.Title == "ZZSECRET" {
			t.Fatal("listed")
		}
	}

	// a mixed release hides it and numbers the rest without a gap
	rel, _ := GetRelease(Public, r.ID.String())
	if len(rel.Tracks) != 2 || rel.Tracks[1].Position != 2 || rel.Tracks[1].Track.Title != "Three" {
		t.Fatalf("mixed release = %+v", rel.Tracks)
	}
	if rel, _ := GetRelease(All, r.ID.String()); len(rel.Tracks) != 3 || rel.Tracks[2].Position != 3 {
		t.Fatalf("unlocked release = %+v", rel.Tracks)
	}

	// what only secret tracks make up is hidden; empty things stay
	if _, err := GetRelease(Public, only.ID.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("secret-only release = %v", err)
	}
	if _, err := GetRelease(Public, empty.ID.String()); err != nil {
		t.Fatalf("empty release hidden: %v", err)
	}
	if _, err := GetArtist(Public, hid.ID.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("secret-only artist = %v", err)
	}
	// Mixed, Empty and the singles of One and Three; not ZZSECRET's single
	for _, rel := range must(ListReleases(Public)) {
		if rel.Title == "ZZSECRET" || rel.Title == "Only Secret" {
			t.Fatalf("listed %q", rel.Title)
		}
	}
	if n := len(must(ListReleases(Public))); n != 4 {
		t.Fatalf("releases = %d", n)
	}
	if n := len(must(ListArtists(Public))); n != 1 {
		t.Fatalf("artists = %d", n)
	}

	// search
	if res, _ := Search(Public, "zzsecret", 10); !res.Empty() {
		t.Fatalf("search found it: %+v", res)
	}
	if res, _ := Search(Public, "hidden", 10); !res.Empty() {
		t.Fatalf("search found the artist: %+v", res.Artists)
	}
	if res, _ := Search(All, "zzsecret", 10); len(res.Tracks) != 1 {
		t.Fatal("unlocked search misses it")
	}

	// saving the tracklist while locked keeps the secret track's slot
	if err := SetReleaseTracks(Public, r.ID.String(), []ReleaseTrackInput{{b, 1}, {a, 1}}); err != nil {
		t.Fatal(err)
	}
	rel, _ = GetRelease(All, r.ID.String())
	if len(rel.Tracks) != 3 || rel.Tracks[1].Track.Title != "ZZSECRET" || rel.Tracks[0].Track.Title != "Three" {
		t.Fatalf("after locked save = %s", titlesOf(rel.Tracks))
	}

	// duplicate checks don't give it away
	fp, _ := FingerprintOf(bytes.NewReader([]byte("secret")))
	if exists, _ := TrackExists(Public, fp); exists {
		t.Fatal("TrackExists reveals it")
	}
	_, err := AddTrack(Public, []string{pub.ID.String()}, &audio.Track{Title: "x"}, bytes.NewReader([]byte("secret")), ".mp3", nil, "")
	if !errors.Is(err, ErrHidden) {
		t.Fatalf("AddTrack err = %v", err)
	}
}

func TestSecretPlaylistSlots(t *testing.T) {
	newTestDB(t)
	a := addTestTrack(t, "A", "a")
	s := addTestTrack(t, "S", "s")
	b := addTestTrack(t, "B", "b")
	p, _ := CreatePlaylist("p")
	SetPlaylistTracks(All, p.ID.String(), []string{a, s, b})
	SetSecret(s, true)

	pl, _ := GetPlaylist(Public, p.ID.String())
	if titles(pl.Tracks) != "A,B" {
		t.Fatalf("locked playlist = %s", titles(pl.Tracks))
	}
	if list, _ := ListPlaylists(Public); list[0].Count != 2 {
		t.Fatalf("count = %d", list[0].Count)
	}
	SetPlaylistTracks(Public, p.ID.String(), []string{b, a})
	pl, _ = GetPlaylist(All, p.ID.String())
	if titles(pl.Tracks) != "B,S,A" {
		t.Fatalf("after locked save = %s", titles(pl.Tracks))
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func titlesOf(rts []*audio.ReleaseTrack) string {
	var ts []*audio.Track
	for _, rt := range rts {
		ts = append(ts, rt.Track)
	}
	return titles(ts)
}
