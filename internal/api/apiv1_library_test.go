package api

import (
	"net/http"
	"slices"
	"testing"
)

func TestV1Artists(t *testing.T) {
	s := newV1(t, true)
	ids := modeLibrary(t)

	list := s.json("/api/v1/artists")
	wantKeys(t, "artist", items(t, list)[0], "id imageUrl name")
	if got := titles(items(t, list), "name"); !slices.Equal(got, []string{"Shared Artist", "ZZPUBLIC Artist"}) {
		t.Errorf("normal artists = %v", got)
	}
	if list["total"] != 2.0 {
		t.Errorf("total = %v", list["total"])
	}

	d := s.json("/api/v1/artists/" + ids["public artist"])
	wantKeys(t, "artist detail", d, "id imageUrl name releases tracks")
	// the album and the single of the cover; "tracks" holds only the tracks
	// on none of the artist's releases, as on the artist page
	if got := titles(tracksOf(d, "releases"), "title"); !slices.Equal(got, []string{"ZZPUBLIC Album", "ZZPUBLIC Cover"}) {
		t.Errorf("releases = %v", got)
	}
	if tr := d["tracks"].([]any); len(tr) != 0 {
		t.Errorf("tracks = %v", tr)
	}
	shared := s.json("/api/v1/artists/" + ids["shared artist"])
	if got := titles(tracksOf(shared, "releases"), "title"); !slices.Equal(got, []string{"ZZPUBLIC Shared Song"}) {
		t.Errorf("shared artist's releases = %v", got)
	}
	for _, hidden := range []string{"secret artist", "waiting secret artist"} {
		if rec := s.get("/api/v1/artists/" + ids[hidden]); rec.Code != http.StatusNotFound {
			t.Errorf("%s in normal mode = %d", hidden, rec.Code)
		}
	}

	s.secret = true
	if got := titles(items(t, s.json("/api/v1/artists")), "name"); !slices.Equal(got, []string{"Shared Artist", "ZZSECRET Artist", "ZZSECRET Waiting"}) {
		t.Errorf("secret artists = %v", got)
	}
	sd := s.json("/api/v1/artists/" + ids["shared artist"])
	if len(sd["releases"].([]any)) != 0 {
		t.Errorf("secret mode shows releases: %v", sd["releases"])
	}
	if got := titles(tracksOf(sd, "tracks"), "title"); !slices.Equal(got, []string{"ZZSECRET Shared Song"}) {
		t.Errorf("shared artist's secret tracks = %v", got)
	}
	if rec := s.get("/api/v1/artists/" + ids["public artist"]); rec.Code != http.StatusNotFound {
		t.Errorf("public artist in secret mode = %d", rec.Code)
	}
}

// tracksOf is the array of track objects under key.
func tracksOf(m map[string]any, key string) []map[string]any {
	var out []map[string]any
	for _, it := range m[key].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func TestV1Releases(t *testing.T) {
	s := newV1(t, true)
	ids := modeLibrary(t)

	list := s.json("/api/v1/releases")
	wantKeys(t, "release", items(t, list)[0], "artists date id imageUrl title type")
	if got := titles(items(t, list), "title"); !slices.Contains(got, "ZZPUBLIC Album") {
		t.Errorf("releases = %v", got)
	}

	d := s.json("/api/v1/releases/" + ids["public release"])
	wantKeys(t, "release detail", d, "artists date duration id imageUrl title tracks type")
	if d["type"] != "album" || d["duration"] != 360.0 {
		t.Errorf("release = %v", d)
	}
	tracks := tracksOf(d, "tracks")
	if len(tracks) != 2 {
		t.Fatalf("tracks = %v", tracks)
	}
	wantKeys(t, "release track", tracks[0], "disc number track")
	if tracks[0]["number"] != 1.0 || tracks[1]["number"] != 2.0 || tracks[0]["disc"] != 1.0 {
		t.Errorf("order = %v", tracks)
	}
	if first := tracks[0]["track"].(map[string]any); first["title"] != "ZZPUBLIC Song 1" ||
		first["streamUrl"] != "/api/tracks/"+ids["public track"]+"/stream" {
		t.Errorf("first track = %v", first)
	}

	// releases don't exist in secret mode
	s.secret = true
	for _, u := range []string{"/api/v1/releases", "/api/v1/releases/" + ids["public release"]} {
		if rec := s.get(u); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s in secret mode = %d", u, rec.Code)
		}
	}
}

func TestV1Playlists(t *testing.T) {
	s := newV1(t, true)
	ids := modeLibrary(t)

	list := s.json("/api/v1/playlists")
	wantKeys(t, "playlist", items(t, list)[0], "count duration id imageUrl name")
	if got := titles(items(t, list), "name"); !slices.Equal(got, []string{"ZZPUBLIC Legacy", "ZZPUBLIC Mix"}) && !slices.Equal(got, []string{"ZZPUBLIC Mix", "ZZPUBLIC Legacy"}) {
		t.Errorf("playlists = %v", got)
	}

	d := s.json("/api/v1/playlists/" + ids["public playlist"])
	wantKeys(t, "playlist detail", d, "count duration id imageUrl name tracks")
	if got := titles(tracksOf(d, "tracks"), "title"); !slices.Equal(got, []string{"ZZPUBLIC Song 1", "ZZPUBLIC Song 2"}) {
		t.Errorf("tracks = %v", got)
	}
	// from before the modes: holds a secret track too, which stays out
	legacy := s.json("/api/v1/playlists/" + ids["legacy playlist"])
	if got := titles(tracksOf(legacy, "tracks"), "title"); !slices.Equal(got, []string{"ZZPUBLIC Song 1"}) || legacy["count"] != 1.0 {
		t.Errorf("legacy playlist = %v", legacy)
	}
	if rec := s.get("/api/v1/playlists/" + ids["secret playlist"]); rec.Code != http.StatusNotFound {
		t.Errorf("secret playlist in normal mode = %d", rec.Code)
	}

	s.secret = true
	if got := titles(items(t, s.json("/api/v1/playlists")), "name"); !slices.Equal(got, []string{"ZZSECRET Mix"}) {
		t.Errorf("secret playlists = %v", got)
	}
	sd := s.json("/api/v1/playlists/" + ids["secret playlist"])
	if got := titles(tracksOf(sd, "tracks"), "title"); !slices.Equal(got, []string{"ZZSECRET Song", "ZZSECRET Shared Song"}) {
		t.Errorf("secret playlist tracks = %v", got)
	}
	for _, hidden := range []string{"public playlist", "legacy playlist"} {
		if rec := s.get("/api/v1/playlists/" + ids[hidden]); rec.Code != http.StatusNotFound {
			t.Errorf("%s in secret mode = %d", hidden, rec.Code)
		}
	}
}

func TestV1Shows(t *testing.T) {
	s := newV1(t, true)
	ids := modeLibrary(t)

	list := s.json("/api/v1/shows")
	wantKeys(t, "show", items(t, list)[0], "author count id imageUrl title unplayed")
	if got := titles(items(t, list), "title"); !slices.Equal(got, []string{"ZZPUBLIC Show"}) {
		t.Errorf("shows = %v", got)
	}

	d := s.json("/api/v1/shows/" + ids["public show"])
	wantKeys(t, "show detail", d, "author count episodes id imageUrl title unplayed")
	eps := tracksOf(d, "episodes")
	if len(eps) != 1 || eps[0]["title"] != "ZZPUBLIC Episode" || eps[0]["position"] != 120.0 || eps[0]["kind"] != "episode" {
		t.Errorf("episodes = %v", eps)
	}
	if rec := s.get("/api/v1/shows/" + ids["secret show"]); rec.Code != http.StatusNotFound {
		t.Errorf("secret show in normal mode = %d", rec.Code)
	}

	s.secret = true
	if got := titles(items(t, s.json("/api/v1/shows")), "title"); !slices.Equal(got, []string{"ZZSECRET Show"}) {
		t.Errorf("secret shows = %v", got)
	}
	sd := s.json("/api/v1/shows/" + ids["secret show"])
	if got := titles(tracksOf(sd, "episodes"), "title"); !slices.Equal(got, []string{"ZZSECRET Episode"}) {
		t.Errorf("secret episodes = %v", got)
	}
	if rec := s.get("/api/v1/shows/" + ids["public show"]); rec.Code != http.StatusNotFound {
		t.Errorf("public show in secret mode = %d", rec.Code)
	}
}
