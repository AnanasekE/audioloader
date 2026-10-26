package downloads

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/sources"
	"audioloader/internal/sources/folder"
	"audioloader/internal/sources/ytdlp"
)

func fakeDownloader(t *testing.T, extra ...sources.Source) *Downloader {
	t.Helper()
	bin, _ := filepath.Abs("../sources/ytdlp/testdata/fake-yt-dlp")
	yt, err := ytdlp.New(bin, "true")
	if err != nil {
		t.Fatal(err)
	}
	return &Downloader{
		Sources: append(extra, ytdlp.Source{C: yt}),
		YT:      yt,
	}
}

// confirmedRelease makes a release with placeholder tracks, as confirming
// an inbox release does, and returns the placeholders' IDs.
func confirmedRelease(t *testing.T, titles ...string) (*audio.Release, []string) {
	t.Helper()
	a, _ := db.AddArtist("Massive Attack")
	var tracks []db.InboxTrack
	for i, title := range titles {
		tracks = append(tracks, db.InboxTrack{Disc: 1, Position: i + 1, Title: title, LengthMs: 2000,
			Artists: []db.MatchArtist{{Name: "Massive Attack"}}})
	}
	err := db.AddInboxRelease(db.InboxRelease{ReleaseGroupMBID: "rg", ArtistID: a.ID.String(), Title: "Mezzanine", Type: "album",
		Data: db.InboxData{Artists: []db.MatchArtist{{Name: "Massive Attack"}}, Tracks: tracks}})
	if err != nil {
		t.Fatal(err)
	}
	relID, ph, err := db.ConfirmInboxRelease("rg")
	if err != nil {
		t.Fatal(err)
	}
	r, _ := db.GetRelease(db.All, relID)
	return r, ph
}

func TestRunFromVideo(t *testing.T) {
	dbtest.New(t)
	d := fakeDownloader(t)
	_, ph := confirmedRelease(t, "Angel", "Teardrop")

	if err := d.Run(context.Background(), ph[0], "AAAAAAAAAA1"); err != nil {
		t.Fatal(err)
	}
	tr, _ := db.GetTrack(db.All, ph[0])
	if !tr.HasFile || tr.Duration != 2*time.Second {
		t.Fatalf("track = %+v", tr)
	}
	dl, _ := db.GetDownload(db.All, ph[0])
	if dl.Status != db.DownloadDone || dl.Source != ytdlp.Name {
		t.Fatalf("download = %+v", dl)
	}
	if id, _ := db.TrackBySourceURL(ytdlp.VideoURL("AAAAAAAAAA1")); id != ph[0] {
		t.Fatalf("source url not stored: %q", id)
	}

	// the same video for another placeholder merges instead of downloading again
	if err := d.Run(context.Background(), ph[1], "AAAAAAAAAA1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetTrack(db.All, ph[1]); err == nil {
		t.Fatal("placeholder wasn't merged")
	}
}

func TestRunFailures(t *testing.T) {
	dbtest.New(t)
	d := fakeDownloader(t)
	_, ph := confirmedRelease(t, "Angel")

	// no source has it
	if err := d.Run(context.Background(), ph[0], ""); err != nil {
		t.Fatal(err)
	}
	if dl, _ := db.GetDownload(db.All, ph[0]); dl.Status != db.DownloadNotFound {
		t.Fatalf("status = %s", dl.Status)
	}
	// yt-dlp fails
	if err := d.Run(context.Background(), ph[0], "failfailfai"); err == nil {
		t.Fatal("no error")
	}
	if dl, _ := db.GetDownload(db.All, ph[0]); dl.Status != db.DownloadFailed || dl.LastError == "" {
		t.Fatalf("download = %+v", dl)
	}
}

func TestRunFromFolder(t *testing.T) {
	dbtest.New(t)
	dir := t.TempDir()
	data, _ := os.ReadFile("../tags/testdata/tagged.flac")
	os.WriteFile(filepath.Join(dir, "teardrop.flac"), data, 0o644)
	d := fakeDownloader(t, &folder.Source{Dir: dir})
	_, ph := confirmedRelease(t, "Teardrop")
	db.DB.Exec(`UPDATE tracks SET mbid = 'f3bba4cd-8018-468b-902e-bc8f029593e5' WHERE uuid = ?;`, ph[0])

	if err := d.Run(context.Background(), ph[0], ""); err != nil {
		t.Fatal(err)
	}
	dl, _ := db.GetDownload(db.All, ph[0])
	if dl.Status != db.DownloadDone || dl.Source != folder.Name || !dl.Track.HasFile {
		t.Fatalf("download = %+v", dl)
	}
	// the watched file is the user's: it stays
	if _, err := os.Stat(filepath.Join(dir, "teardrop.flac")); err != nil {
		t.Fatal(err)
	}
}

func TestImport(t *testing.T) {
	dbtest.New(t)
	d := fakeDownloader(t)
	a, _ := db.AddArtist("Massive Attack")
	imp := YouTubeImport{VideoID: "aqz-KE-bpKQ", Title: "Teardrop", ArtistIDs: []string{a.ID.String()}}
	if err := d.Import(context.Background(), imp); err != nil {
		t.Fatal(err)
	}
	tracks, _ := db.ListTracks(db.All)
	if len(tracks) != 1 || tracks[0].Title != "Teardrop" || len(tracks[0].Releases) != 1 {
		t.Fatalf("tracks = %+v", tracks)
	}
	// importing the same video onto an album places the existing track
	r, _ := db.CreateRelease("Mezzanine", audio.ReleaseAlbum, "", []string{a.ID.String()})
	imp.ReleaseID, imp.Disc, imp.Position = r.ID.String(), 1, 3
	if err := d.Import(context.Background(), imp); err != nil {
		t.Fatal(err)
	}
	if tracks, _ := db.ListTracks(db.All); len(tracks) != 1 || len(tracks[0].Releases) != 2 {
		t.Fatalf("after second import: %+v", tracks)
	}
}

func TestCleanTitle(t *testing.T) {
	cases := map[string]string{
		"Massive Attack - Teardrop (Official Video)": "Teardrop",
		"Teardrop [Official Audio]":                  "Teardrop",
		"Massive Attack – Angel (Lyrics) (HD)":       "Angel",
		"Someone Else - Teardrop":                    "Someone Else - Teardrop",
		"Teardrop (Remastered 2019)":                 "Teardrop (Remastered 2019)",
	}
	for in, want := range cases {
		if got := CleanTitle(in, []string{"Massive Attack"}); got != want {
			t.Errorf("CleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMapPlaylist(t *testing.T) {
	r := &audio.Release{Artists: []*audio.Artist{{Name: "Massive Attack"}}}
	for i, tt := range []struct {
		title string
		dur   time.Duration
		has   bool
	}{{"Angel", 379 * time.Second, false}, {"Risingson", 299 * time.Second, true}, {"Teardrop", 331 * time.Second, false}, {"Inertia Creeps", 357 * time.Second, false}} {
		r.Tracks = append(r.Tracks, &audio.ReleaseTrack{Disc: 1, Position: i + 1, Track: &audio.Track{Title: tt.title, Duration: tt.dur, HasFile: tt.has}})
	}
	videos := []ytdlp.Entry{
		{ID: "v1", Title: "Massive Attack - Angel", Duration: 380},
		{ID: "v2", Title: "Risingson", Duration: 299},
		{ID: "v3", Title: "Teardrop (Official Video)", Duration: 300}, // too short: a radio edit
		{ID: "v4", Title: "Inertia Creeps (Remastered)", Duration: 356},
	}
	m := MapPlaylist(r, videos)
	got := map[string]string{}
	for _, x := range m {
		got[x.Track.Track.Title] = x.VideoID
	}
	want := map[string]string{"Angel": "v1", "Teardrop": "", "Inertia Creeps": "v4"}
	if len(got) != len(want) {
		t.Fatalf("mapped %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s -> %q, want %q", k, got[k], v)
		}
	}
}
