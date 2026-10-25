package db

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"audioloader/internal/audio"
)

func TestEpisodes(t *testing.T) {
	newTestDB(t)
	show, err := ResolveShow("The Show", "Host")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := ResolveShow("the show", "")
	if again.ID != show.ID {
		t.Fatal("ResolveShow made a second show")
	}
	ep1, err := AddEpisode(show.ID.String(), &audio.Track{Title: "Ep 1", Duration: time.Hour, Published: "2026-01-01"}, bytes.NewReader([]byte("ep1")), ".mp3")
	if err != nil {
		t.Fatal(err)
	}
	ep2, _ := AddEpisode(show.ID.String(), &audio.Track{Title: "Ep 2", Duration: time.Hour, Published: "2026-02-01"}, bytes.NewReader([]byte("ep2")), ".mp3")
	if _, err := AddEpisode(show.ID.String(), &audio.Track{Title: "dup"}, bytes.NewReader([]byte("ep1")), ".mp3"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate err = %v", err)
	}
	music := addTestTrack(t, "Song", "song")

	// episodes stay out of the music library and its search
	tracks, _ := ListTracks(All)
	if len(tracks) != 1 || tracks[0].ID.String() != music {
		t.Fatalf("library = %+v", tracks)
	}
	if res, _ := Search(All, "ep", 10); len(res.Tracks) != 0 {
		t.Fatalf("episode in search: %+v", res.Tracks)
	}
	if res, _ := Search(All, "show", 10); len(res.Shows) != 1 {
		t.Fatalf("show not in search: %+v", res.Shows)
	}

	s, err := GetShow(All, show.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if titles(s.Episodes) != "Ep 2,Ep 1" || s.Count != 2 || s.Unplayed != 2 || !s.Episodes[0].IsEpisode() {
		t.Fatalf("show = %+v %s", s, titles(s.Episodes))
	}

	// positions and continue listening
	if err := SavePosition(ep1, 600, false); err != nil {
		t.Fatal(err)
	}
	if err := SavePosition(ep2, 3590, true); err != nil {
		t.Fatal(err)
	}
	cont, _ := ContinueListening(All, 10)
	if len(cont) != 1 || cont[0].ID.String() != ep1 || cont[0].Position != 10*time.Minute {
		t.Fatalf("continue = %+v", cont)
	}
	s, _ = GetShow(All, show.ID.String())
	if s.Unplayed != 1 || !s.Episodes[0].Finished {
		t.Fatalf("after positions: unplayed %d, %+v", s.Unplayed, s.Episodes[0])
	}
	ClearPosition(ep2)
	if s, _ := GetShow(All, show.ID.String()); s.Unplayed != 2 {
		t.Fatalf("after clear: %d", s.Unplayed)
	}

	// chapters
	chs := []Chapter{{Start: 0, Title: "Intro"}, {Start: 90 * time.Second, Title: "Main"}}
	if err := SetChapters(ep1, chs); err != nil {
		t.Fatal(err)
	}
	got, _ := GetChapters(ep1)
	if len(got) != 2 || got[1].Title != "Main" || got[1].Start != 90*time.Second {
		t.Fatalf("chapters = %+v", got)
	}
}
