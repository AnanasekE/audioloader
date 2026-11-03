package api

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"audioloader/internal/db"
	"audioloader/internal/sources/secret"
	"audioloader/internal/sources/secret/secrettest"
)

func TestClassifyAddInput(t *testing.T) {
	_, src := secrettest.New(t)
	secret.SetDefault(src)
	t.Cleanup(func() { secret.SetDefault(nil) })

	const rel = "9a025dc4-fdb2-4b7c-a197-d0fd58216e21"
	secretLink := "https://" + secrettest.PageHost + "/items/someone/one"
	cases := []struct {
		name   string
		v      db.Viewer
		in     string
		target addTarget
		value  string // "" to skip
		msg    string
	}{
		{"mb release", db.Normal, "https://musicbrainz.org/release/" + rel, addMusicBrainz, "https://musicbrainz.org/release/" + rel, ""},
		{"mb recording, upper host", db.Normal, "HTTPS://MusicBrainz.ORG/recording/" + rel, addMusicBrainz, "", ""},
		{"mb www", db.Normal, "https://www.musicbrainz.org/release/" + rel, addMusicBrainz, "", ""},
		{"mb no scheme", db.Normal, "musicbrainz.org/release/" + rel, addMusicBrainz, "https://musicbrainz.org/release/" + rel, ""},
		{"mb junk after", db.Normal, "  https://musicbrainz.org/release/" + rel + "/?x=1#top \n", addMusicBrainz, "https://musicbrainz.org/release/" + rel + "/?x=1#top", ""},
		{"mb other page", db.Normal, "https://musicbrainz.org/artist/" + rel, addMusicBrainz, "", ""},
		{"youtube watch", db.Normal, "https://www.youtube.com/watch?v=aqz-KE-bpKQ", addYouTube, "", ""},
		{"youtube mobile", db.Normal, "https://m.youtube.com/watch?v=aqz-KE-bpKQ", addYouTube, "", ""},
		{"youtube upper host", db.Normal, "https://WWW.YOUTUBE.COM/watch?v=aqz-KE-bpKQ&t=3s", addYouTube, "", ""},
		{"youtu.be", db.Normal, "https://youtu.be/aqz-KE-bpKQ?si=abc", addYouTube, "", ""},
		{"youtu.be no scheme", db.Normal, "youtu.be/aqz-KE-bpKQ", addYouTube, "https://youtu.be/aqz-KE-bpKQ", ""},
		{"youtube playlist", db.Normal, "https://youtube.com/playlist?list=PLabcdefghijk", addYouTube, "", ""},
		{"youtube music", db.Normal, "https://music.youtube.com/watch?v=aqz-KE-bpKQ&list=PLabcdefghijk", addYouTube, "", ""},
		{"apple music song", db.Normal, "https://music.apple.com/pl/song/x/1001", addAppleMusic, "", ""},
		{"apple music playlist, www", db.Normal, "https://www.music.apple.com/us/playlist/x/pl.abc", addAppleMusic, "", ""},
		{"apple music no scheme", db.Normal, "music.apple.com/us/album/x/1?i=2", addAppleMusic, "https://music.apple.com/us/album/x/1?i=2", ""},
		{"apple music lookalike", db.Normal, "https://music.apple.com.evil.test/us/song/x/1", addRefused, "", msgBadSite},
		{"youtube channel", db.Normal, "https://www.youtube.com/@someone", addRefused, "", msgBadSite},
		{"other site", db.Normal, "https://example.com/song", addRefused, "", msgBadSite},
		{"lookalike host", db.Normal, "https://musicbrainz.org.evil.test/release/" + rel, addRefused, "", msgBadSite},
		{"secret link in normal mode", db.Normal, secretLink, addRefused, "", msgBadSite},
		{"secret link, www, in normal mode", db.Normal, "www." + secrettest.PageHost + "/items/someone/one/", addRefused, "", msgBadSite},
		{"text", db.Normal, "  Ella Red - Muse ", addSearch, "Ella Red - Muse", ""},
		{"text with slash", db.Normal, "AC/DC", addSearch, "AC/DC", ""},
		{"text with dot", db.Normal, "St.Vincent", addSearch, "St.Vincent", ""},
		{"text that names a site", db.Normal, "youtube.com is great", addSearch, "", ""},

		{"secret link", db.Secret, secretLink, addSecretSource, secretLink, ""},
		{"secret link, upper host, www", db.Secret, "https://WWW." + strings.ToUpper(secrettest.PageHost) + "/items/someone/one/?utm=1", addSecretSource, "", ""},
		{"secret link no scheme", db.Secret, secrettest.PageHost + "/items/someone/one", addSecretSource, "", ""},
		{"secret mode, other site", db.Secret, "https://example.com/song", addRefused, "", msgBadSite},
		{"secret mode, youtube", db.Secret, "https://youtu.be/aqz-KE-bpKQ", addRefused, "", msgBadSite},
		{"secret mode, apple music", db.Secret, "https://music.apple.com/us/song/x/1", addRefused, "", msgBadSite},
		{"secret mode, mb", db.Secret, "https://musicbrainz.org/release/" + rel, addRefused, "", msgBadSite},
		{"secret mode, text", db.Secret, "some words", addRefused, "", msgSecretNoText},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyAddInput(c.v, c.in)
			if got.Target != c.target || got.Message != c.msg || c.value != "" && got.Value != c.value {
				t.Fatalf("classify(%q) = %+v; want target %d, value %q, message %q", c.in, got, c.target, c.value, c.msg)
			}
		})
	}

	// no source configured: text and links both say so, without naming one
	secret.SetDefault(nil)
	if got := classifyAddInput(db.Secret, "words"); got.Message != msgSecretNoLinks {
		t.Errorf("text without a source = %+v", got)
	}
	if got := classifyAddInput(db.Secret, secretLink); got.Target != addRefused || got.Message != msgBadSite {
		t.Errorf("link without a source = %+v", got)
	}
}

func TestAddLookup(t *testing.T) {
	a := newModeApp(t)
	fake, src := secrettest.New(t)
	secret.SetDefault(src)
	t.Cleanup(func() { secret.SetDefault(nil) })
	pageURL := fake.Page("/items/someone/one", `<html><head><title>Fallback</title></head><body><div class="jp-title">The Title</div>
		<script>$("#p").jPlayer("setMedia", { m4a: "https:\/\/media.example.test\/sounds\/one.m4a" })</script></body></html>`)
	fakeSongMusicBrainz(t)
	mezzanineRelease := mustRead(t, "../mb/testdata/release.json")
	fakeMusicBrainz(t, map[string]string{
		"/recording": mustRead(t, "../mb/testdata/recording_search.json"),
		"/release/":  mezzanineRelease,
	})
	lookup := func(secretMode bool, q string) (int, string) {
		rec := a.do(secretMode, "POST", "/ui/add/lookup", url.Values{"q": {q}})
		return rec.Code, rec.Body.String()
	}

	// a MusicBrainz release link lists its tracks, like the old route
	code, body := lookup(false, "https://musicbrainz.org/release/"+mezzanine)
	if code != 200 || !strings.Contains(body, "Add all (11)") || !strings.Contains(body, "Teardrop") {
		t.Fatalf("release = %d\n%s", code, body)
	}
	direct := a.do(false, "GET", "/ui/add/musicbrainz?q="+url.QueryEscape("https://musicbrainz.org/release/"+mezzanine), nil).Body.String()
	if body != direct {
		t.Error("the lookup answers differently from the MusicBrainz route")
	}
	// other MusicBrainz links keep the existing message
	if code, body := lookup(false, "https://musicbrainz.org/artist/"+mezzanine); code != 400 || !strings.Contains(body, "Paste a release or recording link.") {
		t.Fatalf("artist link = %d %s", code, body)
	}
	// text searches MusicBrainz
	if code, body := lookup(false, "Massive Attack Teardrop"); code != 200 || !strings.Contains(body, "Spirits of Nature 5") {
		t.Fatalf("search = %d\n%s", code, body)
	}
	// unknown sites, and the secret source in normal mode, get the same answer
	for _, q := range []string{"https://example.com/x", pageURL} {
		if code, body := lookup(false, q); code != 400 || strings.TrimSpace(body) != msgBadSite {
			t.Fatalf("%s = %d %q", q, code, body)
		}
	}
	if got := len(fake.Requests()); got != 0 {
		t.Errorf("the secret source was asked in normal mode: %v", fake.Requests())
	}
	if code, _ := lookup(false, ""); code != 400 {
		t.Errorf("empty = %d", code)
	}

	// a YouTube link goes to the import preview
	yt := "https://www.youtube.com/watch?v=aqz-KE-bpKQ"
	ytCode, ytBody := lookup(false, yt)
	direct2 := a.do(false, "POST", "/ui/youtube/preview", url.Values{"url": {yt}})
	if ytCode != direct2.Code || ytBody != direct2.Body.String() {
		t.Errorf("youtube = %d %q; the import route says %d %q", ytCode, ytBody, direct2.Code, direct2.Body)
	}

	// secret mode: only secret source links
	code, body = lookup(true, pageURL)
	if code != 200 || !strings.Contains(body, `value="The Title"`) {
		t.Fatalf("secret link = %d\n%s", code, body)
	}
	if code, body := lookup(true, "some words"); code != 400 || strings.TrimSpace(body) != msgSecretNoText {
		t.Fatalf("secret text = %d %q", code, body)
	}
	for _, q := range []string{"https://example.com/x", yt, "https://musicbrainz.org/release/" + mezzanine} {
		if code, body := lookup(true, q); code != 400 || strings.TrimSpace(body) != msgBadSite {
			t.Fatalf("secret mode %s = %d %q", q, code, body)
		}
	}
}

func TestAddPageAndRedirects(t *testing.T) {
	a := newModeApp(t)
	_, src := secrettest.New(t)
	secret.SetDefault(src)
	t.Cleanup(func() { secret.SetDefault(nil) })
	redirect := func(secretMode bool, from, to string) {
		t.Helper()
		rec := a.do(secretMode, "GET", from, nil)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != to {
			t.Errorf("GET %s = %d, Location %q; want %q", from, rec.Code, rec.Header().Get("Location"), to)
		}
	}
	redirect(false, "/upload", "/add")
	redirect(false, "/upload?q=Teardrop", "/add?q=Teardrop")
	redirect(true, "/upload?q=a%20b", "/add?q=a%20b")
	redirect(false, "/upload/youtube", "/add")
	redirect(false, "/upload/youtube?url=https%3A%2F%2Fyoutu.be%2FaaaaaaaaaaA", "/add?q=https%3A%2F%2Fyoutu.be%2FaaaaaaaaaaA")

	for _, mode := range []bool{false, true} {
		body := a.do(mode, "GET", "/add", nil).Body.String()
		for _, want := range []string{`hx-post="/ui/add/lookup"`, "Or upload a file", `href="/upload/batch"`, `href="/upload/import"`, `name="track"`} {
			if !strings.Contains(body, want) {
				t.Errorf("/add (secret %v) lacks %q", mode, want)
			}
		}
		if strings.Contains(body, `hx-trigger="load, submit"`) {
			t.Errorf("/add without q runs a lookup (secret %v)", mode)
		}
	}
	// with q: the box is filled and looked up on load
	body := a.do(false, "GET", "/add?q="+url.QueryEscape("https://musicbrainz.org/release/"+mezzanine), nil).Body.String()
	if !strings.Contains(body, `hx-trigger="load, submit"`) || !strings.Contains(body, `value="https://musicbrainz.org/release/`+mezzanine+`"`) {
		t.Errorf("/add?q= doesn't look up on load")
	}
	normal := a.do(false, "GET", "/add", nil).Body.String()
	for _, hidden := range []string{"Paste a link, or upload", "Download from URL", "secret"} {
		if strings.Contains(strings.ToLower(normal), strings.ToLower(hidden)) {
			t.Errorf("/add in normal mode mentions %q", hidden)
		}
	}
}

// Secret mode without a secret source has nothing to look up: only the upload part.
func TestAddPageSecretWithoutSource(t *testing.T) {
	a := newModeApp(t)
	secret.SetDefault(nil)
	body := a.do(true, "GET", "/add", nil).Body.String()
	if strings.Contains(body, "/ui/add/lookup") || !strings.Contains(body, "Or upload a file") {
		t.Errorf("add page without a source:\n%s", body)
	}
	if rec := a.do(true, "POST", "/ui/add/lookup", url.Values{"q": {"words"}}); rec.Code != http.StatusBadRequest || strings.TrimSpace(rec.Body.String()) != msgSecretNoLinks {
		t.Errorf("lookup = %d %q", rec.Code, rec.Body)
	}
}

// The kind choices on /add must be one radio group: radios only group within
// the same form owner, so a form= on some of them would leave the episode
// choice checked for good.
func TestAddKindChoicesShareOneGroup(t *testing.T) {
	a := newModeApp(t)
	re := regexp.MustCompile(`<input id="kind-[^>]*>`)
	for _, mode := range []bool{false, true} {
		inputs := re.FindAllString(a.do(mode, "GET", "/add", nil).Body.String(), -1)
		if len(inputs) < 2 {
			t.Fatalf("secret %v: found %d kind choices", mode, len(inputs))
		}
		for _, in := range inputs {
			if !strings.Contains(in, `name="kind"`) || strings.Contains(in, "form=") {
				t.Errorf("secret %v: choice not in the shared group: %s", mode, in)
			}
		}
	}
}
