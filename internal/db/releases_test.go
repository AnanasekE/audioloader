package db

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	"audioloader/internal/audio"
)

func newTestRelease(t *testing.T, title string, typ audio.ReleaseType) (*audio.Release, string) {
	t.Helper()
	a, err := AddArtist(All, "artist " + title)
	if err != nil {
		t.Fatal(err)
	}
	r, err := CreateRelease(title, typ, "", []string{a.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	return r, a.ID.String()
}

func TestUpdateReleaseKeepsTracklist(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "old", audio.ReleaseAlbum)
	tr := addTestTrack(t, "song", "x")
	if err := SetReleaseTracks(All, r.ID.String(), []ReleaseTrackInput{{TrackID: tr, Disc: 1}}); err != nil {
		t.Fatal(err)
	}
	b, err := AddArtist(All, "other")
	if err != nil {
		t.Fatal(err)
	}

	if err := UpdateRelease(r.ID.String(), "new", audio.ReleaseEP, "2020", []string{b.ID.String()}); err != nil {
		t.Fatal(err)
	}
	got, err := GetRelease(All, r.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "new" || got.Type != audio.ReleaseEP || got.Date != "2020" {
		t.Fatalf("got %+v", got)
	}
	if len(got.Artists) != 1 || got.Artists[0].Name != "other" {
		t.Fatalf("artists = %v", got.Artists)
	}
	if len(got.Tracks) != 1 {
		t.Fatalf("tracks = %d, want 1", len(got.Tracks))
	}

	if err := UpdateRelease("00000000-0000-0000-0000-000000000000", "x", audio.ReleaseEP, "", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing release err = %v", err)
	}
}

func TestDeleteReleaseKeepsTracks(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "rel", audio.ReleaseAlbum)
	tr := addTestTrack(t, "song", "x")
	if err := SetReleaseTracks(All, r.ID.String(), []ReleaseTrackInput{{TrackID: tr, Disc: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteRelease(r.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := GetRelease(All, r.ID.String()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRelease err = %v", err)
	}
	if _, err := GetTrack(All, tr); err != nil {
		t.Fatalf("track gone: %v", err)
	}
}

func TestReleaseCoverOnTracks(t *testing.T) {
	newTestDB(t)
	album, _ := newTestRelease(t, "album", audio.ReleaseAlbum)
	tr := addTestTrack(t, "song", "x") // also creates its single
	if err := SetReleaseTracks(All, album.ID.String(), []ReleaseTrackInput{{TrackID: tr, Disc: 1}}); err != nil {
		t.Fatal(err)
	}
	track, _ := GetTrack(All, tr)
	if track.ImgURL != DefaultCover {
		t.Fatalf("no covers yet, got %q", track.ImgURL)
	}

	single := track.Releases[0]
	if single.Type != audio.ReleaseSingle {
		single = track.Releases[1]
	}
	if err := SetReleaseCover(single.ID, strings.NewReader("png"), ".png"); err != nil {
		t.Fatal(err)
	}
	track, _ = GetTrack(All, tr)
	if !strings.HasPrefix(track.ImgURL, "/covers/"+single.ID.String()) {
		t.Fatalf("want the single's cover, got %q", track.ImgURL)
	}

	if err := SetReleaseCover(album.ID, strings.NewReader("jpg"), ".jpg"); err != nil {
		t.Fatal(err)
	}
	track, _ = GetTrack(All, tr)
	if !strings.HasPrefix(track.ImgURL, "/covers/"+album.ID.String()) {
		t.Fatalf("album cover should win, got %q", track.ImgURL)
	}
	// replacing with another extension leaves one file
	if err := SetReleaseCover(album.ID, strings.NewReader("png"), ".png"); err != nil {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(filepath.Join(CoversDir, album.ID.String()+".*")); len(m) != 1 {
		t.Fatalf("cover files = %v", m)
	}
}

func TestRemoveRelease(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(All, "Artist")
	if err := AddInboxRelease(InboxRelease{ReleaseGroupMBID: "rg", ArtistID: a.ID.String(), Title: "Album", Type: "album",
		Data: InboxData{Tracks: []InboxTrack{{Disc: 1, Position: 1, Title: "Missing"}}}}); err != nil {
		t.Fatal(err)
	}
	relID, placeholders, err := ConfirmInboxRelease("rg")
	if err != nil {
		t.Fatal(err)
	}
	placeholder := placeholders[0]
	if _, err := DB.Exec(`INSERT INTO jobs(type, payload) VALUES('download', ?);`, `{"track_id":"`+placeholder+`"}`); err != nil {
		t.Fatal(err)
	}

	res, err := AddTrack(All, []string{a.ID.String()}, &audio.Track{Title: "Only here"}, strings.NewReader("only"), ".mp3", &ReleaseLink{ReleaseID: relID, Disc: 1, Position: 2}, "")
	if err != nil {
		t.Fatal(err)
	}
	onlyHere := res.TrackID
	onlyPath, err := TrackFilePath(uuid.MustParse(onlyHere))
	if err != nil {
		t.Fatal(err)
	}

	shared := addTestTrack(t, "Also a single", "shared", a.ID.String())
	single, err := CreateRelease("Single", audio.ReleaseSingle, "", []string{a.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if err := SetReleaseTracks(All, single.ID.String(), []ReleaseTrackInput{{TrackID: shared, Disc: 1}}); err != nil {
		t.Fatal(err)
	}
	secret, err := AddTrack(Secret, []string{a.ID.String()}, &audio.Track{Title: "Hidden"}, strings.NewReader("hidden"), ".mp3", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetReleaseTracks(All, relID, []ReleaseTrackInput{
		{TrackID: placeholder, Disc: 1}, {TrackID: onlyHere, Disc: 1}, {TrackID: shared, Disc: 1}, {TrackID: secret.TrackID, Disc: 1}}); err != nil {
		t.Fatal(err)
	}

	got, err := RemoveRelease(Normal, relID, "https://caa.example")
	if err != nil {
		t.Fatal(err)
	}
	if got != (ReleaseRemoval{Tracks: 2, WithFiles: 1}) {
		t.Fatalf("removal = %+v", got)
	}
	if _, err := GetRelease(All, relID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("release err = %v", err)
	}
	for _, id := range []string{placeholder, onlyHere} {
		if _, err := GetTrack(All, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("track %s err = %v", id, err)
		}
	}
	if _, err := os.Stat(onlyPath); !os.IsNotExist(err) {
		t.Errorf("file still there: %v", err)
	}
	if _, err := GetTrack(All, shared); err != nil {
		t.Errorf("shared track gone: %v", err)
	}
	if _, err := GetRelease(All, single.ID.String()); err != nil {
		t.Errorf("single gone: %v", err)
	}
	// the secret track survives, off the release
	if _, err := GetTrack(Secret, secret.TrackID); err != nil {
		t.Errorf("secret track gone: %v", err)
	}
	var queued int
	DB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status = 'queued' AND instr(payload, ?) > 0;`, placeholder).Scan(&queued)
	if queued != 0 {
		t.Errorf("%d queued jobs mention the placeholder", queued)
	}
	if _, ig, _ := InboxCounts(); ig != 1 {
		t.Errorf("ignored = %d, want 1", ig)
	}

	if _, err := RemoveRelease(Normal, relID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("second remove err = %v", err)
	}
}

func TestRemoveReleaseWithoutInboxRow(t *testing.T) {
	newTestDB(t)
	r, _ := newTestRelease(t, "Uploaded", audio.ReleaseAlbum)
	tr := addTestTrack(t, "song", "x")
	if err := SetReleaseTracks(All, r.ID.String(), []ReleaseTrackInput{{TrackID: tr, Disc: 1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`UPDATE releases SET release_group_mbid = 'rg-up' WHERE uuid = ?;`, r.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveRelease(Normal, r.ID.String(), "https://caa.example"); err != nil {
		t.Fatal(err)
	}
	if known, _ := InboxKnows("rg-up"); !known {
		t.Fatal("no inbox row")
	}
	ir, err := GetInboxRelease("rg-up")
	if err != nil {
		t.Fatal(err)
	}
	if ir.Status != "ignored" || ir.Title != "Uploaded" {
		t.Fatalf("inbox row = %+v", ir)
	}
}
