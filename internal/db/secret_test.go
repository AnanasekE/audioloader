package db

import (
	"bytes"
	"errors"
	"testing"

	"audioloader/internal/audio"
)

// modeSeed is a library with a side for each mode and one thing in between.
type modeSeed struct {
	normalTrack, secretTrack, sharedNormal, sharedSecret string
	normalArtist, secretArtist, sharedArtist             string
	waitingNormal, waitingSecret                         string // artists without tracks
	mixedRelease, emptyRelease                           string
	normalShow, secretShow                               string
}

func seedModes(t *testing.T) modeSeed {
	t.Helper()
	var m modeSeed
	pa, _ := AddArtist(Normal, "Normal Artist")
	sa, _ := AddArtist(Secret, "Secret Artist")
	sh, _ := AddArtist(Normal, "Shared Artist")
	m.normalArtist, m.secretArtist, m.sharedArtist = pa.ID.String(), sa.ID.String(), sh.ID.String()
	wn, _ := AddArtist(Normal, "Waiting Normal")
	ws, _ := AddArtist(Secret, "Waiting Secret")
	m.waitingNormal, m.waitingSecret = wn.ID.String(), ws.ID.String()

	m.normalTrack = addTestTrack(t, "ZZPUBLIC", "n1", m.normalArtist)
	m.sharedNormal = addTestTrack(t, "Shared Normal", "n2", m.sharedArtist)
	for _, c := range []struct {
		dst  *string
		name string
		body string
		a    string
	}{{&m.secretTrack, "ZZSECRET", "s1", m.secretArtist}, {&m.sharedSecret, "Shared Secret", "s2", m.sharedArtist}} {
		res, err := AddTrack(Secret, []string{c.a}, &audio.Track{Title: c.name}, bytes.NewReader([]byte(c.body)), ".mp3", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		*c.dst = res.TrackID
	}

	r, _ := CreateRelease("Mixed", audio.ReleaseAlbum, "", []string{m.normalArtist})
	m.mixedRelease = r.ID.String()
	if err := SetReleaseTracks(All, m.mixedRelease, []ReleaseTrackInput{{m.normalTrack, 1}}); err != nil {
		t.Fatal(err)
	}
	e, _ := CreateRelease("Empty", audio.ReleaseEP, "", []string{m.normalArtist})
	m.emptyRelease = e.ID.String()

	ns, _ := ResolveShow(Normal, "Normal Show", "")
	ss, _ := ResolveShow(Secret, "Secret Show", "")
	m.normalShow, m.secretShow = ns.ID.String(), ss.ID.String()
	if _, err := AddEpisode(Normal, m.normalShow, &audio.Track{Title: "Normal Episode"}, bytes.NewReader([]byte("e1")), ".mp3"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddEpisode(Secret, m.secretShow, &audio.Track{Title: "Secret Episode"}, bytes.NewReader([]byte("e2")), ".mp3"); err != nil {
		t.Fatal(err)
	}
	return m
}

func names(as []*audio.Artist) []string {
	out := []string{}
	for _, a := range as {
		out = append(out, a.Name)
	}
	return out
}

func TestModesSeeOnlyTheirSide(t *testing.T) {
	newTestDB(t)
	m := seedModes(t)

	tracks := func(v Viewer) string { return titles(must(ListTracks(v))) }
	// order isn't promised
	hasAll := func(got string, want ...string) bool {
		n := 0
		for _, w := range want {
			if bytes.Contains([]byte(got), []byte(w)) {
				n++
			}
		}
		return n == len(want) && len(bytes.Split([]byte(got), []byte(","))) == len(want)
	}
	if got := tracks(Normal); !hasAll(got, "ZZPUBLIC", "Shared Normal") {
		t.Fatalf("normal tracks = %s", got)
	}
	if got := tracks(Secret); !hasAll(got, "ZZSECRET", "Shared Secret") {
		t.Fatalf("secret tracks = %s", got)
	}
	if got := tracks(All); len(bytes.Split([]byte(got), []byte(","))) != 4 {
		t.Fatalf("all tracks = %s", got)
	}

	if _, err := GetTrack(Normal, m.secretTrack); !errors.Is(err, ErrNotFound) {
		t.Fatalf("normal sees a secret track: %v", err)
	}
	if _, err := GetTrack(Secret, m.normalTrack); !errors.Is(err, ErrNotFound) {
		t.Fatalf("secret sees a normal track: %v", err)
	}

	// artists: the shared one is on both sides, one made in secret mode waits there
	if got := names(must(ListArtists(Normal))); len(got) != 3 {
		t.Fatalf("normal artists = %v", got)
	}
	if got := names(must(ListArtists(Secret))); len(got) != 3 {
		t.Fatalf("secret artists = %v", got)
	}
	for _, c := range []struct {
		v    Viewer
		id   string
		want bool
	}{
		{Normal, m.normalArtist, true}, {Normal, m.secretArtist, false}, {Normal, m.sharedArtist, true},
		{Normal, m.waitingNormal, true}, {Normal, m.waitingSecret, false},
		{Secret, m.normalArtist, false}, {Secret, m.secretArtist, true}, {Secret, m.sharedArtist, true},
		{Secret, m.waitingNormal, false}, {Secret, m.waitingSecret, true},
	} {
		_, err := GetArtist(c.v, c.id)
		if (err == nil) != c.want {
			t.Errorf("GetArtist(mode %d, %s) = %v, want visible %v", c.v.Mode, c.id, err, c.want)
		}
	}

	// releases: secret mode has none; normal mode keeps empty ones
	if n := len(must(ListReleases(Secret))); n != 0 {
		t.Fatalf("secret releases = %d", n)
	}
	if _, err := GetRelease(Secret, m.mixedRelease); !errors.Is(err, ErrNotFound) {
		t.Fatalf("secret mode sees a release: %v", err)
	}
	if _, err := GetRelease(Normal, m.emptyRelease); err != nil {
		t.Fatalf("empty release hidden in normal mode: %v", err)
	}
	if _, err := GetRelease(Secret, m.emptyRelease); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty release shown in secret mode: %v", err)
	}

	// shows
	if got := must(ListShows(Normal)); len(got) != 1 || got[0].Title != "Normal Show" {
		t.Fatalf("normal shows = %+v", got)
	}
	if got := must(ListShows(Secret)); len(got) != 1 || got[0].Title != "Secret Show" {
		t.Fatalf("secret shows = %+v", got)
	}
	if _, err := GetShow(Normal, m.secretShow); !errors.Is(err, ErrNotFound) {
		t.Fatalf("normal sees a secret show: %v", err)
	}

	// search, which has its own index
	for _, c := range []struct {
		v    Viewer
		q    string
		want int
	}{
		{Normal, "zzsecret", 0}, {Secret, "zzpublic", 0}, {Secret, "zzsecret", 1}, {Normal, "zzpublic", 1},
		{Normal, "secret", 0}, {Secret, "normal", 0}, {All, "zzsecret", 1},
		{Normal, "shared", 3}, {Secret, "shared", 3}, // two tracks on the side and the artist
	} {
		res, _ := Search(c.v, c.q, 10)
		n := len(res.Tracks) + len(res.Artists) + len(res.Shows)
		if c.q == "shared" {
			c.want = 2
		}
		if n != c.want {
			t.Errorf("Search(mode %d, %q) found %d, want %d", c.v.Mode, c.q, n, c.want)
		}
	}
	if res, _ := Search(Normal, "waiting", 10); len(res.Artists) != 1 || res.Artists[0].Name != "Waiting Normal" {
		t.Errorf("normal search for waiting = %v", names(res.Artists))
	}
}

func TestSecretUploadHasNoRelease(t *testing.T) {
	newTestDB(t)
	m := seedModes(t)
	tr, _ := GetTrack(Secret, m.secretTrack)
	if !tr.Secret || len(tr.Releases) != 0 {
		t.Fatalf("secret upload = %+v", tr)
	}
	// even when asked to place it on a release
	res, err := AddTrack(Secret, []string{m.secretArtist}, &audio.Track{Title: "Linked"}, bytes.NewReader([]byte("l")), ".mp3",
		&ReleaseLink{ReleaseID: m.mixedRelease, Disc: 1, Position: 5}, "")
	if err != nil {
		t.Fatal(err)
	}
	if tr, _ := GetTrack(Secret, res.TrackID); len(tr.Releases) != 0 {
		t.Fatalf("placed on %v", tr.Releases)
	}
}

func TestSetSecretMovesTrack(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(Normal, "A")
	one := addTestTrack(t, "One", "one", a.ID.String()) // on its own single
	two := addTestTrack(t, "Two", "two", a.ID.String())
	album, _ := CreateRelease("Album", audio.ReleaseAlbum, "", []string{a.ID.String()})
	SetReleaseTracks(All, album.ID.String(), []ReleaseTrackInput{{one, 1}, {two, 1}})
	p, _ := CreatePlaylist("p")
	SetPlaylistTracks(All, p.ID.String(), []string{one, two})

	releases, _ := ReleasesOfTrack(one)
	if len(releases) != 2 {
		t.Fatalf("releases of one = %d", len(releases))
	}
	if err := SetSecret(one, true); err != nil {
		t.Fatal(err)
	}
	if tr, _ := GetTrack(Secret, one); tr == nil || len(tr.Releases) != 0 {
		t.Fatalf("moved track = %+v", tr)
	}
	// its single is gone, the album stays with the other track
	for _, r := range must(ListReleases(All)) {
		if r.Title == "One" {
			t.Fatal("empty single kept")
		}
	}
	if rel, _ := GetRelease(All, album.ID.String()); len(rel.Tracks) != 1 || rel.Tracks[0].Position != 2 {
		t.Fatalf("album = %+v", rel.Tracks)
	}
	// the normal playlist lost it
	if pl, _ := GetPlaylist(All, p.ID.String()); titles(pl.Tracks) != "Two" {
		t.Fatalf("playlist = %s", titles(pl.Tracks))
	}

	// back to normal: no release again
	if err := SetSecret(one, false); err != nil {
		t.Fatal(err)
	}
	if tr, _ := GetTrack(Normal, one); tr == nil || len(tr.Releases) != 0 {
		t.Fatalf("back track = %+v", tr)
	}
	if err := SetSecret("nope", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing track = %v", err)
	}
}

func TestHiddenDuplicates(t *testing.T) {
	newTestDB(t)
	seedModes(t)
	fp, _ := FingerprintOf(bytes.NewReader([]byte("s1"))) // the secret track's file
	if exists, _ := TrackExists(Normal, fp); exists {
		t.Fatal("normal mode: TrackExists reveals a secret track")
	}
	if exists, _ := TrackExists(Secret, fp); !exists {
		t.Fatal("secret mode misses its own track")
	}
	fp, _ = FingerprintOf(bytes.NewReader([]byte("n1")))
	if exists, _ := TrackExists(Secret, fp); exists {
		t.Fatal("secret mode: TrackExists reveals a normal track")
	}
	for _, c := range []struct {
		v    Viewer
		body string
		want error
	}{{Normal, "s1", ErrHidden}, {Secret, "n1", ErrHidden}, {Normal, "n1", ErrExists}, {Secret, "s1", ErrExists}} {
		_, err := AddTrack(c.v, nil, &audio.Track{Title: "x"}, bytes.NewReader([]byte(c.body)), ".mp3", nil, "")
		if !errors.Is(err, c.want) {
			t.Errorf("AddTrack(mode %d, %q) = %v, want %v", c.v.Mode, c.body, err, c.want)
		}
	}
}

func TestAddArtistAdoptsHiddenName(t *testing.T) {
	newTestDB(t)
	m := seedModes(t)
	if _, err := AddArtist(Normal, "Normal Artist"); !errors.Is(err, ErrArtistExists) {
		t.Fatalf("visible duplicate = %v", err)
	}
	// a name only the other mode can see doesn't say so
	got, err := AddArtist(Normal, "secret artist")
	if err != nil || got.ID.String() != m.secretArtist {
		t.Fatalf("adopt = %+v, %v", got, err)
	}
}

func TestSecretPlaylistSlots(t *testing.T) {
	newTestDB(t)
	a := addTestTrack(t, "A", "a")
	s := addTestTrack(t, "S", "s")
	b := addTestTrack(t, "B", "b")
	SetSecret(s, true)
	// a playlist from before the modes can mix both sides
	p, _ := CreatePlaylist("p")
	SetPlaylistTracks(All, p.ID.String(), []string{a, s, b})

	pl, _ := GetPlaylist(Normal, p.ID.String())
	if titles(pl.Tracks) != "A,B" {
		t.Fatalf("normal playlist = %s", titles(pl.Tracks))
	}
	if list, _ := ListPlaylists(Normal); list[0].Count != 2 {
		t.Fatalf("count = %d", list[0].Count)
	}
	SetPlaylistTracks(Normal, p.ID.String(), []string{b, a})
	pl, _ = GetPlaylist(All, p.ID.String())
	if titles(pl.Tracks) != "B,S,A" {
		t.Fatalf("after normal save = %s", titles(pl.Tracks))
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func titlesOf(rts []*audio.ReleaseTrack) string {
	var ts []*audio.Track
	for _, rt := range rts {
		ts = append(ts, rt.Track)
	}
	return titles(ts)
}
