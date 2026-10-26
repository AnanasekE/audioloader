package db

import (
	"testing"

	"audioloader/internal/audio"
)

func TestArtistPageLists(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist("Main")
	b, _ := AddArtist("Other")
	own, _ := CreateRelease("Own Album", audio.ReleaseAlbum, "2020", []string{a.ID.String()})
	theirs, _ := CreateRelease("Their Album", audio.ReleaseAlbum, "2021", []string{b.ID.String()})
	onOwn := addTestTrack(t, "On Own", "1", a.ID.String())
	feature := addTestTrack(t, "Feature", "2", a.ID.String(), b.ID.String())
	loose := addTestTrack(t, "Loose", "3", a.ID.String())
	addTestTrack(t, "Not Main", "4", b.ID.String())
	// uploads without a release made singles; drop them to get loose tracks
	DB.Exec(`DELETE FROM releases WHERE type = 'single';`)
	PlaceTrack(onOwn, ReleaseLink{ReleaseID: own.ID.String(), Disc: 1, Position: 1})
	PlaceTrack(feature, ReleaseLink{ReleaseID: theirs.ID.String(), Disc: 1, Position: 1})

	rels, err := ListArtistReleases(All, a.ID.String())
	if err != nil || len(rels) != 1 || rels[0].Title != "Own Album" {
		t.Fatalf("releases = %+v, %v", rels, err)
	}
	tracks, err := ListArtistTracks(All, a.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tr := range tracks {
		got[tr.ID.String()] = true
	}
	if len(tracks) != 2 || !got[feature] || !got[loose] {
		t.Fatalf("tracks = %+v", tracks)
	}
}
