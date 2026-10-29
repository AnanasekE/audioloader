package db

import (
	"testing"

	"audioloader/internal/audio"
)

func songOnRelease(title, rec string, position int) NewSong {
	credit := []MatchArtist{{ID: "artist-mbid", Name: "Ella Red"}}
	return NewSong{
		RecordingMBID: rec, Title: title, ISRC: "ISRC" + rec, LengthMs: 180000, Artists: credit,
		Release: &NewSongRelease{
			MBID: "rel1", ReleaseGroupMBID: "rg1", Title: "Muse", Type: "album", Date: "2020-01-02",
			SpotifyURL: "https://open.spotify.com/album/x", Artists: credit, Disc: 1, Position: position,
		},
	}
}

func TestAddSong(t *testing.T) {
	newTestDB(t)

	trackID, relID, isNew, err := AddSong(songOnRelease("Muse", "r1", 3))
	if err != nil || !isNew {
		t.Fatalf("AddSong = %v, %v", isNew, err)
	}
	rel, _ := GetRelease(All, relID)
	if rel.MBID != "rel1" || rel.ReleaseGroupMBID != "rg1" || rel.Type != audio.ReleaseAlbum || len(rel.Tracks) != 1 {
		t.Fatalf("release = %+v", rel)
	}
	if got := rel.Tracks[0]; got.Position != 3 || got.Track.HasFile || got.Track.MBID != "r1" || got.Track.ID.String() != trackID {
		t.Fatalf("placement = %+v", got)
	}
	if len(rel.Artists) != 1 || rel.Artists[0].MBID != "artist-mbid" {
		t.Fatalf("artists = %+v", rel.Artists)
	}
	queued, err := ListDownloads(All, DownloadQueued)
	if err != nil || len(queued) != 1 {
		t.Fatalf("downloads = %v, %v", queued, err)
	}

	// another song of the same release group lands on the same release
	other := songOnRelease("Other", "r2", 5)
	other.Release.MBID = "rel2"
	_, relID2, isNew, err := AddSong(other)
	if err != nil || isNew || relID2 != relID {
		t.Fatalf("second song: release %q, new %v, %v", relID2, isNew, err)
	}
	// a taken slot moves to the next free position on the disc
	_, relID3, _, err := AddSong(songOnRelease("Third", "r3", 3))
	if err != nil || relID3 != relID {
		t.Fatalf("third song: %q, %v", relID3, err)
	}
	rel, _ = GetRelease(All, relID)
	if len(rel.Tracks) != 3 || rel.Tracks[2].Position != 6 {
		t.Fatalf("tracks = %+v", rel.Tracks)
	}
}

func TestAddSongArtistByName(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(All, "Ella Red")
	if _, _, _, err := AddSong(songOnRelease("Muse", "r1", 1)); err != nil {
		t.Fatal(err)
	}
	got, err := GetArtist(All, a.ID.String())
	if err != nil || got.MBID != "artist-mbid" {
		t.Fatalf("artist = %+v, %v", got, err)
	}
}

func TestAddSongSingle(t *testing.T) {
	newTestDB(t)
	s := songOnRelease("Muse", "r1", 1)
	s.Release = nil
	trackID, relID, isNew, err := AddSong(s)
	if err != nil || !isNew {
		t.Fatalf("AddSong = %v, %v", isNew, err)
	}
	rel, _ := GetRelease(All, relID)
	if !rel.IsSingle() || rel.Title != "Muse" || len(rel.Tracks) != 1 || rel.Tracks[0].Track.ID.String() != trackID {
		t.Fatalf("release = %+v", rel)
	}
}
