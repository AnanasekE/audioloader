package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/mb"
)

const massiveAttack = "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"

// fakeBrowseMusicBrainz serves the browse fixtures and records the paths
// and offsets asked for.
func fakeBrowseMusicBrainz(t *testing.T) *[]string {
	t.Helper()
	var asked []string
	groups := mustRead(t, "../mb/testdata/release_groups.json")
	browse := mustRead(t, "../mb/testdata/releases_browse.json")
	release := mustRead(t, "../mb/testdata/release.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path+"?offset="+r.URL.Query().Get("offset"))
		switch {
		case r.URL.Path == "/ws/2/release-group":
			w.Write([]byte(groups))
		case r.URL.Path == "/ws/2/release":
			w.Write([]byte(browse))
		case strings.HasPrefix(r.URL.Path, "/ws/2/release/"):
			w.Write([]byte(release))
		default:
			http.NotFound(w, r)
		}
	}))
	c := mb.Default()
	oldURL, oldSleep := c.MusicBrainzURL, c.Sleep
	c.MusicBrainzURL = srv.URL + "/ws/2"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() {
		c.MusicBrainzURL, c.Sleep = oldURL, oldSleep
		srv.Close()
	})
	return &asked
}

func TestFindBrowseArtist(t *testing.T) {
	dbtest.New(t)
	asked := fakeBrowseMusicBrainz(t)
	h := downloadsHandler(t)

	rec := do(h, "GET", "/add/find/artist/"+massiveAttack+"?name=Massive+Attack", nil)
	body := rec.Body.String()
	// 5 albums of 107 groups: newest first, the rest behind More
	if rec.Code != 200 || !strings.Contains(body, "Massive Attack") || !strings.Contains(body, ">Albums<") ||
		strings.Index(body, "Heligoland") > strings.Index(body, "Blue Lines") ||
		!strings.Contains(body, "/add/find/release-group/6f9f6899-c0d3-311d-ae87-a10ae6bc53a9") ||
		!strings.Contains(body, "Follow artist") ||
		!strings.Contains(body, "/ui/add/find/artist/"+massiveAttack+"/groups?offset=5") {
		t.Fatalf("artist page = %d\n%s", rec.Code, body)
	}

	rec = do(h, "GET", "/ui/add/find/artist/"+massiveAttack+"/groups?offset=5", nil)
	body = rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Heligoland") || !strings.Contains(body, "offset=10") {
		t.Fatalf("more = %d\n%s", rec.Code, body)
	}
	if got := (*asked)[len(*asked)-1]; got != "/ws/2/release-group?offset=5" {
		t.Fatalf("asked %q", got)
	}
	if rec := do(h, "GET", "/ui/add/find/artist/"+massiveAttack+"/groups?offset=x", nil); rec.Code != 400 {
		t.Fatalf("bad offset = %d", rec.Code)
	}
	if rec := do(h, "GET", "/add/find/artist/nope", nil); rec.Code != 404 {
		t.Fatalf("bad id = %d", rec.Code)
	}

	// a followed artist shows as such
	_, _, _, err := db.AddSong(songOnMezzanine())
	if err != nil {
		t.Fatal(err)
	}
	a, err := db.ArtistByMBID(db.All, massiveAttack)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetFollow(a.ID.String(), massiveAttack, db.DefaultFilters()); err != nil {
		t.Fatal(err)
	}
	body = do(h, "GET", "/add/find/artist/"+massiveAttack, nil).Body.String()
	if strings.Contains(body, "Follow artist") || !strings.Contains(body, "Following") {
		t.Fatalf("followed:\n%s", body)
	}
}

// songOnMezzanine is Teardrop, queued on Mezzanine's release group.
func songOnMezzanine() db.NewSong {
	credit := []db.MatchArtist{{ID: massiveAttack, Name: "Massive Attack"}}
	return db.NewSong{RecordingMBID: teardropRecording, Title: "Teardrop", Artists: credit, Release: &db.NewSongRelease{
		MBID: mezzanine, ReleaseGroupMBID: "6f9f6899-c0d3-311d-ae87-a10ae6bc53a9", Title: "Mezzanine", Type: "album",
		Artists: credit, Disc: 1, Position: 3,
	}}
}

func TestFindBrowseReleaseGroup(t *testing.T) {
	dbtest.New(t)
	fakeBrowseMusicBrainz(t)
	h := downloadsHandler(t)
	const group = "/add/find/release-group/6f9f6899-c0d3-311d-ae87-a10ae6bc53a9"

	rec := do(h, "GET", group, nil)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Mezzanine") || !strings.Contains(body, "Risingson") ||
		strings.Count(body, `name="recording_id"`) < 10 || !strings.Contains(body, `value="`+mezzanine+`"`) ||
		strings.Contains(body, "In your library") {
		t.Fatalf("tracklist = %d\n%s", rec.Code, body)
	}
	all := strings.Count(body, `name="recording_id"`)

	// a track the library has is marked and has no tick box
	if _, _, _, err := db.AddSong(songOnMezzanine()); err != nil {
		t.Fatal(err)
	}
	body = do(h, "GET", group, nil).Body.String()
	if got := strings.Count(body, `name="recording_id"`); got != all-1 || !strings.Contains(body, "Already queued") {
		t.Fatalf("owned: %d ticks of %d\n%s", got, all, body)
	}
	if _, err := db.DB.Exec(`UPDATE tracks SET has_file = 1 WHERE mbid = ?;`, teardropRecording); err != nil {
		t.Fatal(err)
	}
	body = do(h, "GET", group, nil).Body.String()
	if !strings.Contains(body, "In your library") {
		t.Fatalf("downloaded:\n%s", body)
	}
	if rec := do(h, "GET", "/add/find/release-group/nope", nil); rec.Code != 404 {
		t.Fatalf("bad id = %d", rec.Code)
	}
}

func TestFindAddSong(t *testing.T) {
	dbtest.New(t)
	fakeBrowseMusicBrainz(t)
	h := downloadsHandler(t)

	if rec := do(h, "POST", "/ui/add/find/song", url.Values{"recording_id": {"nope"}}); rec.Code != 400 {
		t.Fatalf("bad id = %d", rec.Code)
	}
	form := url.Values{"recording_id": {teardropRecording}, "release_id": {mezzanine}}
	rec := do(h, "POST", "/ui/add/find/song", form)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Added — queued") || !strings.Contains(rec.Body.String(), `href="/releases/`) {
		t.Fatalf("add = %d\n%s", rec.Code, rec.Body)
	}
	queued, err := db.ListDownloads(db.All, db.DownloadQueued)
	if err != nil || len(queued) != 1 {
		t.Fatalf("queued = %v, %v", queued, err)
	}
	// again: nothing new is written
	if rec := do(h, "POST", "/ui/add/find/song", form); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Added — queued") {
		t.Fatalf("again = %d", rec.Code)
	}
	if queued, _ := db.ListDownloads(db.All, db.DownloadQueued); len(queued) != 1 {
		t.Fatalf("queued twice: %v", queued)
	}
	if rec := do(h, "POST", "/ui/add/find/song", url.Values{"recording_id": {searchedRecording}, "release_id": {mezzanine}}); rec.Code != 400 {
		t.Fatalf("wrong release = %d", rec.Code)
	}
}

func TestFindAddRelease(t *testing.T) {
	dbtest.New(t)
	fakeBrowseMusicBrainz(t)
	h := downloadsHandler(t)
	const (
		angel     = "8e74dd9d-e5a3-4acd-918a-c36a0f8cda84"
		risingson = "02011500-814c-461c-b3a6-f24fe68ee5a1"
	)
	count := func() int {
		var n int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM tracks WHERE has_file = 0;`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	for name, form := range map[string]url.Values{
		"bad release":    {"release_id": {"nope"}, "all": {"1"}},
		"nothing ticked": {"release_id": {mezzanine}},
		"stray":          {"release_id": {mezzanine}, "recording_id": {angel, searchedRecording}},
	} {
		if rec := do(h, "POST", "/ui/add/find/release", form); rec.Code != 400 || count() != 0 {
			t.Fatalf("%s = %d, %d tracks", name, rec.Code, count())
		}
	}

	rec := do(h, "POST", "/ui/add/find/release", url.Values{"release_id": {mezzanine}, "recording_id": {angel, risingson}})
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Added 2 — queued") || count() != 2 || strings.Count(body, "Already queued") != 2 ||
		strings.Contains(body, `value="`+angel+`"`) {
		t.Fatalf("selected = %d, %d tracks\n%s", rec.Code, count(), body)
	}
	if queued, _ := db.ListDownloads(db.All, db.DownloadQueued); len(queued) != 2 {
		t.Fatalf("queued = %v", queued)
	}

	// owned tracks are skipped, so ticking only those writes nothing
	rec = do(h, "POST", "/ui/add/find/release", url.Values{"release_id": {mezzanine}, "recording_id": {angel}})
	if !strings.Contains(rec.Body.String(), "already in your library") || count() != 2 {
		t.Fatalf("owned = %d tracks\n%s", count(), rec.Body)
	}

	// the whole release adds the rest: 11 tracks in the fixture or more
	rec = do(h, "POST", "/ui/add/find/release", url.Values{"release_id": {mezzanine}, "all": {"1"}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "— queued") || count() <= 3 || strings.Contains(rec.Body.String(), "Add whole release") {
		t.Fatalf("all = %d, %d tracks\n%s", rec.Code, count(), rec.Body)
	}
}
