package api

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"audioloader/internal/db"
)

// keys are the sorted field names of a decoded JSON object.
func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func wantKeys(t *testing.T, what string, m map[string]any, want string) {
	t.Helper()
	if got := strings.Join(keys(m), " "); got != want {
		t.Errorf("%s fields:\n got  %s\n want %s", what, got, want)
	}
}

// items are the "items" of a decoded list, each a JSON object.
func items(t *testing.T, list map[string]any) []map[string]any {
	t.Helper()
	raw, ok := list["items"].([]any)
	if !ok {
		t.Fatalf("no items array in %v", list)
	}
	out := make([]map[string]any, len(raw))
	for i, it := range raw {
		out[i] = it.(map[string]any)
	}
	return out
}

func titles(its []map[string]any, field string) []string {
	out := []string{}
	for _, it := range its {
		out = append(out, it[field].(string))
	}
	return out
}

// The field names fixed here are what the mobile app reads.
func TestV1TrackShape(t *testing.T) {
	s := newV1(t, true)
	ids := modeLibrary(t)
	setCover(t, ids["public release"])

	got := s.json("/api/v1/tracks/" + ids["public track"])
	wantKeys(t, "track", got, "artists chapters description duration finished id imageUrl kind position releases showId streamUrl tags title")
	if got["id"] != ids["public track"] || got["title"] != "ZZPUBLIC Song 1" || got["kind"] != "music" ||
		got["duration"] != 180.0 || got["streamUrl"] != "/api/tracks/"+ids["public track"]+"/stream" ||
		got["position"] != 0.0 || got["finished"] != false || got["description"] != "" {
		t.Errorf("track = %v", got)
	}
	if img, _ := got["imageUrl"].(string); !strings.HasPrefix(img, "/covers/") {
		t.Errorf("imageUrl = %q, want the release cover", img)
	}
	artists := got["artists"].([]any)
	if len(artists) != 1 {
		t.Fatalf("artists = %v", artists)
	}
	wantKeys(t, "artist ref", artists[0].(map[string]any), "id name")
	if a := artists[0].(map[string]any); a["id"] != ids["public artist"] || a["name"] != "ZZPUBLIC Artist" {
		t.Errorf("artist = %v", a)
	}
	releases := got["releases"].([]any)
	if len(releases) == 0 {
		t.Fatal("no releases")
	}
	wantKeys(t, "release", releases[0].(map[string]any), "artists date id imageUrl title type")
	if len(got["tags"].([]any)) != 0 || len(got["chapters"].([]any)) != 0 {
		t.Errorf("tags and chapters should be empty arrays: %v", got)
	}

	// an episode carries its position, and chapters
	ep := s.json("/api/v1/tracks/" + ids["public episode"])
	if ep["kind"] != "episode" || ep["position"] != 120.0 || ep["finished"] != false || ep["showId"] != ids["public show"] {
		t.Errorf("episode = %v", ep)
	}
	ch := ep["chapters"].([]any)
	if len(ch) != 1 {
		t.Fatalf("chapters = %v", ch)
	}
	wantKeys(t, "chapter", ch[0].(map[string]any), "start title")
	if ch[0].(map[string]any)["title"] != "PUBLIC chapter" {
		t.Errorf("chapter = %v", ch[0])
	}

	// secret mode: tags, description, no releases
	if err := db.SetTrackTagsForTest(ids["secret track"], "b-tag", "a-tag"); err != nil {
		t.Fatal(err)
	}
	s.secret = true
	st := s.json("/api/v1/tracks/" + ids["secret track"])
	if tags := st["tags"].([]any); len(tags) != 2 || tags[0] != "a-tag" {
		t.Errorf("tags = %v", st["tags"])
	}
	if len(st["releases"].([]any)) != 0 {
		t.Errorf("a secret track has releases: %v", st["releases"])
	}
}

// setCover gives a release a cover, as uploading one does.
func setCover(t *testing.T, releaseID string) {
	t.Helper()
	if _, err := db.DB.Exec(`UPDATE releases SET img_url = ? WHERE uuid = ?;`, "/covers/"+releaseID+"?v=1", releaseID); err != nil {
		t.Fatal(err)
	}
}

func TestV1TrackList(t *testing.T) {
	s := newV1(t, true)
	ids := modeLibrary(t)

	list := s.json("/api/v1/tracks")
	// ZZPUBLIC Song 1, Song 2, Shared Song, Cover; episodes and secret tracks are not listed
	if list["total"] != 4.0 {
		t.Fatalf("total = %v", list["total"])
	}
	its := items(t, list)
	if len(its) != 4 {
		t.Fatalf("items = %d", len(its))
	}
	wantKeys(t, "list item", its[0], "artists duration finished id imageUrl kind position showId streamUrl title")

	// paging: the total stays, the items are the window
	page := s.json("/api/v1/tracks?limit=2&offset=3")
	if page["total"] != 4.0 || len(items(t, page)) != 1 {
		t.Errorf("limit=2&offset=3 = %v", page)
	}
	if past := s.json("/api/v1/tracks?offset=10"); past["total"] != 4.0 || len(items(t, past)) != 0 {
		t.Errorf("offset past the end = %v", past)
	}

	// filter
	q := s.json("/api/v1/tracks?q=shared")
	if got := titles(items(t, q), "title"); len(got) != 1 || got[0] != "ZZPUBLIC Shared Song" {
		t.Errorf("q=shared = %v", got)
	}
	if got := s.json("/api/v1/tracks?q=zzpublic+song"); got["total"] != 3.0 {
		t.Errorf("q with two words: total %v", got["total"])
	}

	// sort by creator: "Shared Artist" before "ZZPUBLIC Artist"
	by := titles(items(t, s.json("/api/v1/tracks?sort=creator")), "title")
	if by[0] != "ZZPUBLIC Shared Song" {
		t.Errorf("sort=creator = %v", by)
	}

	// secret tracks are 404 in normal mode and the other way round
	for _, hidden := range []string{"secret track", "shared secret track", "secret episode"} {
		if rec := s.get("/api/v1/tracks/" + ids[hidden]); rec.Code != http.StatusNotFound {
			t.Errorf("%s in normal mode = %d", hidden, rec.Code)
		}
	}
	s.secret = true
	for _, hidden := range []string{"public track", "shared public track", "public episode"} {
		if rec := s.get("/api/v1/tracks/" + ids[hidden]); rec.Code != http.StatusNotFound {
			t.Errorf("%s in secret mode = %d", hidden, rec.Code)
		}
	}
	got := titles(items(t, s.json("/api/v1/tracks")), "title")
	if len(got) != 3 || !slices.Contains(got, "ZZSECRET Song") {
		t.Errorf("secret list = %v", got)
	}
	if rec := s.get("/api/v1/tracks/not-a-uuid"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id = %d", rec.Code)
	}
}

// A secret mode viewer never gets a release cover, whatever the track is on.
func TestV1ImageURLInSecretMode(t *testing.T) {
	s := newV1(t, true)
	ids := modeLibrary(t)
	setCover(t, ids["public release"])
	// a track that is on the covered release and was made secret keeps no cover
	if err := db.SetSecret(ids["public track"], true); err != nil {
		t.Fatal(err)
	}
	s.secret = true
	for _, u := range []string{"/api/v1/tracks", "/api/v1/tracks/" + ids["public track"], "/api/v1/search?q=zz"} {
		if rec := s.get(u); strings.Contains(rec.Body.String(), "/covers/") {
			t.Errorf("GET %s in secret mode shows a cover: %s", u, rec.Body.String())
		}
	}
	// the track with no image of its own shows none, rather than the placeholder
	if got := s.json("/api/v1/tracks/" + ids["secret track"]); got["imageUrl"] != "" {
		t.Errorf("imageUrl = %v", got["imageUrl"])
	}
}

func TestV1Search(t *testing.T) {
	s := newV1(t, true)
	modeLibrary(t)

	got := s.json("/api/v1/search?q=zzpublic")
	wantKeys(t, "search", got, "artists playlists releases shows tracks")
	if n := len(got["tracks"].([]any)); n != 4 {
		t.Errorf("tracks = %d, want 4", n)
	}
	if n := len(got["playlists"].([]any)); n != 2 {
		t.Errorf("playlists = %d, want the mix and the legacy one", n)
	}
	for _, k := range []string{"artists", "releases", "shows"} {
		if len(got[k].([]any)) == 0 {
			t.Errorf("no %s in %v", k, got)
		}
	}

	empty := s.json("/api/v1/search?q=")
	for k, v := range empty {
		if arr, ok := v.([]any); !ok || len(arr) != 0 {
			t.Errorf("empty query: %s = %v, want []", k, v)
		}
	}

	s.secret = true
	got = s.json("/api/v1/search?q=zz")
	if body := s.get("/api/v1/search?q=zz").Body.String(); strings.Contains(strings.ToLower(body), "zzpublic") {
		t.Errorf("secret search shows normal things: %s", body)
	}
	if len(got["playlists"].([]any)) != 1 {
		t.Errorf("secret playlists = %v", got["playlists"])
	}
}

func TestV1SavePositionForm(t *testing.T) {
	s := newV1(t, true)
	ids := modeLibrary(t)
	u := "/api/tracks/" + ids["public episode"] + "/position"
	if rec := s.do("POST", u, "application/x-www-form-urlencoded", "seconds=33.5&finished=false"); rec.Code != http.StatusNoContent {
		t.Fatalf("form position = %d", rec.Code)
	}
	if got := s.json("/api/v1/tracks/" + ids["public episode"]); got["position"] != 33.5 {
		t.Errorf("position = %v", got["position"])
	}
	if rec := s.do("POST", u, "application/json", `{"seconds": 40, "finished": true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("JSON position = %d", rec.Code)
	}
	if got := s.json("/api/v1/tracks/" + ids["public episode"]); got["position"] != 40.0 || got["finished"] != true {
		t.Errorf("episode = %v", got)
	}
	if rec := s.do("POST", u, "application/x-www-form-urlencoded", "seconds=x"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad form = %d", rec.Code)
	}
}

func TestV1Autoplay(t *testing.T) {
	s := newV1(t, true)
	ids := modeLibrary(t)
	list := s.json("/api/v1/autoplay?seed=" + ids["public track"] + "&exclude=" + ids["public track 2"] + "&n=10")
	its := items(t, list)
	if len(its) == 0 || list["total"] != float64(len(its)) {
		t.Fatalf("got %v", list)
	}
	wantKeys(t, "suggestion", its[0], "artists duration finished id imageUrl kind position showId streamUrl title")
	got := strings.Join(titles(its, "title"), ",")
	// the same artist first; the seed, the excluded track and everything of the other mode are left out
	if !strings.HasPrefix(got, "ZZPUBLIC Cover") || strings.Contains(got, "Song 1") || strings.Contains(got, "Song 2") || strings.Contains(got, "ZZSECRET") {
		t.Errorf("normal mode got %s", got)
	}

	if n := len(items(t, s.json("/api/v1/autoplay?n=2"))); n != 2 {
		t.Errorf("n=2 gave %d tracks", n)
	}

	s.secret = true
	got = strings.Join(titles(items(t, s.json("/api/v1/autoplay?seed="+ids["secret track"])), "title"), ",")
	if got == "" || strings.Contains(got, "ZZPUBLIC") || strings.Contains(got, "ZZSECRET Song") {
		t.Errorf("secret mode got %s", got)
	}
	// an episode as the seed: nothing
	if its := items(t, s.json("/api/v1/autoplay?seed="+ids["secret episode"])); len(its) != 0 {
		t.Errorf("episode seed gave %v", its)
	}
}
