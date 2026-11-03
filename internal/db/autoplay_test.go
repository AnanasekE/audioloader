package db

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	"audioloader/internal/audio"
)

func autoplayTitles(t *testing.T, v Viewer, seeds, exclude []string, n int) []string {
	t.Helper()
	tracks, err := AutoplaySuggestions(v, seeds, exclude, n)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, tr := range tracks {
		out = append(out, tr.Title)
	}
	return out
}

// autoplayLibrary: A is on album R1 (2020) with a1-a3; A and B share RX
// (2010, b1); C has c1 on an album of 2021; D has d1 on an EP of 2000. There
// is also a placeholder, an episode and a secret track, none of which may
// ever show up.
func autoplayLibrary(t *testing.T) map[string]string {
	t.Helper()
	newTestDB(t)
	artist := func(name string) string {
		a, err := AddArtist(All, name)
		if err != nil {
			t.Fatal(err)
		}
		return a.ID.String()
	}
	A, B, C, D := artist("A"), artist("B"), artist("C"), artist("D")
	ids := map[string]string{}
	release := func(title string, typ audio.ReleaseType, date string, artists []string, tracks map[string][]string) {
		r, err := CreateRelease(title, typ, date, artists)
		if err != nil {
			t.Fatal(err)
		}
		var in []ReleaseTrackInput
		for name, by := range tracks {
			ids[name] = addTestTrack(t, name, name, by...)
			in = append(in, ReleaseTrackInput{TrackID: ids[name], Disc: 1})
		}
		if err := SetReleaseTracks(All, r.ID.String(), in); err != nil {
			t.Fatal(err)
		}
	}
	release("R1", audio.ReleaseAlbum, "2020", []string{A}, map[string][]string{"a1": {A}, "a2": {A}, "a3": {A}})
	release("RX", audio.ReleaseAlbum, "2010", []string{A, B}, map[string][]string{"b1": {B}})
	release("RC", audio.ReleaseAlbum, "2021", []string{C}, map[string][]string{"c1": {C}})
	release("RD", audio.ReleaseEP, "2000", []string{D}, map[string][]string{"d1": {D}})

	r, _ := CreateRelease("RP", audio.ReleaseAlbum, "2020", []string{A})
	addTestPlaceholder(t, r, "placeholder", 1)
	show, err := ResolveShow(All, "Show", "Host")
	if err != nil {
		t.Fatal(err)
	}
	if ids["episode"], err = AddEpisode(All, show.ID.String(), &audio.Track{Title: "episode", Duration: time.Hour}, bytes.NewReader([]byte("ep")), ".mp3"); err != nil {
		t.Fatal(err)
	}
	res, err := AddTrack(Secret, []string{A}, &audio.Track{Title: "secret", Duration: time.Minute}, bytes.NewReader([]byte("sec")), ".mp3", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	ids["secret"] = res.TrackID
	return ids
}

func TestAutoplayScoring(t *testing.T) {
	ids := autoplayLibrary(t)
	// a3: same artist (3) + same type and year (1); b1: shares a release with A (2);
	// c1: an album within 5 years (1); d1: nothing
	got := autoplayTitles(t, Normal, []string{ids["a1"]}, []string{ids["a2"]}, 10)
	if want := []string{"a3", "b1", "c1", "d1"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := autoplayTitles(t, Normal, []string{ids["a1"]}, []string{ids["a2"]}, 2); !slices.Equal(got, []string{"a3", "b1"}) {
		t.Fatalf("n=2 got %v", got)
	}
}

func TestAutoplayNeverSuggestsHiddenTracks(t *testing.T) {
	ids := autoplayLibrary(t)
	for i := 0; i < 10; i++ {
		got := autoplayTitles(t, Normal, []string{ids["d1"]}, nil, 50)
		for _, title := range got {
			if title == "placeholder" || title == "episode" || title == "secret" || title == "d1" {
				t.Fatalf("suggested %q in %v", title, got)
			}
		}
		if len(got) != 5 { // a1, a2, a3, b1, c1: the rest of the owned music, filled up
			t.Fatalf("got %v, want the 5 other owned tracks", got)
		}
	}
	// the viewer's mode decides: the secret track is only for secret mode
	if got := autoplayTitles(t, Secret, []string{ids["secret"]}, nil, 10); len(got) != 0 {
		t.Fatalf("secret mode got %v, want nothing but the seed", got)
	}
}

func TestAutoplayEpisodeSeedGivesNothing(t *testing.T) {
	ids := autoplayLibrary(t)
	if got := autoplayTitles(t, Normal, []string{ids["episode"], ids["a1"]}, nil, 10); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	// an older episode among the seeds is only ignored
	if got := autoplayTitles(t, Normal, []string{ids["a1"], ids["episode"]}, nil, 1); !slices.Equal(got, []string{"a2"}) && !slices.Equal(got, []string{"a3"}) {
		t.Fatalf("got %v", got)
	}
}

func TestAutoplayFillIsRandom(t *testing.T) {
	newTestDB(t)
	first := ""
	varied := false
	for _, name := range []string{"t1", "t2", "t3", "t4", "t5", "t6"} {
		addTestTrack(t, name, name)
	}
	for i := 0; i < 20 && !varied; i++ {
		order := strings.Join(autoplayTitles(t, Normal, nil, nil, 6), ",")
		if first == "" {
			first = order
		}
		varied = order != first
	}
	if !varied {
		t.Fatalf("20 calls gave the same order %s", first)
	}
}

func TestAutoplaySecretTagsAndCreator(t *testing.T) {
	newTestDB(t)
	zed, _ := AddArtist(Secret, "Zed")
	amy, _ := AddArtist(Secret, "Amy")
	add := func(title, artist string, tags ...string) string {
		res, err := AddTrack(Secret, []string{artist}, &audio.Track{Title: title, Duration: time.Minute, Tags: tags}, bytes.NewReader([]byte(title)), ".mp3", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return res.TrackID
	}
	seed := add("seed", zed.ID.String(), "rain", "lofi")
	add("same creator", zed.ID.String())              // 3
	add("both tags", amy.ID.String(), "rain", "lofi") // 4
	add("one tag", amy.ID.String(), "RAIN")           // 2, tags ignore case
	add("nothing", amy.ID.String(), "metal")          // 0
	normal, _ := AddArtist(Normal, "N")
	addTestTrack(t, "normal", "normal", normal.ID.String()) // never in secret mode

	got := autoplayTitles(t, Secret, []string{seed}, nil, 10)
	if want := []string{"both tags", "same creator", "one tag", "nothing"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAutoplayLimits(t *testing.T) {
	ids := autoplayLibrary(t)
	if got := autoplayTitles(t, Normal, []string{ids["a1"]}, nil, 0); len(got) != 0 {
		t.Fatalf("n=0 got %v", got)
	}
	// an unknown seed is ignored, not an error
	if got := autoplayTitles(t, Normal, []string{"no-such-track"}, nil, 3); len(got) != 3 {
		t.Fatalf("got %v", got)
	}
}
