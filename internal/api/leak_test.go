package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"audioloader/internal/audio"
	"audioloader/internal/db"
	"audioloader/internal/dbtest"
	"audioloader/internal/vault"
)

// secretLibrary seeds a library around one secret track whose title is
// "ZZSECRET": on a release with public tracks, by an artist who has nothing
// else, in a playlist, and a secret episode of an otherwise empty show. It
// returns IDs by name.
func secretLibrary(t *testing.T) map[string]string {
	t.Helper()
	ids := map[string]string{}
	pub, _ := db.AddArtist(db.All, "Public Artist")
	hid, _ := db.AddArtist(db.All, "ZZSECRET Artist")
	ids["public artist"], ids["hidden artist"] = pub.ID.String(), hid.ID.String()

	add := func(title, body string, artist string, link *db.ReleaseLink) string {
		res, err := db.AddTrack(db.All, []string{artist}, &audio.Track{Title: title, Duration: 3 * time.Minute},
			bytes.NewReader([]byte(body)), ".mp3", link, "")
		if err != nil {
			t.Fatal(err)
		}
		return res.TrackID
	}
	mixed, _ := db.CreateRelease("Mixed", audio.ReleaseAlbum, "", []string{pub.ID.String()})
	ids["mixed release"] = mixed.ID.String()
	ids["public track"] = add("Open Song", "open", pub.ID.String(), &db.ReleaseLink{ReleaseID: mixed.ID.String(), Disc: 1, Position: 1})
	ids["secret track"] = add("ZZSECRET", "secret", hid.ID.String(), nil) // also its own single
	add("Closing Song", "closing", pub.ID.String(), &db.ReleaseLink{ReleaseID: mixed.ID.String(), Disc: 1, Position: 3})
	if err := db.PlaceTrack(ids["secret track"], db.ReleaseLink{ReleaseID: mixed.ID.String(), Disc: 1, Position: 2}); err != nil {
		t.Fatal(err)
	}
	secret, _ := db.GetTrack(db.All, ids["secret track"])
	for _, r := range secret.Releases {
		if r.IsSingle() {
			ids["secret single"] = r.ID.String()
		}
	}

	// a public cover of the secret track: its "Cover of" line must not show up
	ids["cover of secret"] = add("Public Cover", "cover", pub.ID.String(), nil)
	if _, err := db.DB.Exec(`UPDATE tracks SET original_track_id = ? WHERE uuid = ?;`, ids["secret track"], ids["cover of secret"]); err != nil {
		t.Fatal(err)
	}

	// an inbox release, so the inbox pages have rows to render
	if err := db.SetFollow(pub.ID.String(), "mb-public", db.DefaultFilters()); err != nil {
		t.Fatal(err)
	}
	if err := db.AddInboxRelease(db.InboxRelease{ReleaseGroupMBID: "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", ArtistID: pub.ID.String(),
		Title: "Upcoming", Type: "album", Date: "2030", Data: db.InboxData{Tracks: []db.InboxTrack{{Position: 1, Title: "Next", OwnedTrackID: ids["public track"]}}}}); err != nil {
		t.Fatal(err)
	}

	show, _ := db.ResolveShow(db.All, "ZZSECRET Show", "")
	ids["secret show"] = show.ID.String()
	ep, err := db.AddEpisode(db.All, show.ID.String(), &audio.Track{Title: "ZZSECRET Episode", Duration: time.Hour}, bytes.NewReader([]byte("episode")), ".mp3")
	if err != nil {
		t.Fatal(err)
	}
	ids["secret episode"] = ep
	db.SavePosition(ep, 120, false)
	db.SetChapters(ep, []db.Chapter{{Start: 0, Title: "ZZSECRET chapter"}})

	p, _ := db.CreatePlaylist("Mix")
	ids["playlist"] = p.ID.String()
	db.SetPlaylistTracks(db.All, p.ID.String(), []string{ids["public track"], ids["secret track"]})
	db.SaveMatches(ids["secret track"], []db.Match{{Source: "search", Score: 0.7, RecordingID: "rec", Title: "ZZSECRET"}}, db.TagReview)
	// an applied match crediting the hidden artist under a close name feeds the "link artists?" list
	if _, err := db.ApplyMatch(ids["secret track"], db.Match{RecordingID: "rec", Title: "ZZSECRET",
		Artists: []db.MatchArtist{{ID: "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", Name: "ZZSECRET Artis"}}}); err != nil {
		t.Fatal(err)
	}

	if err := db.SaveCoverSuggestion(ids["secret track"], db.CoverSuggestion{Title: "ZZSECRET Original", Artists: "ZZSECRET Band"}); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{ids["secret track"], ep} {
		if err := db.SetSecret(id, true); err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

func TestNothingLeaksWhileLocked(t *testing.T) {
	dbtest.New(t)
	fakeMusicBrainz(t, map[string]string{"/release-group": `{"release-groups": [{"id": "x", "title": "Public Album", "primary-type": "Album", "first-release-date": "2001"}], "release-group-count": 1}`})
	ids := secretLibrary(t)
	hash, _ := vault.Hash("pass")
	v := vault.New([]byte(hash), []byte("0123456789abcdef0123456789abcdef"))
	mux := http.NewServeMux()
	Register(mux, v)
	h := WithViewer(mux)

	secretIDs := []string{ids["secret track"], ids["secret single"], ids["hidden artist"], ids["secret show"], ids["secret episode"]}
	// IDs to try for each path parameter: the hidden ones, and pages that
	// show public things next to hidden ones
	params := map[string][]string{
		"track_id":      {ids["secret track"], ids["secret episode"], ids["public track"], ids["cover of secret"]},
		"release_id":    {ids["secret single"], ids["mixed release"]},
		"artist_id":     {ids["hidden artist"]},
		"playlist_id":   {ids["playlist"]},
		"show_id":       {ids["secret show"]},
		"recording_id":  {"00000000-0000-0000-0000-000000000000"},
		"release_group": {"10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", "00000000-0000-0000-0000-000000000000"},
		"mbid":          {"10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"},
	}
	extra := []string{
		"/search?q=zzsecret", "/ui/search?q=zzsecret", "/search?q=secret", "/ui/search?q=artist",
		"/inbox?tab=ignored", "/inbox?type=album&owned=none&q=upcoming&sort=oldest",
		"/ui/tracks/pick?q=zzsecret", "/ui/tracks/pick?q=secret",
		"/ui/playlists/picker?track_id=" + ids["secret track"],
		"/upload/batch?release=" + ids["mixed release"],
	}

	var urls []string
	for _, route := range Routes {
		method, path, _ := strings.Cut(route, " ")
		if method != "GET" {
			continue
		}
		path = strings.TrimSuffix(path, "{$}")
		expanded := []string{path}
		for name, values := range params {
			if !strings.Contains(path, "{"+name+"}") {
				continue
			}
			var next []string
			for _, u := range expanded {
				for _, val := range values {
					next = append(next, strings.ReplaceAll(u, "{"+name+"}", val))
				}
			}
			expanded = next
		}
		urls = append(urls, expanded...)
	}
	urls = append(urls, extra...)

	for _, u := range urls {
		if strings.Contains(u, "{") {
			t.Errorf("no test value for a parameter in %s", u)
			continue
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", u, nil)
		h.ServeHTTP(rec, req)
		body := rec.Body.String()
		// search pages echo the query; that's not the library talking
		if q := req.URL.Query().Get("q"); q != "" {
			body = strings.NewReplacer(q, "", "“"+q+"”", "", "q="+q, "").Replace(body)
		}
		if rec.Code >= 500 {
			t.Errorf("GET %s: %d %s", u, rec.Code, body)
		}
		if strings.Contains(strings.ToLower(body), "zzsecret") {
			t.Errorf("GET %s shows the secret title", u)
		}
		for _, id := range secretIDs {
			if strings.Contains(body, id) {
				t.Errorf("GET %s contains secret id %s", u, id)
			}
		}
	}

	// the cover itself is listed, without its original
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/tracks", nil))
	if !strings.Contains(rec.Body.String(), "Public Cover") || strings.Contains(rec.Body.String(), "Cover of") {
		t.Error("tracks page should list the cover without a Cover of line")
	}

	// the mixed release numbers its public tracks 1, 2
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/releases/"+ids["mixed release"], nil))
	if strings.Contains(rec.Body.String(), ">3<") {
		t.Error("mixed release shows a gap in its numbering")
	}

	// the duplicate check says no
	fp, _ := db.FingerprintOf(bytes.NewReader([]byte("secret")))
	body, _ := json.Marshal([]db.Fingerprint{fp})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/tracks/exists", bytes.NewReader(body)))
	if strings.TrimSpace(rec.Body.String()) != "[false]" {
		t.Errorf("exists check = %s", rec.Body.String())
	}

	// unlocked, it's all there
	unlocked := httptest.NewRequest("GET", "/tracks", nil)
	unlocked.AddCookie(v.Cookie(time.Now(), false))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, unlocked)
	if !strings.Contains(rec.Body.String(), "ZZSECRET") {
		t.Error("unlocked tracks page misses the secret track")
	}
}

func TestVaultRoutes(t *testing.T) {
	dbtest.New(t)
	mux := http.NewServeMux()
	Register(mux, nil)
	rec := httptest.NewRecorder()
	WithViewer(mux).ServeHTTP(rec, httptest.NewRequest("GET", "/vault", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("vault without a secret = %d", rec.Code)
	}

	hash, _ := vault.Hash("pass")
	mux = http.NewServeMux()
	Register(mux, vault.New([]byte(hash), []byte("0123456789abcdef0123456789abcdef")))
	h := WithViewer(mux)
	post := func(pass string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/vault", strings.NewReader("passphrase="+pass))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := post("pass"); rec.Code != http.StatusSeeOther || len(rec.Result().Cookies()) != 1 {
		t.Fatalf("unlock = %d, cookies %v", rec.Code, rec.Result().Cookies())
	}
	if rec := post("wrong"); len(rec.Result().Cookies()) != 0 || !strings.Contains(rec.Header().Get("Location"), "wrong") {
		t.Fatalf("wrong passphrase = %d, %v", rec.Code, rec.Result().Cookies())
	}
}
