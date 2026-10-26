package tracking

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
)

const massiveAttack = "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"

func testClient(t *testing.T) *mb.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file := ""
		switch r.URL.Path {
		case "/ws/2/release-group":
			// one page; later offsets are empty, as past the end
			if r.URL.Query().Get("offset") != "0" {
				w.Write([]byte(`{"release-groups": [], "release-group-count": 107}`))
				return
			}
			file = "release_groups.json"
		case "/ws/2/release":
			file = "releases_browse.json"
		default:
			http.NotFound(w, r)
			return
		}
		data, err := os.ReadFile("../mb/testdata/" + file)
		if err != nil {
			t.Error(err)
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	c := mb.New()
	c.MusicBrainzURL = srv.URL + "/ws/2"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestCheck(t *testing.T) {
	dbtest.New(t)
	c := testClient(t)
	a, _ := db.AddArtist("Massive Attack")
	if _, err := db.CreateRelease("Mezzanine", audio.ReleaseAlbum, "1998", []string{a.ID.String()}); err != nil {
		t.Fatal(err)
	}
	// owned as a single; the tracklists in the fixture all have Teardrop
	_, err := db.AddTrack(db.All, []string{a.ID.String()}, &audio.Track{Title: "Teardrop", Duration: 331 * time.Second},
		bytes.NewReader([]byte("x")), ".mp3", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetFollow(a.ID.String(), massiveAttack, db.DefaultFilters()); err != nil {
		t.Fatal(err)
	}

	if err := Check(context.Background(), c, a.ID.String()); err != nil {
		t.Fatal(err)
	}
	inbox, err := db.ListInbox("new")
	if err != nil {
		t.Fatal(err)
	}
	// five albums in the fixture, minus Mezzanine which is in the library
	if len(inbox) != 4 {
		t.Fatalf("inbox = %d releases", len(inbox))
	}
	first := inbox[0]
	if first.Title != "Heligoland" || first.Type != "album" || len(first.Data.Tracks) != 11 || first.Owned() != 1 {
		t.Fatalf("first = %+v, owned %d", first.Title, first.Owned())
	}

	if first.Data.SpotifyURL != "https://open.spotify.com/album/0sNOF9WDwhWunNAHPD3Baj" ||
		first.Data.Tracks[0].SpotifyURL != "https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb" || first.Data.Tracks[1].SpotifyURL != "" {
		t.Fatalf("spotify links = %q, %q", first.Data.SpotifyURL, first.Data.Tracks[0].SpotifyURL)
	}

	// ignored releases don't come back, and nothing is added twice
	if err := db.IgnoreInboxRelease(first.ReleaseGroupMBID); err != nil {
		t.Fatal(err)
	}
	if err := Check(context.Background(), c, a.ID.String()); err != nil {
		t.Fatal(err)
	}
	if inbox, _ := db.ListInbox("new"); len(inbox) != 3 {
		t.Fatalf("after second check: %d", len(inbox))
	}

	// confirming queues the downloads and the cover
	relID, err := Confirm(inbox[1].ReleaseGroupMBID)
	if err != nil {
		t.Fatal(err)
	}
	rel, _ := db.GetRelease(db.All, relID)
	if len(rel.Tracks) != 11 {
		t.Fatalf("tracks = %d", len(rel.Tracks))
	}
	if counts, _ := jobs.Counts("download"); counts["queued"] != 10 {
		t.Fatalf("download jobs = %v", counts)
	}
}

func TestWanted(t *testing.T) {
	f := db.DefaultFilters()
	cases := []struct {
		typ       string
		secondary []string
		want      bool
	}{
		{"album", nil, true},
		{"album", []string{"Live"}, false},
		{"compilation", []string{"Compilation"}, false}, // not in Types
		{"", nil, false},
	}
	for _, c := range cases {
		if got := Wanted(f, c.typ, c.secondary); got != c.want {
			t.Errorf("Wanted(%q, %v) = %v", c.typ, c.secondary, got)
		}
	}
	f.Types = append(f.Types, "compilation")
	if !Wanted(f, "compilation", []string{"Compilation"}) {
		t.Error("asked-for compilations are skipped")
	}
}
