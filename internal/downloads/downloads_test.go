package downloads

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/sources"
	"audioloader/internal/sources/folder"
	"audioloader/internal/sources/spotdl"
	"audioloader/internal/sources/ytdlp"
)

func fakeDownloader(t *testing.T, extra ...sources.Source) *Downloader {
	t.Helper()
	bin, _ := filepath.Abs("../sources/ytdlp/testdata/fake-yt-dlp")
	yt, err := ytdlp.New(bin, "true")
	if err != nil {
		t.Fatal(err)
	}
	spBin, _ := filepath.Abs("../sources/spotdl/testdata/fake-spotdl")
	sp, err := spotdl.New(spBin, "true")
	if err != nil {
		t.Fatal(err)
	}
	return &Downloader{
		Sources: append(extra, spotdl.Source{C: sp}, ytdlp.MusicSource{C: yt}, ytdlp.Source{C: yt}),
		YT:      yt,
		SP:      sp,
	}
}

// confirmedRelease makes a release with placeholder tracks, as confirming
// an inbox release does, and returns the placeholders' IDs.
func confirmedRelease(t *testing.T, titles ...string) (*audio.Release, []string) {
	t.Helper()
	a, _ := db.AddArtist(db.All, "Massive Attack")
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

	// a pick queued after the file arrived ends as done, not stuck queued
	if err := EnqueueVideo(ph[0], "AAAAAAAAAA2"); err != nil {
		t.Fatal(err)
	}
	if err := d.Run(context.Background(), ph[0], "AAAAAAAAAA2"); err != nil {
		t.Fatal(err)
	}
	if dl, _ := db.GetDownload(db.All, ph[0]); dl.Status != db.DownloadDone {
		t.Fatalf("download = %+v", dl)
	}
}

func TestRunFailures(t *testing.T) {
	dbtest.New(t)
	d := fakeDownloader(t)
	_, ph := confirmedRelease(t, "Angel")

	// no source has it, and YouTube waits for a pick
	if err := d.Run(context.Background(), ph[0], ""); err != nil {
		t.Fatal(err)
	}
	if dl, _ := db.GetDownload(db.All, ph[0]); dl.Status != db.DownloadNeedsPick {
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

// TestRunSourceOrder covers who wins when several sources have a track.
func TestRunSourceOrder(t *testing.T) {
	dbtest.New(t)
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("FAKE_SPOTDL_LOG", log)
	dir := t.TempDir()
	data, _ := os.ReadFile("../tags/testdata/tagged.flac")
	os.WriteFile(filepath.Join(dir, "teardrop.flac"), data, 0o644)
	d := fakeDownloader(t, &folder.Source{Dir: dir})
	_, ph := confirmedRelease(t, "Teardrop", "Angel")
	db.DB.Exec(`UPDATE tracks SET mbid = 'f3bba4cd-8018-468b-902e-bc8f029593e5' WHERE uuid = ?;`, ph[0])
	db.DB.Exec(`UPDATE tracks SET spotify_url = 'https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb' WHERE uuid IN (?, ?);`, ph[0], ph[1])
	status := func(id string) *db.Download {
		dl, err := db.GetDownload(db.All, id)
		if err != nil {
			t.Fatal(err)
		}
		return dl
	}

	// folder wins over spotDL, which isn't even asked
	if err := d.Run(context.Background(), ph[0], ""); err != nil {
		t.Fatal(err)
	}
	if dl := status(ph[0]); dl.Source != folder.Name || dl.Status != db.DownloadDone {
		t.Fatalf("download = %+v", dl)
	}
	if b, _ := os.ReadFile(log); len(b) != 0 {
		t.Fatalf("spotdl ran: %q", b)
	}
	// spotDL wins over YouTube
	if err := d.Run(context.Background(), ph[1], ""); err != nil {
		t.Fatal(err)
	}
	if dl := status(ph[1]); dl.Source != spotdl.Name || dl.Status != db.DownloadDone {
		t.Fatalf("download = %+v", dl)
	}
}

func TestRunNeedsPick(t *testing.T) {
	dbtest.New(t)
	_, ph := confirmedRelease(t, "Angel", "Inertia Creeps")

	// only YouTube is left: wait for a pick
	d := fakeDownloader(t)
	if err := d.Run(context.Background(), ph[0], ""); err != nil {
		t.Fatal(err)
	}
	if dl, _ := db.GetDownload(db.All, ph[0]); dl.Status != db.DownloadNeedsPick || dl.Track.HasFile {
		t.Fatalf("download = %+v", dl)
	}
	// the pick runs like a pasted URL
	if err := d.Run(context.Background(), ph[0], "AAAAAAAAAA1"); err != nil {
		t.Fatal(err)
	}
	if dl, _ := db.GetDownload(db.All, ph[0]); dl.Status != db.DownloadDone || dl.Source != ytdlp.Name {
		t.Fatalf("download = %+v", dl)
	}

	// without yt-dlp there is nothing to pick from
	d.YT, d.YTErr = nil, ytdlp.ErrUnavailable
	d.Sources = d.Sources[:len(d.Sources)-2] // YouTube Music and YouTube
	if err := d.Run(context.Background(), ph[1], ""); err != nil {
		t.Fatal(err)
	}
	if dl, _ := db.GetDownload(db.All, ph[1]); dl.Status != db.DownloadNotFound {
		t.Fatalf("status = %s", dl.Status)
	}
}

// teardropRelease confirms a release of n placeholders titled "Teardrop",
// 331 s long as on Mezzanine.
func teardropRelease(t *testing.T, n int) []string {
	t.Helper()
	titles := make([]string, n)
	for i := range titles {
		titles[i] = "Teardrop"
	}
	_, ph := confirmedRelease(t, titles...)
	for _, id := range ph {
		db.DB.Exec(`UPDATE tracks SET duration_seconds = 331 WHERE uuid = ?;`, id)
	}
	return ph
}

func TestRunFromYouTubeMusic(t *testing.T) {
	dbtest.New(t)
	d := fakeDownloader(t)
	ph := teardropRelease(t, 2)

	// the Topic song is 1 s off: confident, so no pick is needed
	if err := d.Run(context.Background(), ph[0], ""); err != nil {
		t.Fatal(err)
	}
	dl, _ := db.GetDownload(db.All, ph[0])
	if dl.Status != db.DownloadDone || dl.Source != ytdlp.MusicName || !dl.Track.HasFile {
		t.Fatalf("download = %+v", dl)
	}
	if id, _ := db.TrackBySourceURL(ytdlp.VideoURL("u7qY1uQ2l0w")); id != ph[0] {
		t.Fatalf("source url not stored: %q", id)
	}

	// the same song for another placeholder merges instead of storing a second file
	if err := d.Run(context.Background(), ph[1], ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetTrack(db.All, ph[1]); err == nil {
		t.Fatal("placeholder wasn't merged")
	}
}

func TestRunYouTubeMusicNeedsConfidence(t *testing.T) {
	dbtest.New(t)
	d := fakeDownloader(t)
	ph := teardropRelease(t, 1)
	db.DB.Exec(`UPDATE tracks SET duration_seconds = 351 WHERE uuid = ?;`, ph[0])

	// 20 s off: the pick is left to the user
	if err := d.Run(context.Background(), ph[0], ""); err != nil {
		t.Fatal(err)
	}
	if dl, _ := db.GetDownload(db.All, ph[0]); dl.Status != db.DownloadNeedsPick || dl.Track.HasFile {
		t.Fatalf("download = %+v", dl)
	}
}

func TestRunFolderBeforeYouTubeMusic(t *testing.T) {
	dbtest.New(t)
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("FAKE_YTDLP_LOG", log)
	dir := t.TempDir()
	data, _ := os.ReadFile("../tags/testdata/tagged.flac")
	os.WriteFile(filepath.Join(dir, "teardrop.flac"), data, 0o644)
	d := fakeDownloader(t, &folder.Source{Dir: dir})
	ph := teardropRelease(t, 1)
	db.DB.Exec(`UPDATE tracks SET mbid = 'f3bba4cd-8018-468b-902e-bc8f029593e5' WHERE uuid = ?;`, ph[0])

	if err := d.Run(context.Background(), ph[0], ""); err != nil {
		t.Fatal(err)
	}
	if dl, _ := db.GetDownload(db.All, ph[0]); dl.Source != folder.Name || dl.Status != db.DownloadDone {
		t.Fatalf("download = %+v", dl)
	}
	if b, _ := os.ReadFile(log); len(b) != 0 {
		t.Fatalf("yt-dlp was asked: %q", b)
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

func TestRunFromSpotdl(t *testing.T) {
	dbtest.New(t)
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("FAKE_SPOTDL_LOG", log)
	d := fakeDownloader(t)
	_, ph := confirmedRelease(t, "Angel", "Teardrop")
	calls := func() string { b, _ := os.ReadFile(log); return string(b) }

	// a track with its own link downloads it with no search
	db.DB.Exec(`UPDATE tracks SET spotify_url = 'https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb' WHERE uuid = ?;`, ph[1])
	if err := d.Run(context.Background(), ph[1], ""); err != nil {
		t.Fatal(err)
	}
	dl, _ := db.GetDownload(db.All, ph[1])
	if dl.Status != db.DownloadDone || dl.Source != spotdl.Name || !dl.Track.HasFile {
		t.Fatalf("download = %+v", dl)
	}
	if got := calls(); got != "download https://open.spotify.com/track/2dgjBzSYiTVqlSc3u1Yosb\n" {
		t.Fatalf("calls = %q", got)
	}

	// without a link, a search result of the wrong length (the fake has Angel at 380 s) is rejected
	if err := d.Run(context.Background(), ph[0], ""); err != nil {
		t.Fatal(err)
	}
	if dl, _ := db.GetDownload(db.All, ph[0]); dl.Status != db.DownloadNeedsPick {
		t.Fatalf("status = %s", dl.Status)
	}
}

func TestRunSearchAccepted(t *testing.T) {
	dbtest.New(t)
	d := fakeDownloader(t)
	_, ph := confirmedRelease(t, "Teardrop")
	if err := d.Run(context.Background(), ph[0], ""); err != nil {
		t.Fatal(err)
	}
	dl, _ := db.GetDownload(db.All, ph[0])
	if dl.Status != db.DownloadDone || dl.Source != spotdl.Name {
		t.Fatalf("download = %+v", dl)
	}
}

func TestRunMapsSpotifyAlbum(t *testing.T) {
	dbtest.New(t)
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("FAKE_SPOTDL_LOG", log)
	d := fakeDownloader(t)
	rel, ph := confirmedRelease(t, "Angel", "Teardrop", "Inertia Creeps")
	db.DB.Exec(`UPDATE releases SET spotify_url = 'https://open.spotify.com/album/0sNOF9WDwhWunNAHPD3Baj' WHERE uuid = ?;`, rel.ID.String())

	for _, id := range ph[:2] {
		if err := d.Run(context.Background(), id, ""); err != nil {
			t.Fatal(err)
		}
		if dl, _ := db.GetDownload(db.All, id); dl.Status != db.DownloadDone || dl.Source != spotdl.Name {
			t.Fatalf("download = %+v", dl)
		}
	}
	// one album lookup for both, and no search; the mapped links are kept
	b, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 || lines[0] != "save https://open.spotify.com/album/0sNOF9WDwhWunNAHPD3Baj" || strings.Contains(string(b), "Massive Attack -") {
		t.Fatalf("calls = %q", lines)
	}
	dl, _ := db.GetDownload(db.All, ph[2])
	if dl.SpotifyURL != "" {
		t.Fatalf("unmapped track got %q", dl.SpotifyURL)
	}
}

func TestImport(t *testing.T) {
	dbtest.New(t)
	d := fakeDownloader(t)
	a, _ := db.AddArtist(db.All, "Massive Attack")
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

func TestGuessArtist(t *testing.T) {
	cases := []struct{ title, channel, want string }{
		{"Massive Attack - Teardrop (Official Video)", "Some Channel", "Massive Attack"},
		{"Massive Attack – Angel", "", "Massive Attack"},
		{"Teardrop", "Massive Attack - Topic", "Massive Attack"},
		{"Teardrop", "Massive Attack", "Massive Attack"},
		{"Teardrop", "", ""},
	}
	for _, c := range cases {
		if got := GuessArtist(ytdlp.Entry{Title: c.title, Channel: c.channel}); got != c.want {
			t.Errorf("GuessArtist(%q, %q) = %q, want %q", c.title, c.channel, got, c.want)
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
		got[x.Track.Track.Title] = x.ID
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
