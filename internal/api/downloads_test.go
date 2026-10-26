package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/sources/ytdlp"
	"audioloader/internal/vault"
)

// placeholder confirms a one-track release and returns the placeholder's ID.
func placeholder(t *testing.T) string {
	t.Helper()
	a, _ := db.AddArtist(db.All, "Massive Attack")
	err := db.AddInboxRelease(db.InboxRelease{ReleaseGroupMBID: "rg", ArtistID: a.ID.String(), Title: "Mezzanine", Type: "album",
		Data: db.InboxData{Artists: []db.MatchArtist{{Name: "Massive Attack"}}, Tracks: []db.InboxTrack{
			{Disc: 1, Position: 1, Title: "Teardrop", LengthMs: 331000, Artists: []db.MatchArtist{{Name: "Massive Attack"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, ph, err := db.ConfirmInboxRelease("rg")
	if err != nil {
		t.Fatal(err)
	}
	return ph[0]
}

func downloadsHandler(t *testing.T) http.Handler {
	t.Helper()
	bin, _ := filepath.Abs("../sources/ytdlp/testdata/fake-yt-dlp")
	yt, err := ytdlp.New(bin, "true")
	if err != nil {
		t.Fatal(err)
	}
	downloads.SetDefault(&downloads.Downloader{YT: yt})
	hash, _ := vault.Hash("pass")
	mux := http.NewServeMux()
	Register(mux, vault.New([]byte(hash), []byte("0123456789abcdef0123456789abcdef")))
	return WithViewer(mux)
}

func do(h http.Handler, method, target string, form url.Values) *httptest.ResponseRecorder {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPickFromYouTube(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	id := placeholder(t)
	db.SetDownload(id, db.DownloadNeedsPick, "", 0, "")

	// the page lists it under "Needs a pick", and the nav badge counts it
	if body := do(h, "GET", "/downloads", nil).Body.String(); !strings.Contains(body, "Needs a pick") || !strings.Contains(body, "Pick from YouTube") {
		t.Fatalf("downloads page:\n%s", body)
	}
	if body := do(h, "GET", "/ui/badges/downloads", nil).Body.String(); !strings.Contains(body, ">1<") {
		t.Fatalf("badge = %q", body)
	}

	// up to 5 candidates with preview links and badges
	rec := do(h, "GET", "/ui/downloads/"+id+"/youtube/search", nil)
	body := rec.Body.String()
	if rec.Code != 200 || strings.Count(body, "Use this") != 5 || !strings.Contains(body, "Topic") || !strings.Contains(body, "Verified") ||
		!strings.Contains(body, "https://www.youtube.com/watch?v=aqz-KE-bpKQ") || !strings.Contains(body, "https://i.ytimg.com/vi/") ||
		!strings.Contains(body, "same length") || !strings.Contains(body, "-88 s") {
		t.Fatalf("search = %d\n%s", rec.Code, body)
	}

	// a bad video ID is refused; a good one queues the download
	if rec := do(h, "POST", "/ui/downloads/"+id+"/youtube", url.Values{"video_id": {"--exec=rm"}}); rec.Code != 400 {
		t.Fatalf("bad id: %d", rec.Code)
	}
	if rec := do(h, "POST", "/ui/downloads/"+id+"/youtube", url.Values{"video_id": {"aqz-KE-bpKQ"}}); rec.Code != 204 {
		t.Fatalf("use: %d %s", rec.Code, rec.Body)
	}
	if counts, _ := jobs.Counts(downloads.JobDownload); counts["queued"] != 1 {
		t.Fatalf("jobs = %v", counts)
	}

	// "None of these" gives up
	if rec := do(h, "POST", "/ui/downloads/"+id+"/youtube/none", nil); rec.Code != 204 {
		t.Fatalf("none: %d", rec.Code)
	}
	if dl, _ := db.GetDownload(db.All, id); dl.Status != db.DownloadNotFound {
		t.Fatalf("status = %s", dl.Status)
	}
}

func TestPickFromYouTubeErrors(t *testing.T) {
	dbtest.New(t)
	h := downloadsHandler(t)
	// not a download at all
	if rec := do(h, "GET", "/ui/downloads/00000000-0000-0000-0000-000000000000/youtube/search", nil); rec.Code != 404 {
		t.Fatalf("unknown track: %d", rec.Code)
	}
	// yt-dlp isn't installed
	id := placeholder(t)
	downloads.SetDefault(&downloads.Downloader{YTErr: ytdlp.ErrUnavailable})
	if rec := do(h, "GET", "/ui/downloads/"+id+"/youtube/search", nil); rec.Code != 503 {
		t.Fatalf("no yt-dlp: %d", rec.Code)
	}
}
