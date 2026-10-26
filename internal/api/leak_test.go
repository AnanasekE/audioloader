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

// modeLibrary seeds a library with a normal side, whose titles all carry
// "ZZPUBLIC", and a secret side, whose titles carry "ZZSECRET". They are
// tied together as much as the app allows: an artist on both sides, covers
// pointing across, a playlist from before the modes that holds both. It
// returns IDs by name.
func modeLibrary(t *testing.T) map[string]string {
	t.Helper()
	ids := map[string]string{}
	artist := func(v db.Viewer, name string) string {
		a, err := db.AddArtist(v, name)
		if err != nil {
			t.Fatal(err)
		}
		return a.ID.String()
	}
	ids["public artist"] = artist(db.Normal, "ZZPUBLIC Artist")
	ids["secret artist"] = artist(db.Secret, "ZZSECRET Artist")
	ids["shared artist"] = artist(db.Normal, "Shared Artist")
	ids["waiting secret artist"] = artist(db.Secret, "ZZSECRET Waiting")

	add := func(v db.Viewer, title, body, artist string, link *db.ReleaseLink) string {
		res, err := db.AddTrack(v, []string{artist}, &audio.Track{Title: title, Duration: 3 * time.Minute},
			bytes.NewReader([]byte(body)), ".mp3", link, "")
		if err != nil {
			t.Fatal(err)
		}
		return res.TrackID
	}
	album, _ := db.CreateRelease("ZZPUBLIC Album", audio.ReleaseAlbum, "", []string{ids["public artist"]})
	ids["public release"] = album.ID.String()
	ids["public track"] = add(db.Normal, "ZZPUBLIC Song 1", "p1", ids["public artist"], &db.ReleaseLink{ReleaseID: album.ID.String(), Disc: 1, Position: 1})
	ids["public track 2"] = add(db.Normal, "ZZPUBLIC Song 2", "p2", ids["public artist"], &db.ReleaseLink{ReleaseID: album.ID.String(), Disc: 1, Position: 2})
	ids["shared public track"] = add(db.Normal, "ZZPUBLIC Shared Song", "p3", ids["shared artist"], nil)
	ids["secret track"] = add(db.Secret, "ZZSECRET Song", "s1", ids["secret artist"], nil)
	ids["shared secret track"] = add(db.Secret, "ZZSECRET Shared Song", "s2", ids["shared artist"], nil)
	pt, _ := db.GetTrack(db.Normal, ids["public track"])
	for _, r := range pt.Releases {
		if r.IsSingle() {
			ids["public single"] = r.ID.String()
		}
	}
	st, _ := db.GetTrack(db.Normal, ids["shared public track"])
	ids["public single"] = st.Releases[0].ID.String()

	// covers across the line: neither may show its original's title
	ids["public cover"] = add(db.Normal, "ZZPUBLIC Cover", "p4", ids["public artist"], nil)
	ids["secret cover"] = add(db.Secret, "ZZSECRET Cover", "s3", ids["secret artist"], nil)
	for cover, original := range map[string]string{ids["public cover"]: ids["secret track"], ids["secret cover"]: ids["public track"]} {
		if _, err := db.DB.Exec(`UPDATE tracks SET original_track_id = ? WHERE uuid = ?;`, original, cover); err != nil {
			t.Fatal(err)
		}
	}

	// an inbox release, so the inbox pages have rows to render
	if err := db.SetFollow(ids["public artist"], "mb-public", db.DefaultFilters()); err != nil {
		t.Fatal(err)
	}
	if err := db.AddInboxRelease(db.InboxRelease{ReleaseGroupMBID: "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", ArtistID: ids["public artist"],
		Title: "ZZPUBLIC Upcoming", Type: "album", Date: "2030", Data: db.InboxData{Tracks: []db.InboxTrack{{Position: 1, Title: "Next", OwnedTrackID: ids["public track"]}}}}); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		v      db.Viewer
		name   string
		key    string
		kept   string
		keepID string
	}{{db.Normal, "ZZPUBLIC Show", "public", "ZZPUBLIC Episode", "pe"}, {db.Secret, "ZZSECRET Show", "secret", "ZZSECRET Episode", "se"}} {
		show, err := db.ResolveShow(c.v, c.name, "")
		if err != nil {
			t.Fatal(err)
		}
		ids[c.key+" show"] = show.ID.String()
		ep, err := db.AddEpisode(c.v, show.ID.String(), &audio.Track{Title: c.kept, Duration: time.Hour}, bytes.NewReader([]byte(c.keepID)), ".mp3")
		if err != nil {
			t.Fatal(err)
		}
		ids[c.key+" episode"] = ep
		db.SavePosition(ep, 120, false)
		db.SetChapters(ep, []db.Chapter{{Start: 0, Title: strings.ToUpper(c.key) + " chapter"}})
	}

	pp, err := db.CreatePlaylist(db.Normal, "ZZPUBLIC Mix")
	if err != nil {
		t.Fatal(err)
	}
	ids["public playlist"] = pp.ID.String()
	db.SetPlaylistTracks(db.Normal, pp.ID.String(), []string{ids["public track"], ids["public track 2"]})
	sp, err := db.CreatePlaylist(db.Secret, "ZZSECRET Mix")
	if err != nil {
		t.Fatal(err)
	}
	ids["secret playlist"] = sp.ID.String()
	db.SetPlaylistTracks(db.Secret, sp.ID.String(), []string{ids["secret track"], ids["shared secret track"]})
	// from before the modes, when a playlist could hold both: it belongs to
	// normal mode and its secret entries stay out of sight
	legacy, _ := db.CreatePlaylist(db.Normal, "ZZPUBLIC Legacy")
	ids["legacy playlist"] = legacy.ID.String()
	db.SetPlaylistTracks(db.All, legacy.ID.String(), []string{ids["public track"], ids["secret track"]})

	db.SaveMatches(ids["secret track"], []db.Match{{Source: "search", Score: 0.7, RecordingID: "rec", Title: "ZZSECRET"}}, db.TagReview)
	// an applied match crediting the secret artist under a close name feeds the "link artists?" list
	if _, err := db.ApplyMatch(ids["secret track"], db.Match{RecordingID: "rec", Title: "ZZSECRET",
		Artists: []db.MatchArtist{{ID: "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", Name: "ZZSECRET Artis"}}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCoverSuggestion(ids["secret track"], db.CoverSuggestion{Title: "ZZSECRET Original", Artists: "ZZSECRET Band"}); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestNothingLeaksAcrossModes(t *testing.T) {
	dbtest.New(t)
	fakeMusicBrainz(t, map[string]string{"/release-group": `{"release-groups": [{"id": "x", "title": "ZZPUBLIC Album 2", "primary-type": "Album", "first-release-date": "2001"}], "release-group-count": 1}`})
	ids := modeLibrary(t)
	hash, _ := vault.Hash("pass")
	v := vault.New([]byte(hash), []byte("0123456789abcdef0123456789abcdef"))
	mux := http.NewServeMux()
	Register(mux, v)
	h := WithViewer(mux)

	pick := func(keys ...string) []string {
		var out []string
		for _, k := range keys {
			out = append(out, ids[k])
		}
		return out
	}
	// IDs to try for each path parameter: those of both sides, since each
	// side's pages must answer 404 for the other's
	params := map[string][]string{
		"track_id": pick("secret track", "secret episode", "public track", "public episode", "public cover", "secret cover",
			"shared public track", "shared secret track"),
		"release_id":    pick("public single", "public release"),
		"artist_id":     pick("public artist", "secret artist", "shared artist", "waiting secret artist"),
		"playlist_id":   pick("public playlist", "secret playlist", "legacy playlist"),
		"show_id":       pick("public show", "secret show"),
		"recording_id":  {"00000000-0000-0000-0000-000000000000"},
		"release_group": {"10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8", "00000000-0000-0000-0000-000000000000"},
		"mbid":          {"10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"},
	}
	extra := []string{
		"/search?q=zz", "/ui/search?q=zz", "/search?q=secret", "/search?q=public", "/ui/search?q=artist", "/search?q=shared",
		"/inbox?tab=ignored", "/inbox?type=album&owned=none&q=upcoming&sort=oldest",
		"/ui/tracks/pick?q=zz", "/ui/tracks/pick?q=song",
		"/tracks?sort=creator", "/tracks?sort=tag", "/tracks?sort=added",
		"/upload/batch?release=" + ids["public release"],
	}
	for _, k := range []string{"public track", "secret track", "shared secret track"} {
		extra = append(extra, "/ui/playlists/picker?track_id="+ids[k])
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

	modes := []struct {
		name   string
		secret bool
		// hidden: a marker and the IDs of what lives only on the other side
		hidden    string
		hiddenIDs []string
		// shown: what each page must list, so the checks aren't vacuous
		shown map[string][]string
	}{
		{"normal", false, "zzsecret", pick("secret track", "shared secret track", "secret cover", "secret episode", "secret show", "secret playlist", "secret artist", "waiting secret artist"),
			map[string][]string{
				"/tracks":                          {"ZZPUBLIC Song 1", "ZZPUBLIC Shared Song", "ZZPUBLIC Cover"},
				"/artists":                         {"ZZPUBLIC Artist", "Shared Artist"},
				"/releases":                        {"ZZPUBLIC Album"},
				"/shows":                           {"ZZPUBLIC Show"},
				"/playlists":                       {"ZZPUBLIC Mix", "ZZPUBLIC Legacy"},
				"/search?q=zzpublic":               {"ZZPUBLIC Song 1", "ZZPUBLIC Album"},
				"/artists/" + ids["shared artist"]: {"ZZPUBLIC Shared Song"},
			}},
		{"secret", true, "zzpublic", pick("public track", "public track 2", "shared public track", "public cover", "public release", "public single", "public episode", "public show", "public playlist", "legacy playlist", "public artist"),
			map[string][]string{
				"/tracks":                          {"ZZSECRET Song", "ZZSECRET Shared Song", "ZZSECRET Cover"},
				"/artists":                         {"ZZSECRET Artist", "Shared Artist", "ZZSECRET Waiting"},
				"/shows":                           {"ZZSECRET Show"},
				"/playlists":                       {"ZZSECRET Mix"},
				"/search?q=zzsecret":               {"ZZSECRET Song", "ZZSECRET Artist"},
				"/artists/" + ids["shared artist"]: {"ZZSECRET Shared Song"},
			}},
	}
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			get := func(u string) (*httptest.ResponseRecorder, string) {
				req := httptest.NewRequest("GET", u, nil)
				if m.secret {
					req.AddCookie(v.Cookie(time.Now(), false))
				}
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				body := rec.Body.String()
				// search pages echo the query; that's not the library talking
				if q := req.URL.Query().Get("q"); q != "" {
					body = strings.NewReplacer(q, "", "\u201c"+q+"\u201d", "", "q="+q, "").Replace(body)
				}
				return rec, body
			}
			for _, u := range urls {
				if strings.Contains(u, "{") {
					t.Errorf("no test value for a parameter in %s", u)
					continue
				}
				rec, body := get(u)
				if rec.Code >= 500 {
					t.Errorf("GET %s: %d %s", u, rec.Code, body)
				}
				if strings.Contains(strings.ToLower(body), m.hidden) {
					t.Errorf("GET %s shows %s", u, m.hidden)
				}
				for _, id := range m.hiddenIDs {
					if strings.Contains(body, id) {
						t.Errorf("GET %s contains the ID %s of the other side", u, id)
					}
				}
			}
			for u, want := range m.shown {
				_, body := get(u)
				for _, w := range want {
					if !strings.Contains(body, w) {
						t.Errorf("GET %s doesn't list %q", u, w)
					}
				}
			}
		})
	}

	// routes that only exist in normal mode answer 404 in secret mode, for
	// every method, and still work in normal mode (something other than 404
	// for at least the pages that need no data)
	secretReq := func(method, u string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, u, strings.NewReader(""))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(v.Cookie(time.Now(), false))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if len(NormalOnly) == 0 {
		t.Fatal("no normal-only routes registered")
	}
	for _, route := range NormalOnly {
		method, path, _ := strings.Cut(route, " ")
		path = strings.TrimSuffix(path, "{$}")
		for name, values := range params {
			path = strings.ReplaceAll(path, "{"+name+"}", values[0])
		}
		if rec := secretReq(method, path); rec.Code != http.StatusNotFound {
			t.Errorf("%s in secret mode = %d, want 404", route, rec.Code)
		}
	}
	for _, page := range []string{"/releases", "/inbox", "/tagging", "/downloads", "/upload/youtube"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", page, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s in normal mode = %d", page, rec.Code)
		}
	}

	// the duplicate check says no across the line, both ways
	for _, c := range []struct {
		secret bool
		body   string
		want   string
	}{{false, "s1", "[false]"}, {true, "p1", "[false]"}, {false, "p1", "[true]"}, {true, "s1", "[true]"}} {
		fp, _ := db.FingerprintOf(bytes.NewReader([]byte(c.body)))
		body, _ := json.Marshal([]db.Fingerprint{fp})
		req := httptest.NewRequest("POST", "/api/tracks/exists", bytes.NewReader(body))
		if c.secret {
			req.AddCookie(v.Cookie(time.Now(), false))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if strings.TrimSpace(rec.Body.String()) != c.want {
			t.Errorf("exists check of %q (secret mode %v) = %s", c.body, c.secret, rec.Body.String())
		}
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
