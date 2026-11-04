package appleimport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"audioloader/internal/applemusic"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
)

// syncServices is fakeServices with a page that can change between checks,
// and MusicBrainz release lookups: "rel-Album of X" is a one-track release
// of "rec-X".
func syncServices(t *testing.T, lengths map[string]int) (am *applemusic.Client, mbc *mb.Client, page *atomic.Value) {
	t.Helper()
	page = &atomic.Value{}
	apple := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(page.Load().(string)))
	}))
	t.Cleanup(apple.Close)
	brainz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if title, ok := strings.CutPrefix(r.URL.Path, "/ws/2/release/rel-Album of "); ok {
			w.Write([]byte(`{"id":"rel-Album of ` + title + `","title":"Album of ` + title + `","date":"2020","release-group":{"primary-type":"Album"},
				"media":[{"position":1,"tracks":[{"position":1,"title":"` + title + `","recording":` +
				rec("rec-"+title, title, "Some Artist", lengths[title]) + `}]}]}`))
			return
		}
		q := r.URL.Query().Get("query")
		for title, ms := range lengths {
			if strings.Contains(q, `recording:"`+title+`"`) {
				w.Write([]byte(results(rec("rec-"+title, title, "Some Artist", ms, "Album of "+title))))
				return
			}
		}
		w.Write([]byte(results()))
	}))
	t.Cleanup(brainz.Close)
	am = applemusic.New()
	am.BaseURL = apple.URL
	am.Sleep = func(context.Context, time.Duration) error { return nil }
	mbc = mb.New()
	mbc.MusicBrainzURL = brainz.URL + "/ws/2"
	mbc.Sleep = func(context.Context, time.Duration) error { return nil }
	return am, mbc, page
}

func TestSyncAddsNewSongs(t *testing.T) {
	dbtest.New(t)
	am, mbc, page := syncServices(t, map[string]int{"Old": 100000, "New": 180000, "Unsure": 240000})
	// song IDs are 1000, 1001, ... in a page's order; Gone gets one of its own
	page.Store(strings.Replace(playlistPage("Mine", 2, song{"Old", "Some Artist", "Album of Old", 100000}, song{"Gone", "Some Artist", "X", 100000}), "1001", "9001", 1))
	id, _ := Start(playlistURL)
	if err := Run(context.Background(), am, mbc, id); err != nil {
		t.Fatal(err)
	}
	// the import already made its playlist, so new songs are appended to it
	imp, _ := db.GetAppleImport(id)
	tracks, _ := db.AppleImportTracks(id)
	if err := AddTrack(context.Background(), mbc, map[string]*mb.Release{}, nil, tracks[0]); err != nil {
		t.Fatal(err)
	}
	tracks, _ = db.AppleImportTracks(id)
	if err := SyncPlaylist(imp, tracks); err != nil {
		t.Fatal(err)
	}
	imp, _ = db.GetAppleImport(id)

	f, err := db.FollowApplePlaylist(id)
	if err != nil {
		t.Fatal(err)
	}
	// Gone is removed from the playlist, Old is seen, New and Unsure are new
	page.Store(playlistPage("Mine", 3,
		song{"Old", "Some Artist", "Album of Old", 100000},
		song{"New", "Some Artist", "Album of New", 180000},
		song{"Unsure", "Some Artist", "Album of Unsure", 180000}))
	if err := Sync(context.Background(), am, mbc, f.ID); err != nil {
		t.Fatal(err)
	}

	tracks, _ = db.AppleImportTracks(id)
	if len(tracks) != 4 {
		t.Fatalf("%d songs: %+v", len(tracks), tracks)
	}
	byTitle := map[string]*db.AppleImportTrack{}
	for _, tr := range tracks {
		byTitle[tr.Title] = tr
	}
	if n := byTitle["New"]; n.Position != 3 || n.State != db.AppleSongAdded || n.TrackID == "" {
		t.Errorf("New = %+v", n)
	}
	if u := byTitle["Unsure"]; u.Position != 4 || u.State != db.AppleSongUnsure || u.TrackID != "" {
		t.Errorf("Unsure = %+v", u)
	}
	if g := byTitle["Gone"]; g.State != db.AppleSongNotFound {
		t.Errorf("a removed song = %+v", g)
	}
	// a queued download, and the song in the import's playlist
	if counts, _ := jobs.Counts(downloads.JobDownload); counts["queued"] < 1 {
		t.Errorf("download jobs = %v", counts)
	}
	pl, err := db.GetPlaylist(db.Normal, imp.PlaylistID)
	if err != nil {
		t.Fatal(err)
	}
	var inList bool
	for _, tr := range pl.Tracks {
		inList = inList || tr.ID.String() == byTitle["New"].TrackID
	}
	if !inList {
		t.Errorf("playlist = %+v, want the new song in it", pl.Tracks)
	}
	f, _ = db.GetAppleFollow(f.ID)
	if f.LastCheckedAt == "" || f.LastError != "" || f.ToPick != 2 {
		t.Errorf("follow = %+v", f)
	}

	// a second check finds nothing new
	if err := Sync(context.Background(), am, mbc, f.ID); err != nil {
		t.Fatal(err)
	}
	if again, _ := db.AppleImportTracks(id); len(again) != 4 {
		t.Errorf("%d songs after a second check", len(again))
	}
}

func TestSyncRecordsWhatWentWrong(t *testing.T) {
	dbtest.New(t)
	am, mbc, page := syncServices(t, map[string]int{"One": 100000})
	page.Store(playlistPage("Mine", 1, song{"One", "Some Artist", "Album of One", 100000}))
	id, _ := Start(playlistURL)
	if err := Run(context.Background(), am, mbc, id); err != nil {
		t.Fatal(err)
	}
	f, _ := db.FollowApplePlaylist(id)

	page.Store("<html>nothing</html>")
	if err := Sync(context.Background(), am, mbc, f.ID); err != nil {
		t.Fatal(err)
	}
	if f, _ = db.GetAppleFollow(f.ID); f.LastError != msgUnreadable || f.LastCheckedAt == "" {
		t.Errorf("unreadable page: %+v", f)
	}

	// a page that lists fewer songs than it says is a note, and still syncs
	page.Store(playlistPage("Mine", 5, song{"One", "Some Artist", "Album of One", 100000}))
	if err := Sync(context.Background(), am, mbc, f.ID); err != nil {
		t.Fatal(err)
	}
	if f, _ = db.GetAppleFollow(f.ID); f.LastError != "Apple Music shows the first 1 of 5 songs" {
		t.Errorf("short page: %+v", f)
	}

	// a playlist that is not public any more keeps the follow
	notFound := httptest.NewServer(http.NotFoundHandler())
	defer notFound.Close()
	am.BaseURL = notFound.URL
	if err := Sync(context.Background(), am, mbc, f.ID); err != nil {
		t.Fatal(err)
	}
	if f, err := db.GetAppleFollow(f.ID); err != nil || f.LastError != msgNotPublic {
		t.Errorf("not public: %+v, %v", f, err)
	}

	// an unfollowed playlist is left alone
	db.DeleteAppleFollow(f.ID)
	if err := Sync(context.Background(), am, mbc, f.ID); err != nil {
		t.Errorf("sync of an unfollowed playlist: %v", err)
	}
}

func TestScheduleQueuesDueFollows(t *testing.T) {
	dbtest.New(t)
	am, mbc, page := syncServices(t, nil)
	page.Store(playlistPage("Mine", 0))
	id, _ := Start(playlistURL)
	db.SetAppleImportStatus(id, db.ImportDone, "")
	f, _ := db.FollowApplePlaylist(id)
	_, _ = am, mbc

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { Schedule(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if counts, _ := jobs.Counts(JobSync); counts["queued"] == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no sync job queued")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	// queuing again while one waits adds no second job
	if err := EnqueueSync(f.ID); err != nil {
		t.Fatal(err)
	}
	if counts, _ := jobs.Counts(JobSync); counts["queued"] != 1 {
		t.Errorf("jobs = %v", counts)
	}
}
