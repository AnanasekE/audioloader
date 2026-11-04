package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"audioloader/internal/appleimport"
	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
)

const (
	angelRecording     = "8e74dd9d-e5a3-4acd-918a-c36a0f8cda84" // on the release fixture
	applePlaylist      = "https://music.apple.com/us/playlist/mix/pl.u-abc123"
	applePlaylistGone  = "https://music.apple.com/us/playlist/gone/pl.u-gone"
	applePlaylistBroke = "https://music.apple.com/us/playlist/broke/pl.u-broke"
)

// appleSongSpec is a song of a fake playlist page: title, artists, album, length.
type appleSongSpec struct {
	title, artists, album string
	ms                    int
}

// applePlaylistPage is a playlist page of the shape Apple serves.
func applePlaylistPage(name string, count int, songs ...appleSongSpec) string {
	var items []any
	for i, s := range songs {
		items = append(items, map[string]any{
			"title": s.title, "artistName": s.artists, "duration": s.ms,
			"tertiaryLinks":     []any{map[string]any{"title": s.album}},
			"contentDescriptor": map[string]any{"identifiers": map[string]string{"storeAdamID": fmt.Sprint(2000 + i)}},
		})
	}
	data := map[string]any{"data": []any{map[string]any{"data": map[string]any{"sections": []any{
		map[string]any{"itemKind": "containerDetailHeaderLockup", "items": []any{map[string]any{"title": name, "trackCount": count}}},
		map[string]any{"itemKind": "trackLockup", "items": items},
	}}}}}
	b, _ := json.Marshal(data)
	return `<html><body><script type="application/json" id="serialized-server-data">` + string(b) + `</script></body></html>`
}

// fakeMusicBrainzSearch points the shared client at a server that answers a
// recording search with search(query), a lookup of the Angel recording, and
// release lookups with the Mezzanine fixture. The queries are kept.
func fakeMusicBrainzSearch(t *testing.T, search func(query string) string) *[]string {
	t.Helper()
	var queries []string
	release := mustRead(t, "../mb/testdata/release.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/ws/2/release/"):
			w.Write([]byte(release))
		case r.URL.Path == "/ws/2/recording/"+angelRecording:
			w.Write([]byte(mezzanineRecording(angelRecording, "Angel", 380533)))
		case r.URL.Path == "/ws/2/recording":
			queries = append(queries, r.URL.Query().Get("query"))
			w.Write([]byte(search(r.URL.Query().Get("query"))))
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
	return &queries
}

// mezzanineRecording is a search result for a recording on Mezzanine.
func mezzanineRecording(id, title string, ms int) string {
	return `{"id":"` + id + `","title":"` + title + `","length":` + fmt.Sprint(ms) + `,
		"artist-credit":[{"name":"Massive Attack","artist":{"id":"10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8","name":"Massive Attack"}}],
		"releases":[{"id":"` + mezzanine + `","title":"Mezzanine","status":"Official","date":"1998-04-20","release-group":{"primary-type":"Album"}}]}`
}

// runAppleImportJob works the queue's import job once.
func runAppleImportJob(t *testing.T) {
	t.Helper()
	appleimport.Register()
	ran, err := jobs.RunOnce(context.Background(), jobs.Lane{Only: []string{appleimport.JobImport}})
	if err != nil || !ran {
		t.Fatalf("import job: ran %v, %v", ran, err)
	}
}

func TestAppleImport(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	fakeAppleMusic(t, map[string]string{
		"/us/playlist/mix/pl.u-abc123": applePlaylistPage("My Mix", 6,
			appleSongSpec{"Teardrop", "Massive Attack", "Mezzanine", 330000},
			appleSongSpec{"Angel", "Massive Attack", "Mezzanine", 300000}, // MusicBrainz has 6:20: unsure
			appleSongSpec{"Nowhere", "Massive Attack", "Nope", 100000},
			appleSongSpec{"Owned Song", "Massive Attack", "Mezzanine", 100000},
		),
		"/us/playlist/broke/pl.u-broke": "<html>nothing</html>",
	})
	fakeMusicBrainzSearch(t, func(q string) string {
		switch {
		case strings.Contains(q, `recording:"Teardrop"`):
			return `{"recordings":[` + mezzanineRecording(teardropRecording, "Teardrop", 330026) + `]}`
		case strings.Contains(q, `recording:"Angel"`):
			return `{"recordings":[` + mezzanineRecording(angelRecording, "Angel", 380533) + `]}`
		}
		return `{"recordings":[]}`
	})
	ma, _ := db.AddArtist(db.All, "Massive Attack")
	ownedTrack, err := db.AddTrack(db.All, []string{ma.ID.String()}, &audio.Track{Title: "Owned Song", Duration: 100 * time.Second},
		bytes.NewReader([]byte("owned")), ".mp3", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// a playlist link in the Add box starts an import; the page polls while it runs
	rec := do(h, "POST", "/ui/add/lookup", url.Values{"q": {applePlaylist}})
	loc := rec.Header().Get("HX-Location")
	if rec.Code != 204 || !strings.Contains(loc, `/add/apple/1`) {
		t.Fatalf("lookup = %d %s %q", rec.Code, rec.Body, loc)
	}
	if rec := do(h, "POST", "/ui/add/lookup", url.Values{"q": {applePlaylist}}); !strings.Contains(rec.Header().Get("HX-Location"), `/add/apple/1`) {
		t.Fatalf("a second paste of the same link made another import: %q", rec.Header().Get("HX-Location"))
	}
	if counts, _ := jobs.Counts(appleimport.JobImport); counts["queued"] != 1 {
		t.Fatalf("import jobs = %v", counts)
	}
	page := do(h, "GET", "/add/apple/1", nil)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Reading the playlist") || !strings.Contains(page.Body.String(), `hx-trigger="every 2s"`) {
		t.Fatalf("running page = %d\n%s", page.Code, page.Body)
	}
	if do(h, "GET", "/add/apple/99", nil).Code != 404 || do(h, "GET", "/ui/add/apple/x", nil).Code != 404 {
		t.Error("an import that doesn't exist is not 404")
	}

	runAppleImportJob(t)

	// done: no more polling, and the groups
	body := do(h, "GET", "/ui/add/apple/1", nil).Body.String()
	if strings.Contains(body, "every 2s") || !strings.Contains(body, "Matched (1)") || !strings.Contains(body, "Unsure (1)") ||
		!strings.Contains(body, "Not found (1)") || !strings.Contains(body, "In your library (1)") ||
		!strings.Contains(body, `value="1" checked`) || strings.Contains(body, `value="2" checked`) || !strings.Contains(body, ">Change<") ||
		!strings.Contains(body, "/add?q=Massive+Attack+-+Nowhere") || !strings.Contains(body, "Also create playlist “My Mix”") {
		t.Fatalf("done page:\n%s", body)
	}
	// the Search link sits inside the form that targets #apple-import: it must not inherit that
	if !strings.Contains(body, `<a hx-target="body" hx-swap="innerHTML" class="btn btn-ghost" href="/add?q=Massive+Attack+-+Nowhere"`) {
		t.Errorf("the Search link of a not-found song inherits the form's target:\n%s", body)
	}
	if strings.Contains(body, "Apple Music shows the first") == false {
		t.Errorf("no note about the 6 songs of which 4 are listed:\n%s", body)
	}
	if recent := do(h, "GET", "/add", nil).Body.String(); !strings.Contains(recent, `href="/add/apple/1"`) || !strings.Contains(recent, "My Mix") {
		t.Errorf("the Add page doesn't list the import:\n%s", recent)
	}

	// Change lists the candidates, and picking one makes it the match
	cands := do(h, "GET", "/ui/add/apple/1/2/candidates", nil).Body.String()
	if !strings.Contains(cands, "Angel") || !strings.Contains(cands, "Use this") || !strings.Contains(cands, "Mezzanine") {
		t.Fatalf("candidates:\n%s", cands)
	}
	if rec := do(h, "POST", "/ui/add/apple/1/2/pick", url.Values{"recording_id": {"nope"}}); rec.Code != 400 {
		t.Errorf("bad recording = %d", rec.Code)
	}

	// Add selected: the matched and the unsure song, each on Mezzanine
	add := func(form url.Values) (int, string) {
		rec := do(h, "POST", "/ui/add/apple/1/add", form)
		return rec.Code, rec.Body.String()
	}
	code, body := add(url.Values{"pos": {"1", "2", "3", "4", "99"}, "playlist": {"1"}}) // not found and owned ones are not addable
	if code != 200 || !strings.Contains(body, "Added 2 songs.") || !strings.Contains(body, "Added (2)") || !strings.Contains(body, "In your library (1)") {
		t.Fatalf("add = %d\n%s", code, body)
	}
	if counts, _ := jobs.Counts(downloads.JobDownload); counts["queued"] != 2 {
		t.Fatalf("download jobs = %v", counts)
	}
	releases, _ := db.ListReleases(db.Normal)
	var mez *audio.Release
	for _, r := range releases {
		if r.MBID == mezzanine {
			mez, _ = db.GetRelease(db.All, r.ID.String())
		}
	}
	if mez == nil || len(mez.Tracks) != 2 || mez.Tracks[0].Position != 1 || mez.Tracks[1].Position != 3 {
		t.Fatalf("release = %+v", mez)
	}

	// the playlist has Apple's order: Teardrop, Angel, and the song the library had
	playlists, _ := db.ListPlaylists(db.Normal)
	if len(playlists) != 1 || playlists[0].Name != "My Mix" {
		t.Fatalf("playlists = %+v", playlists)
	}
	pl, _ := db.GetPlaylist(db.Normal, playlists[0].ID.String())
	var titles []string
	for _, tr := range pl.Tracks {
		titles = append(titles, tr.Title)
	}
	if strings.Join(titles, ",") != "Teardrop,Angel,Owned Song" || pl.Tracks[2].ID.String() != ownedTrack.TrackID {
		t.Fatalf("playlist = %v", titles)
	}

	// again: nothing new is added, and the playlist stays one with no duplicates
	code, body = add(url.Values{"pos": {"1", "2"}, "playlist": {"1"}})
	if code != 200 || !strings.Contains(body, "Added 0 songs.") || !strings.Contains(body, "Also update playlist") {
		t.Fatalf("second add = %d\n%s", code, body)
	}
	if counts, _ := jobs.Counts(downloads.JobDownload); counts["queued"] != 2 {
		t.Errorf("download jobs after the second add = %v", counts)
	}
	if playlists, _ := db.ListPlaylists(db.Normal); len(playlists) != 1 || playlists[0].Count != 3 {
		t.Errorf("playlists after the second add = %+v", playlists)
	}
}

func TestAppleImportChangeAndSkip(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	fakeAppleMusic(t, map[string]string{
		"/us/playlist/mix/pl.u-abc123": applePlaylistPage("My Mix", 2,
			appleSongSpec{"Angel", "Massive Attack", "Mezzanine", 300000}),
	})
	fakeMusicBrainzSearch(t, func(q string) string {
		return `{"recordings":[` + mezzanineRecording(angelRecording, "Angel", 380533) + `]}`
	})
	do(h, "POST", "/ui/add/lookup", url.Values{"q": {applePlaylist}})
	runAppleImportJob(t)

	// an unsure song is not ticked, so Add selected without it adds nothing and no playlist is made
	rec := do(h, "POST", "/ui/add/apple/1/add", url.Values{})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Added 0 songs.") {
		t.Fatalf("add nothing = %d %s", rec.Code, rec.Body)
	}
	if playlists, _ := db.ListPlaylists(db.Normal); len(playlists) != 0 {
		t.Fatalf("a playlist was made without being asked: %+v", playlists)
	}

	// picking a candidate makes it matched, so the page ticks it
	rec = do(h, "POST", "/ui/add/apple/1/1/pick", url.Values{"recording_id": {angelRecording}, "release_id": {mezzanine}})
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Matched (1)") || strings.Contains(body, "Unsure") || !strings.Contains(body, `value="1" checked`) {
		t.Fatalf("pick = %d\n%s", rec.Code, body)
	}
	// a release the recording isn't on is refused
	if rec := do(h, "POST", "/ui/add/apple/1/1/pick", url.Values{"recording_id": {angelRecording}, "release_id": {"f9199933-4e7a-4a71-baa9-687258054114"}}); rec.Code != 400 {
		t.Errorf("wrong release = %d", rec.Code)
	}
}

func TestAppleImportFailures(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	fakeAppleMusic(t, map[string]string{"/us/playlist/broke/pl.u-broke": "<html>nothing</html>"})
	fakeMusicBrainzSearch(t, func(string) string { return `{"recordings":[]}` })

	do(h, "POST", "/ui/add/lookup", url.Values{"q": {applePlaylistBroke}})
	runAppleImportJob(t)
	body := do(h, "GET", "/ui/add/apple/1", nil).Body.String()
	if strings.Contains(body, "every 2s") || !strings.Contains(body, "Apple Music changed its page; this link can&#39;t be read.") {
		t.Fatalf("unreadable playlist:\n%s", body)
	}

	// a playlist that isn't there says so
	do(h, "POST", "/ui/add/lookup", url.Values{"q": {applePlaylistGone}})
	runAppleImportJob(t)
	if body := do(h, "GET", "/ui/add/apple/2", nil).Body.String(); !strings.Contains(body, "isn&#39;t on Apple Music") {
		t.Fatalf("missing playlist:\n%s", body)
	}

	// adding to an import that isn't done is refused
	id, err := appleimport.Start("https://music.apple.com/us/playlist/mix/pl.u-abc123")
	if err != nil {
		t.Fatal(err)
	}
	if rec := do(h, "POST", fmt.Sprintf("/ui/add/apple/%d/add", id), url.Values{"pos": {"1"}}); rec.Code != http.StatusConflict {
		t.Errorf("add before the import is done = %d", rec.Code)
	}
}

func TestAppleFollow(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	fakeAppleMusic(t, map[string]string{
		"/us/playlist/mix/pl.u-abc123": applePlaylistPage("My Mix", 1, appleSongSpec{"Nowhere", "Massive Attack", "Nope", 100000}),
	})
	fakeMusicBrainzSearch(t, func(string) string { return `{"recordings":[]}` })

	// a running import can't be followed yet
	do(h, "POST", "/ui/add/lookup", url.Values{"q": {applePlaylist}})
	if rec := do(h, "POST", "/ui/add/apple/1/follow", url.Values{}); rec.Code != 409 {
		t.Fatalf("follow while running = %d", rec.Code)
	}
	runAppleImportJob(t)

	rec := do(h, "GET", "/add/apple/1", nil)
	if !strings.Contains(rec.Body.String(), "Keep in sync") || strings.Contains(rec.Body.String(), "Stop syncing") {
		t.Fatalf("page before following:\n%s", rec.Body)
	}
	rec = do(h, "POST", "/ui/add/apple/1/follow", url.Values{})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Stop syncing") || strings.Contains(rec.Body.String(), "Keep in sync</button>") {
		t.Fatalf("follow = %d\n%s", rec.Code, rec.Body)
	}
	do(h, "POST", "/ui/add/apple/1/follow", url.Values{}) // a second time reuses the follow
	if follows, _ := db.ListAppleFollows(); len(follows) != 1 || follows[0].ImportID != 1 {
		t.Fatalf("follows = %+v", follows)
	}
	if rec := do(h, "POST", "/ui/add/apple/9/follow", url.Values{}); rec.Code != 404 {
		t.Errorf("follow of a missing import = %d", rec.Code)
	}

	// the Add page lists it, with the song to pick
	rec = do(h, "GET", "/add", nil)
	body := rec.Body.String()
	if !strings.Contains(body, "My Mix") || !strings.Contains(body, "Not checked yet") || !strings.Contains(body, "1 to pick") || !strings.Contains(body, "Check now") {
		t.Fatalf("Add page:\n%s", body)
	}

	// Check now queues the job once, and the list says so and keeps polling
	for range 2 {
		rec = do(h, "POST", "/ui/add/apple-follows/1/check", url.Values{})
	}
	if counts, _ := jobs.Counts(appleimport.JobSync); rec.Code != 200 || counts["queued"] != 1 {
		t.Fatalf("check = %d, jobs = %v", rec.Code, counts)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Checking…") || !strings.Contains(body, `hx-trigger="every 3s"`) {
		t.Fatalf("list while checking:\n%s", body)
	}
	if rec := do(h, "POST", "/ui/add/apple-follows/7/check", url.Values{}); rec.Code != 404 {
		t.Errorf("check of a missing follow = %d", rec.Code)
	}

	// Stop syncing deletes the follow and keeps the import
	rec = do(h, "POST", "/ui/add/apple-follows/1/delete", url.Values{})
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "My Mix") {
		t.Fatalf("delete = %d\n%s", rec.Code, rec.Body)
	}
	if follows, _ := db.ListAppleFollows(); len(follows) != 0 {
		t.Fatalf("follows after delete = %+v", follows)
	}
	if _, err := db.GetAppleImport(1); err != nil {
		t.Errorf("import after delete: %v", err)
	}
	if rec := do(h, "POST", "/ui/add/apple-follows/1/delete", url.Values{}); rec.Code != 404 {
		t.Errorf("second delete = %d", rec.Code)
	}
}

func TestAppleFollowInbox(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	fakeAppleMusic(t, map[string]string{
		"/us/playlist/mix/pl.u-abc123": applePlaylistPage("My Mix", 2,
			appleSongSpec{"Nowhere", "Massive Attack", "Nope", 100000},
			appleSongSpec{"Elsewhere", "Massive Attack", "Nope", 100000}),
	})
	fakeMusicBrainzSearch(t, func(string) string { return `{"recordings":[]}` })
	do(h, "POST", "/ui/add/lookup", url.Values{"q": {applePlaylist}})
	runAppleImportJob(t)

	// an import that isn't followed adds nothing to the Inbox
	if body := do(h, "GET", "/inbox", nil).Body.String(); strings.Contains(body, "Apple Music:") {
		t.Fatalf("inbox before following:\n%s", body)
	}
	do(h, "POST", "/ui/add/apple/1/follow", url.Values{})

	body := do(h, "GET", "/inbox", nil).Body.String()
	if !strings.Contains(body, "Apple Music: 2 songs to pick") || !strings.Contains(body, `href="/add/apple/1"`) {
		t.Fatalf("inbox:\n%s", body)
	}
	if badge := do(h, "GET", "/ui/badges/inbox", nil).Body.String(); !strings.Contains(badge, "1") {
		t.Errorf("badge = %q, want it to count the playlist", badge)
	}
	// the Ignored tab is about releases
	if body := do(h, "GET", "/inbox?tab=ignored", nil).Body.String(); strings.Contains(body, "Apple Music:") {
		t.Errorf("ignored tab shows the Apple Music section")
	}

	// nothing left to pick, nothing in the Inbox
	db.SetAppleTrackState(1, 1, db.AppleSongOwned)
	db.SetAppleTrackState(1, 2, db.AppleSongAdded)
	if body := do(h, "GET", "/inbox", nil).Body.String(); strings.Contains(body, "Apple Music:") {
		t.Errorf("inbox after picking:\n%s", body)
	}
	if badge := do(h, "GET", "/ui/badges/inbox", nil).Body.String(); strings.Contains(badge, "1") {
		t.Errorf("badge = %q, want none", badge)
	}
}
