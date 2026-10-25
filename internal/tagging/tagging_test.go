package tagging

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/jobs"
	"audioloader/internal/mb"
)

const teardrop = "f3bba4cd-8018-468b-902e-bc8f029593e5"

// png is the smallest file http.DetectContentType calls image/png.
var png = []byte("\x89PNG\r\n\x1a\n0000")

func testTagger(t *testing.T) *Tagger {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var file string
		switch {
		case strings.HasPrefix(r.URL.Path, "/ws/2/recording/"+teardrop):
			file = "recording.json"
		case r.URL.Path == "/ws/2/recording":
			file = "recording_search.json"
		case strings.HasPrefix(r.URL.Path, "/caa/release/"):
			w.Write(png)
			return
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
	c.MusicBrainzURL, c.CoverArtURL = srv.URL+"/ws/2", srv.URL+"/caa"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return &Tagger{MB: c}
}

// addTrack uploads a track by artist on a new release of the given title and type.
func addTrack(t *testing.T, title, artist string, dur time.Duration, mbid, releaseTitle string, typ audio.ReleaseType) (string, string) {
	t.Helper()
	a, err := db.AddArtist(artist)
	if err != nil {
		t.Fatal(err)
	}
	r, err := db.CreateRelease(releaseTitle, typ, "", []string{a.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.AddTrack(db.All, []string{a.ID.String()}, &audio.Track{Title: title, Duration: dur, MBID: mbid},
		bytes.NewReader([]byte(title+artist)), ".mp3", &db.ReleaseLink{ReleaseID: r.ID.String(), Disc: 1, Position: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	return res.TrackID, r.ID.String()
}

func TestTagFromEmbeddedID(t *testing.T) {
	dbtest.New(t)
	tg := testTagger(t)
	trackID, releaseID := addTrack(t, "Teardrop", "Massive Attack", 330*time.Second, teardrop, "Mezzanine", audio.ReleaseAlbum)

	if err := tg.Tag(context.Background(), trackID); err != nil {
		t.Fatal(err)
	}
	tr, _ := db.GetTrack(db.All, trackID)
	if tr.MBID != teardrop || tr.ISRC == "" {
		t.Fatalf("track = %+v", tr)
	}
	if tr.Artists[0].MBID != "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8" {
		t.Fatalf("artist mbid = %q", tr.Artists[0].MBID)
	}
	r, _ := db.GetRelease(db.All, releaseID)
	if r.MBID == "" || r.ReleaseGroupMBID == "" || r.Date == "" {
		t.Fatalf("release = %+v", r)
	}
	if counts, _ := jobs.Counts(JobCover); counts["queued"] != 1 {
		t.Fatalf("cover jobs = %v", counts)
	}

	// the cover job stores the Cover Art Archive image
	if err := tg.FetchCover(context.Background(), releaseID); err != nil {
		t.Fatal(err)
	}
	r, _ = db.GetRelease(db.All, releaseID)
	if !strings.HasPrefix(r.ImgURL, "/covers/") {
		t.Fatalf("cover = %q", r.ImgURL)
	}
}

func TestTagKeepsUserValues(t *testing.T) {
	dbtest.New(t)
	tg := testTagger(t)
	trackID, releaseID := addTrack(t, "Teardrop", "Massive Attack", 330*time.Second, teardrop, "Mezzanine", audio.ReleaseAlbum)
	a, _ := db.ListArtists(db.All)
	if err := db.UpdateRelease(releaseID, "Mezzanine", audio.ReleaseAlbum, "2001", []string{a[0].ID.String()}); err != nil {
		t.Fatal(err)
	}
	if err := tg.Tag(context.Background(), trackID); err != nil {
		t.Fatal(err)
	}
	r, _ := db.GetRelease(db.All, releaseID)
	if r.Date != "2001" || r.Title != "Mezzanine" {
		t.Fatalf("user values overwritten: %+v", r)
	}
}

func TestTagFromSearch(t *testing.T) {
	dbtest.New(t)
	tg := testTagger(t)
	trackID, _ := addTrack(t, "Teardrop", "Massive Attack", 322*time.Second, "", "Teardrop", audio.ReleaseSingle)
	if err := tg.Tag(context.Background(), trackID); err != nil {
		t.Fatal(err)
	}
	tr, _ := db.GetTrack(db.All, trackID)
	if tr.MBID != "a31e069b-1025-4700-971d-c6cfe937e31a" {
		t.Fatalf("mbid = %q", tr.MBID)
	}
}

func TestTagNoMatchAndReview(t *testing.T) {
	dbtest.New(t)
	tg := testTagger(t)
	// nothing like the search results
	other, _ := addTrack(t, "Completely Different", "Nobody", time.Minute, "", "Other", audio.ReleaseSingle)
	if err := tg.Tag(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if counts, _ := db.TagCounts(db.All); counts[db.TagNone] != 1 {
		t.Fatalf("counts = %v", counts)
	}

	// the right title but another artist and length: worth a look, not auto-applied
	close, _ := addTrack(t, "Teardrop", "Someone Else", 200*time.Second, "", "Teardrop", audio.ReleaseSingle)
	if err := tg.Tag(context.Background(), close); err != nil {
		t.Fatal(err)
	}
	pending, _ := db.PendingMatches(db.All)
	if len(pending) == 0 || pending[0].TrackID != close {
		t.Fatalf("pending = %+v", pending)
	}
	tr, _ := db.GetTrack(db.All, close)
	if tr.MBID != "" {
		t.Fatal("uncertain match was applied")
	}

	// rejected candidates aren't proposed again
	for _, p := range pending {
		if err := db.RejectMatch(p.TrackID, p.RecordingID); err != nil {
			t.Fatal(err)
		}
	}
	if err := tg.Tag(context.Background(), close); err != nil {
		t.Fatal(err)
	}
	if pending, _ := db.PendingMatches(db.All); len(pending) != 0 {
		t.Fatalf("rejected came back: %+v", pending)
	}

	// applying from review
	if counts, _ := db.TagCounts(db.All); counts[db.TagNone] != 2 {
		t.Fatalf("counts after reject = %v", counts)
	}
}

func TestPlaceholdersSkipped(t *testing.T) {
	dbtest.New(t)
	tg := testTagger(t)
	trackID, _ := addTrack(t, "Teardrop", "Massive Attack", time.Minute, "", "Teardrop", audio.ReleaseSingle)
	db.DB.Exec(`UPDATE tracks SET has_file = 0 WHERE uuid = ?;`, trackID)
	if err := tg.Tag(context.Background(), trackID); err != nil {
		t.Fatal(err)
	}
	if counts, _ := db.TagCounts(db.All); len(counts) != 0 {
		t.Fatalf("placeholder was tagged: %v", counts)
	}
}
