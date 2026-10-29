package ytdlp

import (
	"context"
	"testing"
	"time"

	"audioloader/internal/sources"
)

func TestBestSong(t *testing.T) {
	want := sources.Want{Title: "Teardrop", Artists: []string{"Massive Attack"}, Duration: 331 * time.Second}
	song := func(title string, secs float64, channel string, artists ...string) Entry {
		return Entry{ID: "aqz-KE-bpKQ", Title: title, Duration: secs, Channel: channel, Artists: artists}
	}
	cases := []struct {
		name  string
		found []Entry
		want  sources.Want
		ok    bool
	}{
		{"exact", []Entry{song("Teardrop", 331, "Massive Attack - Topic", "Massive Attack")}, want, true},
		{"2 s off", []Entry{song("Teardrop", 329, "", "Massive Attack")}, want, true},
		{"3 s off", []Entry{song("Teardrop", 334, "", "Massive Attack")}, want, false},
		{"another artist", []Entry{song("Teardrop", 331, "Someone Else - Topic", "Someone Else")}, want, false},
		{"official video title", []Entry{song("Massive Attack - Teardrop (Official Video)", 331, "Some Channel")}, want, true},
		{"channel names the artist", []Entry{song("Teardrop", 331, "Massive Attack - Topic")}, want, true},
		{"another song", []Entry{song("Angel", 331, "", "Massive Attack")}, want, false},
		{"unknown length", []Entry{song("Teardrop", 0, "", "Massive Attack")}, want, false},
		{"no length wanted", []Entry{song("Teardrop", 331, "", "Massive Attack")}, sources.Want{Title: "Teardrop", Artists: want.Artists}, false},
		{"no artist wanted", []Entry{song("Teardrop", 331, "", "Massive Attack")}, sources.Want{Title: "Teardrop", Duration: want.Duration}, false},
	}
	for _, c := range cases {
		if _, ok := BestSong(c.found, c.want); ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
		}
	}

	// of two that pass, the closer one wins
	closer := song("Teardrop", 331, "", "Massive Attack")
	closer.ID = "u7qY1uQ2l0w"
	got, ok := BestSong([]Entry{song("Teardrop", 329, "", "Massive Attack"), closer}, want)
	if !ok || got.ID != "u7qY1uQ2l0w" {
		t.Fatalf("best = %+v, %v", got, ok)
	}
}

func TestMusicSourceFind(t *testing.T) {
	s := MusicSource{C: fake(t)}
	want := sources.Want{Title: "Teardrop", Artists: []string{"Massive Attack"}, Duration: 331 * time.Second}

	got, err := s.Find(context.Background(), want)
	if err != nil || len(got) != 1 || got[0].Source != MusicName || got[0].Ref != "u7qY1uQ2l0w" || got[0].Label != VideoURL("u7qY1uQ2l0w") {
		t.Fatalf("candidates = %+v, %v", got, err)
	}
	// 20 s off: nothing is confident
	want.Duration = 351 * time.Second
	if got, err := s.Find(context.Background(), want); err != nil || len(got) != 0 {
		t.Fatalf("off length: %+v, %v", got, err)
	}
	if got, _ := s.Find(context.Background(), sources.Want{Title: "Teardrop", Artists: want.Artists}); got != nil {
		t.Fatalf("unknown length: %+v", got)
	}
}
