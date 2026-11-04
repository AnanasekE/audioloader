package api

import (
	"net/url"
	"strings"
	"testing"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
)

func fakeFindMusicBrainz(t *testing.T) {
	t.Helper()
	fakeMusicBrainz(t, map[string]string{
		"/recording":     mustRead(t, "../mb/testdata/recording_search.json"),
		"/release-group": mustRead(t, "../mb/testdata/release_group_search.json"),
		"/artist":        mustRead(t, "../mb/testdata/artist_search.json"),
	})
}

func TestFindSongs(t *testing.T) {
	dbtest.New(t)
	fakeFindMusicBrainz(t)
	h := downloadsHandler(t)

	form := url.Values{"mode": {"song"}, "q": {"Teardrop"}, "artist": {"Massive Attack"}, "year": {"1998"}}
	rec := do(h, "GET", "/ui/add/find/results?"+form.Encode(), nil)
	body := rec.Body.String()
	if rec.Code != 200 || strings.Count(body, ">Add<") != 2 || !strings.Contains(body, "Spirits of Nature 5") ||
		!strings.Contains(body, "5:22") || !strings.Contains(body, "· 1998") {
		t.Fatalf("results = %d\n%s", rec.Code, body)
	}
	if got := rec.Header().Get("HX-Push-Url"); !strings.HasPrefix(got, "/add/find?") || !strings.Contains(got, "q=Teardrop") || !strings.Contains(got, "year=1998") {
		t.Fatalf("HX-Push-Url = %q", got)
	}
	if rec := do(h, "GET", "/ui/add/find/results?mode=song&q=", nil); rec.Code != 400 {
		t.Fatalf("empty = %d", rec.Code)
	}

	// a recording the library has is marked, linked to its release
	_, relID, _, err := db.AddSong(db.NewSong{
		RecordingMBID: searchedRecording, Title: "Teardrop",
		Artists: []db.MatchArtist{{ID: "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", Name: "Massive Attack"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body = do(h, "GET", "/ui/add/find/results?mode=song&q=Teardrop", nil).Body.String()
	if strings.Count(body, ">Add<") != 1 || !strings.Contains(body, "Already queued") {
		t.Fatalf("queued song:\n%s", body)
	}
	if _, err := db.DB.Exec(`UPDATE tracks SET has_file = 1 WHERE mbid = ?;`, searchedRecording); err != nil {
		t.Fatal(err)
	}
	body = do(h, "GET", "/ui/add/find/results?mode=song&q=Teardrop", nil).Body.String()
	if strings.Count(body, ">Add<") != 1 || !strings.Contains(body, `href="/releases/`+relID+`">In your library`) {
		t.Fatalf("owned song:\n%s", body)
	}

	// the page runs a search in its address
	rec = do(h, "GET", "/add/find?mode=song&q=Teardrop", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Spirits of Nature 5") || !strings.Contains(rec.Body.String(), `value="Teardrop"`) {
		t.Fatalf("page = %d", rec.Code)
	}
}

func TestFindReleasesAndArtists(t *testing.T) {
	dbtest.New(t)
	fakeFindMusicBrainz(t)
	h := downloadsHandler(t)

	body := do(h, "GET", "/ui/add/find/results?mode=release&q=Heligoland", nil).Body.String()
	if !strings.Contains(body, "Heligoland (Live)") || !strings.Contains(body, "Album + Live · 2011") ||
		!strings.Contains(body, "/release-group/29a4c71c-2e7a-40e3-81d7-7960e8ec1bb1/front-250") || strings.Contains(body, " owned") {
		t.Fatalf("release groups:\n%s", body)
	}

	// owned count: two tracks queued on the release group, one downloaded
	song := func(title, rec string, pos int) db.NewSong {
		credit := []db.MatchArtist{{ID: "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", Name: "Massive Attack"}}
		return db.NewSong{RecordingMBID: rec, Title: title, Artists: credit, Release: &db.NewSongRelease{
			MBID: "rel1", ReleaseGroupMBID: "29a4c71c-2e7a-40e3-81d7-7960e8ec1bb1", Title: "Heligoland", Type: "album",
			Artists: credit, Disc: 1, Position: pos,
		}}
	}
	first, _, _, err := db.AddSong(song("Pandora", "rec-1", 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := db.AddSong(song("Splitting", "rec-2", 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`UPDATE tracks SET has_file = 1 WHERE uuid = ?;`, first); err != nil {
		t.Fatal(err)
	}
	body = do(h, "GET", "/ui/add/find/results?mode=release&q=Heligoland", nil).Body.String()
	if !strings.Contains(body, "1/2 owned") {
		t.Fatalf("owned count:\n%s", body)
	}

	body = do(h, "GET", "/ui/add/find/results?mode=artist&q=Massive+Attack", nil).Body.String()
	if !strings.Contains(body, "Massive Attack") || strings.Contains(body, "Following") {
		t.Fatalf("artists:\n%s", body)
	}
	// the songs above created the artist with its MusicBrainz ID
	a, err := db.ArtistByMBID(db.All, "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetFollow(a.ID.String(), a.MBID, db.DefaultFilters()); err != nil {
		t.Fatal(err)
	}
	body = do(h, "GET", "/ui/add/find/results?mode=artist&q=Massive+Attack", nil).Body.String()
	if !strings.Contains(body, "Following") {
		t.Fatalf("followed artist:\n%s", body)
	}
}
