package appleimport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"audioloader/internal/applemusic"
	"audioloader/internal/mb"
)

// fakeMB answers every recording search with body and records the queries.
func fakeMB(t *testing.T, bodies ...string) (*mb.Client, *[]string) {
	t.Helper()
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		i := min(len(queries), len(bodies)) - 1
		w.Write([]byte(bodies[i]))
	}))
	t.Cleanup(srv.Close)
	c := mb.New()
	c.MusicBrainzURL = srv.URL + "/ws/2"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c, &queries
}

// rec is a search result: a recording by artist on the given releases.
func rec(id, title, artist string, lengthMs int, releases ...string) string {
	s := `{"id":"` + id + `","title":"` + title + `","length":` + strconv.Itoa(lengthMs) + `,
		"artist-credit":[{"name":"` + artist + `","artist":{"id":"a-` + artist + `","name":"` + artist + `"}}],"releases":[`
	for i, r := range releases {
		if i > 0 {
			s += ","
		}
		s += `{"id":"rel-` + r + `","title":"` + r + `","status":"Official","date":"2020","release-group":{"primary-type":"Album"}}`
	}
	return s + `]}`
}

func results(recs ...string) string {
	s := `{"recordings":[`
	for i, r := range recs {
		if i > 0 {
			s += ","
		}
		s += r
	}
	return s + `]}`
}

var teardrop = applemusic.Song{ID: "1", Title: "Teardrop", Artist: "Massive Attack", Album: "Mezzanine", Duration: 330 * time.Second}

func TestFindConfident(t *testing.T) {
	c, queries := fakeMB(t, results(
		rec("r1", "Teardrop", "Massive Attack", 330000, "Mezzanine"),
		rec("r2", "Teardrop", "Massive Attack", 200000, "Remixes"),
	))
	cands, err := Find(context.Background(), c, teardrop)
	if err != nil {
		t.Fatal(err)
	}
	best := Best(cands)
	if best == nil || best.Recording.ID != "r1" || best.Release == nil || best.Release.Title != "Mezzanine" {
		t.Fatalf("best = %+v", best)
	}
	if cands[1].Confident {
		t.Errorf("a version 130 s shorter is confident: %+v", cands[1])
	}
	if len(*queries) != 1 || (*queries)[0] != `artist:"Massive Attack" AND recording:"Teardrop"` {
		t.Errorf("queries = %q", *queries)
	}
}

func TestFindUnsure(t *testing.T) {
	cases := []struct {
		name string
		song applemusic.Song
		recs []string
	}{
		{"length too different", teardrop, []string{rec("r1", "Teardrop", "Massive Attack", 300000, "Mezzanine")}},
		{"length unknown", teardrop, []string{rec("r1", "Teardrop", "Massive Attack", 0, "Mezzanine")}},
		{"another title", teardrop, []string{rec("r1", "Angel", "Massive Attack", 330000, "Mezzanine")}},
		{"another artist", teardrop, []string{rec("r1", "Teardrop", "Someone Else", 330000, "Mezzanine")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client, _ := fakeMB(t, results(c.recs...))
			cands, err := Find(context.Background(), client, c.song)
			if err != nil {
				t.Fatal(err)
			}
			if len(cands) != 1 || Best(cands) != nil {
				t.Fatalf("candidates = %+v", cands)
			}
		})
	}
}

func TestFindNothing(t *testing.T) {
	c, _ := fakeMB(t, results())
	cands, err := Find(context.Background(), c, teardrop)
	if err != nil || len(cands) != 0 || Best(cands) != nil {
		t.Fatalf("Find = %+v, %v", cands, err)
	}
}

func TestFindPrefersTheAppleAlbum(t *testing.T) {
	// the same recording on a compilation (earliest) and on the album
	r := rec("r1", "Teardrop", "Massive Attack", 330000, "Best Of 90s", "Mezzanine")
	c, _ := fakeMB(t, results(r))
	cands, _ := Find(context.Background(), c, teardrop)
	if best := Best(cands); best == nil || best.Release.Title != "Mezzanine" {
		t.Fatalf("best = %+v", best)
	}
	// without the album on the page, tagging's choice stands
	noAlbum := teardrop
	noAlbum.Album = "Unknown Album"
	c, _ = fakeMB(t, results(r))
	cands, _ = Find(context.Background(), c, noAlbum)
	if best := Best(cands); best == nil || best.Release == nil {
		t.Fatalf("best = %+v", best)
	}
}

func TestFindAlbumBreaksATie(t *testing.T) {
	// two sure matches: the one on the Apple album comes first
	c, _ := fakeMB(t, results(
		rec("r1", "Teardrop", "Massive Attack", 330000, "Best Of 90s"),
		rec("r2", "Teardrop", "Massive Attack", 330500, "Mezzanine"),
	))
	cands, _ := Find(context.Background(), c, teardrop)
	if len(cands) != 2 || cands[0].Recording.ID != "r2" || !cands[0].Confident || !cands[1].Confident {
		t.Fatalf("candidates = %+v", cands)
	}
}

func TestFindSearchesTheFirstArtistToo(t *testing.T) {
	song := applemusic.Song{ID: "2", Title: "Solar Eclipse", Artist: "Drake & Don Toliver", Album: "HABIBTI", Duration: 218 * time.Second}
	c, queries := fakeMB(t,
		results(),
		results(rec("r1", "Solar Eclipse", "Drake", 218400, "HABIBTI")),
	)
	cands, err := Find(context.Background(), c, song)
	if err != nil {
		t.Fatal(err)
	}
	if Best(cands) == nil || len(*queries) != 2 || (*queries)[1] != `artist:"Drake" AND recording:"Solar Eclipse"` {
		t.Fatalf("best = %+v, queries = %q", Best(cands), *queries)
	}
}

func TestArtistNames(t *testing.T) {
	got := artistNames("Drake, Don Toliver & Yebba")
	if len(got) != 3 || got[0] != "Drake" || got[1] != "Don Toliver" || got[2] != "Yebba" {
		t.Fatalf("names = %q", got)
	}
	if got := artistNames("Simon"); len(got) != 1 || got[0] != "Simon" {
		t.Fatalf("names = %q", got)
	}
}

func TestAlbumTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Record Player - Single": "Record Player",
		"Muse - EP":              "Muse",
		"Muse - ep":              "Muse",
		"X - SINGLE":             "X",
		"Mezzanine":              "Mezzanine",
		"Single - Remixes":       "Single - Remixes",
		"":                       "",
	} {
		if got := albumTitle(in); got != want {
			t.Errorf("albumTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindSkipsLiveAndBootlegReleases(t *testing.T) {
	live := `{"id":"rel-live","title":"2021-12-06: Pamnation HQ","status":"Bootleg","date":"2021","release-group":{"primary-type":"Album","secondary-types":["Live"]}}`
	single := `{"id":"rel-single","title":"Record Player","status":"Official","date":"2022","release-group":{"primary-type":"Single"}}`
	for name, releases := range map[string]string{"live first": live + "," + single, "single first": single + "," + live} {
		body := `{"recordings":[{"id":"r1","title":"Record Player","length":180000,
			"artist-credit":[{"name":"Daisy the Great","artist":{"id":"a1","name":"Daisy the Great"}}],"releases":[` + releases + `]}]}`
		song := applemusic.Song{ID: "1", Title: "Record Player", Artist: "Daisy the Great", Album: "Record Player - Single", Duration: 180 * time.Second}
		c, _ := fakeMB(t, body)
		cands, _ := Find(context.Background(), c, song)
		if len(cands) != 1 || cands[0].Release == nil || cands[0].Release.ID != "rel-single" {
			t.Errorf("%s: release = %+v", name, cands)
		}
		// the suffix-stripped album also scores as the album
		if cands[0].Score < 0.99 {
			t.Errorf("%s: score = %v, want album bonus", name, cands[0].Score)
		}
	}
}
