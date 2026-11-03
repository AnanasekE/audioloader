package appleimport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"audioloader/internal/applemusic"
	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
)

type song struct {
	title, artist, album string
	ms                   int
}

// playlistPage is a playlist page of the shape Apple serves.
func playlistPage(name string, count int, songs ...song) string {
	var items []any
	for i, s := range songs {
		items = append(items, map[string]any{
			"title": s.title, "artistName": s.artist, "duration": s.ms,
			"tertiaryLinks":     []any{map[string]any{"title": s.album}},
			"contentDescriptor": map[string]any{"identifiers": map[string]string{"storeAdamID": fmt.Sprint(1000 + i)}},
		})
	}
	data := map[string]any{"data": []any{map[string]any{"data": map[string]any{"sections": []any{
		map[string]any{"itemKind": "containerDetailHeaderLockup", "items": []any{map[string]any{"title": name, "trackCount": count}}},
		map[string]any{"itemKind": "trackLockup", "items": items},
	}}}}}
	b, _ := json.Marshal(data)
	return `<script type="application/json" id="serialized-server-data">` + string(b) + `</script>`
}

// fakeServices serves the playlist page, and MusicBrainz searches by title:
// every song but "Nowhere" is found, with the length given in the map.
func fakeServices(t *testing.T, page string, lengths map[string]int) (*applemusic.Client, *mb.Client, *atomic.Int32) {
	t.Helper()
	apple := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/playlist/") {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(page))
	}))
	t.Cleanup(apple.Close)
	var searches atomic.Int32
	brainz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searches.Add(1)
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
	am := applemusic.New()
	am.BaseURL = apple.URL
	am.Sleep = func(context.Context, time.Duration) error { return nil }
	mbc := mb.New()
	mbc.MusicBrainzURL = brainz.URL + "/ws/2"
	mbc.Sleep = func(context.Context, time.Duration) error { return nil }
	return am, mbc, &searches
}

const playlistURL = "https://music.apple.com/us/playlist/mine/pl.u-abc123"

func TestRunMatchesEverySong(t *testing.T) {
	dbtest.New(t)
	a, _ := db.AddArtist(db.All, "Some Artist")
	_, err := db.AddTrack(db.All, []string{a.ID.String()}, &audio.Track{Title: "Have It", Duration: 200 * time.Second},
		bytes.NewReader([]byte("have it")), ".mp3", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	page := playlistPage("Mine", 6,
		song{"Sure", "Some Artist", "Album of Sure", 180000},
		song{"Unsure", "Some Artist", "Album of Unsure", 180000},
		song{"Nowhere", "Some Artist", "Nothing", 150000},
		song{"Have It", "Some Artist", "Whatever", 200000},
		song{"Placeholder", "Some Artist", "Album of Placeholder", 90000},
	)
	am, mbc, searches := fakeServices(t, page, map[string]int{"Sure": 180500, "Unsure": 240000, "Placeholder": 90000})

	// a recording already queued is the library's too
	if _, _, _, err := db.AddSong(db.NewSong{RecordingMBID: "rec-Placeholder", Title: "Placeholder", LengthMs: 90000,
		Artists: []db.MatchArtist{{Name: "Some Artist"}}}); err != nil {
		t.Fatal(err)
	}

	id, err := Start(playlistURL)
	if err != nil {
		t.Fatal(err)
	}
	if counts, _ := jobs.Counts(JobImport); counts["queued"] != 1 {
		t.Fatalf("jobs = %v", counts)
	}
	if err := Run(context.Background(), am, mbc, id); err != nil {
		t.Fatal(err)
	}

	imp, err := db.GetAppleImport(id)
	if err != nil || imp.Status != db.ImportDone || imp.Name != "Mine" || imp.TrackCount != 6 {
		t.Fatalf("import = %+v, %v", imp, err)
	}
	tracks, _ := db.AppleImportTracks(id)
	want := []struct{ title, state, release string }{
		{"Sure", db.AppleSongMatched, "Album of Sure"},
		{"Unsure", db.AppleSongUnsure, "Album of Unsure"},
		{"Nowhere", db.AppleSongNotFound, ""},
		{"Have It", db.AppleSongOwned, ""},
		{"Placeholder", db.AppleSongOwned, "Album of Placeholder"},
	}
	if len(tracks) != len(want) {
		t.Fatalf("%d songs", len(tracks))
	}
	for i, w := range want {
		got := tracks[i]
		if got.Position != i+1 || got.Title != w.title || got.State != w.state || got.ReleaseTitle != w.release {
			t.Errorf("song %d = %+v, want %+v", i+1, got, w)
		}
	}
	if tracks[0].RecordingID != "rec-Sure" || tracks[0].ReleaseID != "rel-Album of Sure" || tracks[0].MatchArtist != "Some Artist" || tracks[0].Score < 0.9 {
		t.Errorf("matched song = %+v", tracks[0])
	}
	if tracks[1].RecordingID != "rec-Unsure" || tracks[2].RecordingID != "" {
		t.Errorf("candidates: unsure %q, not found %q", tracks[1].RecordingID, tracks[2].RecordingID)
	}
	// the song the library has was never searched for
	if n := searches.Load(); n != 4 {
		t.Errorf("%d searches, want 4 (Have It is owned by title)", n)
	}

	// a second run does nothing more
	before := searches.Load()
	if err := Run(context.Background(), am, mbc, id); err != nil || searches.Load() != before {
		t.Errorf("second run: %v, %d new searches", err, searches.Load()-before)
	}
}

func TestRunResumes(t *testing.T) {
	dbtest.New(t)
	page := playlistPage("Mine", 2, song{"One", "Some Artist", "A", 100000}, song{"Two", "Some Artist", "B", 100000})
	am, mbc, searches := fakeServices(t, page, map[string]int{"One": 100000, "Two": 100000})
	id, _ := Start(playlistURL)

	// MusicBrainz goes away after the first song
	mbc.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if searches.Load() >= 1 {
			return nil, errors.New("connection refused")
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	if err := Run(context.Background(), am, mbc, id); err == nil {
		t.Fatal("run succeeded without MusicBrainz")
	}
	counts, _ := db.AppleImportCounts(id)
	if counts[db.AppleSongMatched] != 1 || counts[db.AppleSongPending] != 1 {
		t.Fatalf("after the failure: %v", counts)
	}
	if imp, _ := db.GetAppleImport(id); imp.Status != db.ImportRunning {
		t.Fatalf("status = %q", imp.Status)
	}

	// the retry matches only what is left
	mbc.HTTP = &http.Client{}
	before := searches.Load()
	if err := Run(context.Background(), am, mbc, id); err != nil {
		t.Fatal(err)
	}
	counts, _ = db.AppleImportCounts(id)
	if counts[db.AppleSongMatched] != 2 || searches.Load()-before != 1 {
		t.Fatalf("after the retry: %v, %d searches", counts, searches.Load()-before)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRunFailsForAnUnreadablePage(t *testing.T) {
	dbtest.New(t)
	am, mbc, searches := fakeServices(t, "<html>nothing</html>", nil)
	id, _ := Start(playlistURL)
	err := Run(context.Background(), am, mbc, id)
	if !errors.Is(err, applemusic.ErrUnreadable) {
		t.Fatalf("err = %v, want a permanent ErrUnreadable", err)
	}
	imp, _ := db.GetAppleImport(id)
	if imp.Status != db.ImportFailed || imp.Error != applemusic.ErrUnreadable.Error() {
		t.Fatalf("import = %+v", imp)
	}
	if searches.Load() != 0 {
		t.Error("searched MusicBrainz for a playlist that wasn't read")
	}
	// a failed import isn't run again
	if err := Run(context.Background(), am, mbc, id); err != nil {
		t.Errorf("rerun = %v", err)
	}

	// a link that isn't a playlist fails the same way
	id2, _ := Start("https://music.apple.com/us/album/x/1")
	if err := Run(context.Background(), am, mbc, id2); err == nil {
		t.Fatal("run succeeded for a link that is not a playlist")
	}
}
