package applemusic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLink(t *testing.T) {
	cases := []struct {
		in         string
		kind, id   string
		storefront string
		ok         bool
	}{
		{"https://music.apple.com/pl/playlist/favourite-songs/pl.u-m6UNaGY7xo", "playlist", "pl.u-m6UNaGY7xo", "pl", true},
		{"https://music.apple.com/us/playlist/todays-hits/pl.f4d106fed2bd41149aaacabb233eb5eb?l=en", "playlist", "pl.f4d106fed2bd41149aaacabb233eb5eb", "us", true},
		{"music.apple.com/us/playlist/pl.abc123", "playlist", "pl.abc123", "us", true},
		{"http://www.music.apple.com/gb/song/solar-eclipse/6818209989", "song", "6818209989", "gb", true},
		{"https://music.apple.com/us/song/6818209989", "song", "6818209989", "us", true},
		{"https://music.apple.com/us/album/solar-eclipse/6818209777?i=6818209989", "song", "6818209989", "us", true},
		{"https://music.apple.com/us/album/habibti-fomo/6818209777", "album", "6818209777", "us", true},
		{"https://music.apple.com/us/album/habibti-fomo/6818209777?i=abc", "album", "6818209777", "us", true},
		{"https://music.apple.com/us/artist/drake/271256", "", "", "", false},
		{"https://music.apple.com/us/playlist/x/notapl", "", "", "", false},
		{"https://music.apple.com/", "", "", "", false},
		{"https://itunes.apple.com/us/album/x/1", "", "", "", false},
		{"https://example.com/us/song/x/1", "", "", "", false},
		{"ftp://music.apple.com/us/song/x/1", "", "", "", false},
		{"Drake - Solar Eclipse", "", "", "", false},
	}
	for _, c := range cases {
		kind, id, sf, ok := ParseLink(c.in)
		if kind != c.kind || id != c.id || sf != c.storefront || ok != c.ok {
			t.Errorf("ParseLink(%q) = %q %q %q %v, want %q %q %q %v", c.in, kind, id, sf, ok, c.kind, c.id, c.storefront, c.ok)
		}
	}
}

// fakeApple serves testdata pages by request path (with the query when the
// map has it) and counts requests.
func fakeApple(t *testing.T, pages map[string]string) (*Client, *int) {
	t.Helper()
	n := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*n++
		name, ok := pages[r.URL.Path+"?"+r.URL.RawQuery]
		if !ok {
			name, ok = pages[r.URL.Path]
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Error(err)
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	c := New()
	c.BaseURL = srv.URL
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c, n
}

var pages = map[string]string{
	"/us/playlist/todays-hits/pl.f4d106fed2bd41149aaacabb233eb5eb": "playlist.html",
	"/us/playlist/nothing/pl.empty":                                "empty.html",
	"/us/song/solar-eclipse/6818209989":                            "song.html",
	"/us/album/solar-eclipse/6818209777?i=6818209989":              "album.html",
}

func TestGetPlaylist(t *testing.T) {
	c, _ := fakeApple(t, pages)
	pl, err := c.GetPlaylist(context.Background(), "https://music.apple.com/us/playlist/todays-hits/pl.f4d106fed2bd41149aaacabb233eb5eb")
	if err != nil {
		t.Fatal(err)
	}
	if pl.Name != "Today’s Hits" || pl.TrackCount != 50 || len(pl.Songs) != 3 {
		t.Fatalf("playlist = %+v", pl)
	}
	s := pl.Songs[0]
	if s.ID != "6818209989" || s.Title != "Solar Eclipse" || s.Artist != "Drake & Don Toliver" ||
		s.Album != "HABIBTI (FOMO)" || s.Duration != 218389*time.Millisecond {
		t.Fatalf("first song = %+v", s)
	}
}

func TestGetPlaylistWithoutTracks(t *testing.T) {
	c, _ := fakeApple(t, pages)
	_, err := c.GetPlaylist(context.Background(), "https://music.apple.com/us/playlist/nothing/pl.empty")
	if !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v, want ErrUnreadable", err)
	}
	_, err = c.GetPlaylist(context.Background(), "https://music.apple.com/us/playlist/gone/pl.gone")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetPlaylistRefusesOtherLinks(t *testing.T) {
	c, n := fakeApple(t, pages)
	for _, link := range []string{"https://music.apple.com/us/song/x/1", "https://example.com/us/playlist/x/pl.abc"} {
		if _, err := c.GetPlaylist(context.Background(), link); !errors.Is(err, ErrUnreadable) {
			t.Errorf("GetPlaylist(%q) err = %v", link, err)
		}
	}
	if *n != 0 {
		t.Fatalf("%d requests for links that aren't playlists", *n)
	}
}

func TestGetSong(t *testing.T) {
	c, _ := fakeApple(t, pages)
	want := Song{ID: "6818209989", Title: "Solar Eclipse", Artist: "Drake & Don Toliver", Album: "HABIBTI (FOMO)", Duration: 3*time.Minute + 38*time.Second}
	got, err := c.GetSong(context.Background(), "https://music.apple.com/us/song/solar-eclipse/6818209989")
	if err != nil || got != want {
		t.Fatalf("song page: %+v, %v", got, err)
	}
	// an album page picks the track named by ?i=, with the album's title
	got, err = c.GetSong(context.Background(), "https://music.apple.com/us/album/solar-eclipse/6818209777?i=6818209989")
	want.Duration = 218389 * time.Millisecond
	if err != nil || got != want {
		t.Fatalf("album page: %+v, %v", got, err)
	}
}

func TestGetSongNotOnPage(t *testing.T) {
	c, _ := fakeApple(t, map[string]string{"/us/album/x/1": "album.html"})
	if _, err := c.GetSong(context.Background(), "https://music.apple.com/us/album/x/1?i=999"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v, want ErrUnreadable", err)
	}
}

func TestRequestsAreSpaced(t *testing.T) {
	c, _ := fakeApple(t, pages)
	var slept []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	for range 2 {
		if _, err := c.GetSong(context.Background(), "https://music.apple.com/us/song/solar-eclipse/6818209989"); err != nil {
			t.Fatal(err)
		}
	}
	if len(slept) != 1 || slept[0] <= 0 || slept[0] > time.Second {
		t.Fatalf("sleeps = %v, want one wait of up to 1s", slept)
	}
}
