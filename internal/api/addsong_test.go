package api

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
)

const (
	teardropRecording = "f3bba4cd-8018-468b-902e-bc8f029593e5" // on the release fixture
	searchedRecording = "a31e069b-1025-4700-971d-c6cfe937e31a" // first search fixture hit
	mezzanine         = "0d33ef7a-1f5d-4365-b807-b412271b99c3"
)

func fakeSongMusicBrainz(t *testing.T) {
	t.Helper()
	search, err := os.ReadFile("../mb/testdata/recording_search.json")
	if err != nil {
		t.Fatal(err)
	}
	release, err := os.ReadFile("../mb/testdata/release.json")
	if err != nil {
		t.Fatal(err)
	}
	fakeMusicBrainz(t, map[string]string{"/recording": string(search), "/release/": string(release)})
}

func TestAddSongFromMusicBrainz(t *testing.T) {
	dbtest.New(t)
	fakeSongMusicBrainz(t)
	h := downloadsHandler(t)

	rec := do(h, "GET", "/ui/add/musicbrainz?q="+url.QueryEscape("Massive Attack Teardrop"), nil)
	body := rec.Body.String()
	if rec.Code != 200 || strings.Count(body, ">Add<") != 2 || !strings.Contains(body, "Spirits of Nature 5") || !strings.Contains(body, "· 1998") ||
		!strings.Contains(body, "/release/f9199933-4e7a-4a71-baa9-687258054114/front-250") || !strings.Contains(body, "None of these") {
		t.Fatalf("search = %d\n%s", rec.Code, body)
	}
	if rec := do(h, "GET", "/ui/add/musicbrainz?q=", nil); rec.Code != 400 {
		t.Fatalf("empty search: %d", rec.Code)
	}

	// a song the library has is linked, not offered (the second hit has no
	// length, so the title and artist are enough)
	a, _ := db.AddArtist(db.All, "Massive Attack")
	_, err := db.AddTrack(db.All, []string{a.ID.String()}, &audio.Track{Title: "Teardrop", MBID: searchedRecording, Duration: time.Minute},
		bytes.NewReader([]byte("teardrop")), ".mp3", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	body = do(h, "GET", "/ui/add/musicbrainz?q=Teardrop", nil).Body.String()
	if strings.Contains(body, ">Add<") || strings.Count(body, "In your library") != 2 || !strings.Contains(body, `href="/releases/`) {
		t.Fatalf("owned search:\n%s", body)
	}

	// the recording must be on the posted release
	if rec := do(h, "POST", "/ui/add/musicbrainz", url.Values{"recording_id": {searchedRecording}, "release_id": {mezzanine}}); rec.Code != 400 {
		t.Fatalf("wrong release: %d", rec.Code)
	}
	if rec := do(h, "POST", "/ui/add/musicbrainz", url.Values{"recording_id": {"nope"}}); rec.Code != 400 {
		t.Fatalf("bad id: %d", rec.Code)
	}

	form := url.Values{"recording_id": {teardropRecording}, "release_id": {mezzanine}}
	if rec := do(h, "POST", "/ui/add/musicbrainz", form); rec.Code != 204 || !strings.Contains(rec.Header().Get("HX-Location"), "/downloads") {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	releases, _ := db.ListReleases(db.Normal)
	var mez *audio.Release
	for _, r := range releases {
		if r.MBID == mezzanine {
			mez, _ = db.GetRelease(db.All, r.ID.String())
		}
	}
	if mez == nil || mez.Title != "Mezzanine" || len(mez.Tracks) != 1 || mez.Tracks[0].Position != 3 || mez.Tracks[0].Track.HasFile {
		t.Fatalf("release = %+v, tracks %+v", mez, mez.Tracks[0])
	}
	if counts, _ := jobs.Counts(downloads.JobDownload); counts["queued"] != 1 {
		t.Fatalf("download jobs = %v", counts)
	}
	if queued, _ := db.ListDownloads(db.Normal, db.DownloadQueued); len(queued) != 1 {
		t.Fatalf("downloads = %v", queued)
	}

	// the same recording again is refused
	if rec := do(h, "POST", "/ui/add/musicbrainz", form); rec.Code != 409 {
		t.Fatalf("again: %d", rec.Code)
	}
}

func TestAddSongFromYouTube(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)

	rec := do(h, "GET", "/ui/add/youtube?q=Massive+Attack+Teardrop", nil)
	body := rec.Body.String()
	if rec.Code != 200 || strings.Count(body, "Use this") != 5 || !strings.Contains(body, `value="Teardrop"`) ||
		strings.Count(body, `value="Massive Attack"`) < 2 {
		t.Fatalf("search = %d\n%s", rec.Code, body)
	}

	form := url.Values{"video_id": {"aqz-KE-bpKQ"}, "title": {"Teardrop"}, "artist": {"Massive Attack"}}
	bad := url.Values{"video_id": {"--exec=rm"}, "title": {"x"}, "artist": {"y"}}
	if rec := do(h, "POST", "/ui/add/youtube", bad); rec.Code != 400 {
		t.Fatalf("bad id: %d", rec.Code)
	}
	if rec := do(h, "POST", "/ui/add/youtube", url.Values{"video_id": {"aqz-KE-bpKQ"}, "title": {"Teardrop"}}); rec.Code != 400 {
		t.Fatalf("no artist: %d", rec.Code)
	}
	if rec := do(h, "POST", "/ui/add/youtube", form); rec.Code != 204 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	recent, err := jobs.Recent(downloads.JobYouTubeImport, 5)
	if err != nil || len(recent) != 1 {
		t.Fatalf("jobs = %v, %v", recent, err)
	}
	var imp downloads.YouTubeImport
	json.Unmarshal(recent[0].Payload, &imp)
	artists, _ := db.ListArtists(db.Normal)
	if len(artists) != 1 || imp.Title != "Teardrop" || imp.VideoID != "aqz-KE-bpKQ" || len(imp.ArtistIDs) != 1 || imp.ArtistIDs[0] != artists[0].ID.String() {
		t.Fatalf("import = %+v, artists = %v", imp, artists)
	}
}
