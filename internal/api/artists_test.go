package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
)

func TestCandidateReleases(t *testing.T) {
	fakeMusicBrainz(t, map[string]string{"/release-group": `{"release-groups": [
		{"id": "a", "title": "Old Album", "primary-type": "Album", "first-release-date": "1991"},
		{"id": "b", "title": "New Single", "primary-type": "Single", "first-release-date": "2020-05-01"}], "release-group-count": 2}`})
	mux := http.NewServeMux()
	Register(mux, nil)

	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		WithViewer(mux).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code, rec.Body.String()
	}
	code, body := get("/ui/musicbrainz/artists/10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8/releases")
	if code != 200 || strings.Index(body, "New Single") > strings.Index(body, "Old Album") || !strings.Contains(body, "Single, 2020-05-01") {
		t.Fatalf("%d %s", code, body)
	}
	if code, _ := get("/ui/musicbrainz/artists/not-a-uuid/releases"); code != http.StatusBadRequest {
		t.Fatalf("bad id = %d", code)
	}
	// an unreachable MusicBrainz shows a message, not an error page
	fakeMusicBrainz(t, nil)
	if code, body := get("/ui/musicbrainz/artists/10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8/releases"); code != 200 || !strings.Contains(body, "Couldn't load") {
		t.Fatalf("%d %s", code, body)
	}
}

func TestParseArtistMBID(t *testing.T) {
	const id = "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"
	for in, want := range map[string]string{
		id:                                     id,
		" " + strings.ToUpper(id) + " ":        id,
		"https://musicbrainz.org/artist/" + id: id,
		"musicbrainz.org/artist/" + id + "/releases":        id,
		"https://beta.musicbrainz.org/artist/" + id:         id,
		"https://musicbrainz.org/release/" + id:             "",
		"https://evil.example/artist/" + id:                 "",
		"https://musicbrainz.org.evil.example/artist/" + id: "",
		"https://musicbrainz.org/artist/not-an-id":          "",
		"hello": "",
		"":      "",
	} {
		if got := parseArtistMBID(in); got != want {
			t.Errorf("parseArtistMBID(%q) = %q, want %q", in, got, want)
		}
	}
}

func postForm(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLinkArtist(t *testing.T) {
	dbtest.New(t)
	fakeMusicBrainz(t, map[string]string{"/artist": `{"artists": [
		{"id": "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", "name": "Massive Attack", "type": "Group", "country": "GB",
		 "area": {"name": "United Kingdom"}, "life-span": {"begin": "1987", "ended": null}}]}`})
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)

	a, _ := db.AddArtist(db.All, "Massive Attack")
	b, _ := db.AddArtist(db.All, "Massive Attak")
	const mbid = "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/ui/artists/"+a.ID.String()+"/link", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "United Kingdom") || !strings.Contains(rec.Body.String(), "1987–") {
		t.Fatalf("candidates = %d %s", rec.Code, rec.Body.String())
	}

	if rec := postForm(h, "/ui/artists/"+a.ID.String()+"/link", "mbid_manual=https://musicbrainz.org/release/"+mbid); rec.Code != http.StatusBadRequest {
		t.Fatalf("release URL = %d", rec.Code)
	}
	if rec := postForm(h, "/ui/artists/"+a.ID.String()+"/link", "mbid="+mbid); rec.Code != http.StatusNoContent {
		t.Fatalf("link = %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := db.GetArtist(db.All, a.ID.String()); got.MBID != mbid {
		t.Fatalf("mbid = %q", got.MBID)
	}
	// linked artists can't be linked again
	if rec := postForm(h, "/ui/artists/"+a.ID.String()+"/link", "mbid="+mbid); rec.Code != http.StatusConflict {
		t.Fatalf("relink = %d", rec.Code)
	}

	// the same ID on another artist offers a merge and sets nothing
	rec = postForm(h, "/ui/artists/"+b.ID.String()+"/link", "mbid_manual=musicbrainz.org/artist/"+mbid)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Merge into Massive Attack") {
		t.Fatalf("taken = %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := db.GetArtist(db.All, b.ID.String()); got.MBID != "" {
		t.Fatalf("duplicate mbid set: %q", got.MBID)
	}
}

func TestAddArtistFromMusicBrainz(t *testing.T) {
	dbtest.New(t)
	const (
		ma = "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"
		po = "8f6bd1e4-fbe1-4f50-aa9b-94c450ec0f11"
		tr = "3fef8d1f-2a9a-4b1f-9f9b-0e6a8d5d8a11"
	)
	fakeMusicBrainz(t, map[string]string{"/artist": `{"artists": [
		{"id": "` + ma + `", "name": "Massive Attack", "type": "Group", "country": "GB"},
		{"id": "` + po + `", "name": "Portishead", "type": "Group", "country": "GB"},
		{"id": "` + tr + `", "name": "Tricky", "disambiguation": "UK rapper", "type": "Person", "country": "GB"}]}`})
	mux := http.NewServeMux()
	Register(mux, nil)
	h := WithViewer(mux)

	db.AddArtist(db.All, "portishead")
	tricky, _ := db.AddArtist(db.All, "Tricky")
	db.SetArtistMBID(tricky.ID.String(), "11111111-1111-1111-1111-111111111111")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/ui/artists/musicbrainz?q=Massive", nil))
	body := rec.Body.String()
	for _, want := range []string{">Add<", "Link portishead", "Add as Tricky (UK rapper)", "/ui/musicbrainz/artists/" + ma + "/releases"} {
		if rec.Code != 200 || !strings.Contains(body, want) {
			t.Fatalf("search = %d, lacks %q: %s", rec.Code, want, body)
		}
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/ui/artists/musicbrainz?q=", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty q = %d", rec.Code)
	}

	count := func(name string) int {
		n := 0
		list, _ := db.ListArtists(db.All)
		for _, a := range list {
			if a.Name == name {
				n++
			}
		}
		return n
	}
	rec = postForm(h, "/ui/artists/musicbrainz", "mbid="+ma+"&name=Massive Attack")
	a, err := db.ArtistByMBID(db.All, ma)
	if err != nil {
		t.Fatalf("add = %d %s, err %v", rec.Code, rec.Body.String(), err)
	}
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("HX-Trigger"), "artistsChanged") || !strings.Contains(rec.Body.String(), "/artists/"+a.ID.String()+"/follow") {
		t.Fatalf("add = %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	rec = postForm(h, "/ui/artists/musicbrainz", "mbid="+ma+"&name=Massive Attack")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "already in your library") || count("Massive Attack") != 1 {
		t.Fatalf("again = %d %s", rec.Code, rec.Body.String())
	}

	before, _ := db.ListArtists(db.All)
	if rec := postForm(h, "/ui/artists/musicbrainz", "mbid="+po+"&name=Portishead"); rec.Code != 200 {
		t.Fatalf("link = %d %s", rec.Code, rec.Body.String())
	}
	after, _ := db.ListArtists(db.All)
	if p, err := db.ArtistByMBID(db.All, po); err != nil || p.Name != "portishead" || len(after) != len(before) {
		t.Fatalf("link: %+v, %v, %d -> %d artists", p, err, len(before), len(after))
	}

	if rec := postForm(h, "/ui/artists/musicbrainz", "mbid="+tr+"&name=Tricky"); rec.Code != http.StatusConflict {
		t.Fatalf("taken = %d", rec.Code)
	}
	if rec := postForm(h, "/ui/artists/musicbrainz", "mbid="+tr+"&name=Tricky (UK rapper)"); rec.Code != 200 || count("Tricky (UK rapper)") != 1 {
		t.Fatalf("disambiguated = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postForm(h, "/ui/artists/musicbrainz", "mbid=nope&name=X"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad mbid = %d", rec.Code)
	}
}
