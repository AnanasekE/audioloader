package secret

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	pageHost  = "pages.example.test"
	mediaHost = "media.example.test"
)

// fake serves the pages and files of a pretend source. Every host name goes
// to the one test server, which answers by Host, so the tests use the same
// URLs a real source would have, over https, without any network.
type fake struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	requests []string // "host path"
	referers map[string]string
	agents   map[string]string
	pages    map[string]string // path -> HTML
	files    map[string][]byte // "host/path" -> body
	redirect map[string]string // "host/path" -> Location
}

func newFake(t *testing.T) (*fake, *Source) {
	f := &fake{t: t, referers: map[string]string{}, agents: map[string]string{}, pages: map[string]string{}, files: map[string][]byte{}, redirect: map[string]string{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	s := New(pageHost)
	s.Media = []string{mediaHost}
	s.UserAgent = "audioloader-test"
	s.Client = &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // the test server's own certificate
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, f.srv.Listener.Addr().String())
		},
	}}
	return f, s
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Host+r.URL.Path)
	f.referers[r.Host+r.URL.Path] = r.Header.Get("Referer")
	f.agents[r.Host+r.URL.Path] = r.Header.Get("User-Agent")
	f.mu.Unlock()
	key := r.Host + r.URL.Path
	if loc, ok := f.redirect[key]; ok {
		http.Redirect(w, r, loc, http.StatusFound)
		return
	}
	if body, ok := f.files[key]; ok {
		w.Write(body)
		return
	}
	if r.Host == pageHost {
		if page, ok := f.pages[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(page))
			return
		}
	}
	http.NotFound(w, r)
}

func (f *fake) page(path, html string) string {
	f.pages[path] = html
	return "https://" + pageHost + path
}

func TestNormalize(t *testing.T) {
	_, s := newFake(t)
	for _, c := range []struct{ in, want string }{
		{"https://pages.example.test/items/someone/a-slug", "https://pages.example.test/items/someone/a-slug"},
		{"  https://PAGES.example.test/items/someone/a-slug/?utm=1#t  ", "https://pages.example.test/items/someone/a-slug"},
		{"https://pages.example.test:443/items/x/y", "https://pages.example.test/items/x/y"},
	} {
		got, err := s.Normalize(c.in)
		if err != nil || got != c.want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{
		"http://pages.example.test/items/a/b",            // not https
		"https://evil.example.test/items/a/b",            // another host
		"https://pages.example.test.evil.test/items/a/b", // a lookalike
		"https://evil.test/?https://pages.example.test/items/a/b",
		"https://user:pw@pages.example.test/items/a/b", // credentials
		"https://pages.example.test",               // not a page
		"https://pages.example.test/",
		"https://media.example.test/sounds/a.m4a", // a media host isn't a page host
		"javascript:alert(1)",
		"",
	} {
		if got, err := s.Normalize(bad); !errors.Is(err, ErrBadURL) {
			t.Errorf("Normalize(%q) = %q, %v; want ErrBadURL", bad, got, err)
		}
	}
}

func TestWrongHostIsNeverFetched(t *testing.T) {
	f, s := newFake(t)
	if _, err := s.Inspect(context.Background(), "https://evil.example.test/items/a/b"); !errors.Is(err, ErrBadURL) {
		t.Fatalf("err = %v", err)
	}
	if len(f.requests) != 0 {
		t.Fatalf("requests = %v", f.requests)
	}
}

func TestInspectReadsTheMetaTags(t *testing.T) {
	f, s := newFake(t)
	url := f.page("/items/someone/tags", `<html><head><title>Plain title</title>
		<meta property="og:title" content="  Tagged  title ">
		<meta property="og:description" content="About it">
		<meta property="og:audio" content="https://media.example.test/sounds/one.m4a">
		</head><body><audio src="https://media.example.test/sounds/other.m4a"></audio></body></html>`)
	p, err := s.Inspect(context.Background(), url+"/?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != url || p.Title != "Tagged title" || p.Description != "About it" || p.AudioURL != "https://media.example.test/sounds/one.m4a" {
		t.Fatalf("page = %+v", p)
	}
	if got := f.agents[pageHost+"/items/someone/tags"]; got != "audioloader-test" {
		t.Errorf("user agent = %q", got)
	}
	if len(f.requests) != 1 {
		t.Errorf("requests = %v, want the one page", f.requests)
	}
}

func TestInspectAudioTagsAndRelativeURLs(t *testing.T) {
	f, s := newFake(t)
	// og:audio:secure_url when there is no og:audio
	p, err := s.Inspect(context.Background(), f.page("/items/a/secure", `<meta property="og:audio:secure_url" content="https://media.example.test/s.mp3">`))
	if err != nil || p.AudioURL != "https://media.example.test/s.mp3" {
		t.Fatalf("secure_url: %+v, %v", p, err)
	}
	// <audio src>, then <source src>
	p, err = s.Inspect(context.Background(), f.page("/items/a/audio", `<audio controls><source src="/files/x.ogg" type="audio/ogg"></audio>`))
	if err != nil || p.AudioURL != "https://pages.example.test/files/x.ogg" {
		t.Fatalf("source: %+v, %v", p, err)
	}
	p, err = s.Inspect(context.Background(), f.page("/items/a/audio2", `<audio src="https://media.example.test/y.opus"></audio><audio><source src="https://media.example.test/z.opus"></audio>`))
	if err != nil || p.AudioURL != "https://media.example.test/y.opus" {
		t.Fatalf("audio: %+v, %v", p, err)
	}
}

func TestInspectFindsAnAudioURLInAScript(t *testing.T) {
	f, s := newFake(t)
	// a jPlayer setup: the file is only in the script, slashes escaped like a JSON string
	url := f.page("/items/someone/player", `<html><head><title>Fallback</title></head><body>
		<div class="jp-title" aria-label="title">  My   track </div>
		<div class="jp-description"><p>First line<br>Second line</p><p>Next <b>paragraph</b></p></div>
		<script>
		var tracker = "https://other.example.test/pixel.mp3";
		var cover = "https://media.example.test/sounds/cover.jpg";
		$("#player").jPlayer({ ready: function () {
			$(this).jPlayer("setMedia", { m4a: "https:\/\/media.example.test\/sounds\/abc123.m4a" });
		}});
		</script></body></html>`)
	p, err := s.Inspect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if p.AudioURL != "https://media.example.test/sounds/abc123.m4a" {
		t.Errorf("audio = %q", p.AudioURL)
	}
	if p.Title != "My track" {
		t.Errorf("title = %q", p.Title)
	}
	if p.Description != "First line\nSecond line\nNext paragraph" {
		t.Errorf("description = %q", p.Description)
	}
}

func TestInspectDefaultsOrder(t *testing.T) {
	f, s := newFake(t)
	src := `<script>x("https://media.example.test/a.mp3")</script>`
	// og beats jPlayer beats <title>
	p, _ := s.Inspect(context.Background(), f.page("/items/a/1", `<title>T</title><div class="jp-title">J</div><meta property="og:title" content="O">`+src))
	if p.Title != "O" {
		t.Errorf("title = %q, want og:title", p.Title)
	}
	p, _ = s.Inspect(context.Background(), f.page("/items/a/2", `<title>T</title><div class="jp-title">J</div>`+src))
	if p.Title != "J" {
		t.Errorf("title = %q, want the jp-title", p.Title)
	}
	p, _ = s.Inspect(context.Background(), f.page("/items/a/3", `<title> The   title </title>`+src))
	if p.Title != "The title" {
		t.Errorf("title = %q, want <title>", p.Title)
	}
	p, _ = s.Inspect(context.Background(), f.page("/items/a/4", `<div class="jp-description">only this</div><meta property="og:description" content="from og">`+src))
	if p.Description != "from og" {
		t.Errorf("description = %q", p.Description)
	}
	p, _ = s.Inspect(context.Background(), f.page("/items/a/5", `<div class="jp-description">only this</div>`+src))
	if p.Description != "only this" {
		t.Errorf("description = %q", p.Description)
	}
	p, _ = s.Inspect(context.Background(), f.page("/items/a/6", src))
	if p.Title != "" || p.Description != "" {
		t.Errorf("empty page = %+v", p)
	}
}

func TestInspectRefusesAudioOnAnotherHost(t *testing.T) {
	f, s := newFake(t)
	for name, html := range map[string]string{
		"og:audio":   `<meta property="og:audio" content="https://evil.example.test/a.mp3">`,
		"audio tag":  `<audio src="https://evil.example.test/a.mp3"></audio>`,
		"http":       `<meta property="og:audio" content="http://media.example.test/a.mp3">`,
		"credential": `<audio src="https://user@media.example.test/a.mp3"></audio>`,
		"lookalike":  `<audio src="https://media.example.test.evil.test/a.mp3"></audio>`,
	} {
		_, err := s.Inspect(context.Background(), f.page("/items/a/"+strings.ReplaceAll(name, " ", "-"), html))
		if !errors.Is(err, ErrAudioHost) {
			t.Errorf("%s: err = %v, want ErrAudioHost", name, err)
		}
	}
	// a URL in script text on a host that isn't allowed is passed over, and
	// one that isn't audio too: with nothing else, there is no audio
	_, err := s.Inspect(context.Background(), f.page("/items/a/script", `<script>a("https://evil.example.test/a.mp3"); b("https://media.example.test/a.jpg")</script>`))
	if !errors.Is(err, ErrNoAudio) {
		t.Errorf("script: err = %v, want ErrNoAudio", err)
	}
}

func TestInspectWithoutAudio(t *testing.T) {
	f, s := newFake(t)
	if _, err := s.Inspect(context.Background(), f.page("/items/a/none", `<title>Nothing</title><p>text</p>`)); !errors.Is(err, ErrNoAudio) {
		t.Errorf("err = %v", err)
	}
	if _, err := s.Inspect(context.Background(), "https://pages.example.test/items/a/missing"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("missing page: err = %v", err)
	}
}

func TestRedirectsStayOnAllowedHosts(t *testing.T) {
	f, s := newFake(t)
	// a page that moves within the host is followed
	f.redirect[pageHost+"/items/a/old"] = "https://pages.example.test/items/a/new"
	f.page("/items/a/new", `<meta property="og:audio" content="https://media.example.test/a.mp3">`)
	if p, err := s.Inspect(context.Background(), "https://pages.example.test/items/a/old"); err != nil || p.AudioURL == "" {
		t.Fatalf("same-host redirect: %+v, %v", p, err)
	}
	// off the host: refused, and the other host is never asked
	f.redirect[pageHost+"/items/a/away"] = "https://evil.example.test/items/a/new"
	before := len(f.requests)
	if _, err := s.Inspect(context.Background(), "https://pages.example.test/items/a/away"); !errors.Is(err, ErrOffHost) {
		t.Fatalf("off-host redirect: err = %v", err)
	}
	for _, r := range f.requests[before:] {
		if strings.HasPrefix(r, "evil.") {
			t.Errorf("followed the redirect: %v", f.requests[before:])
		}
	}
	// downgrading to http is refused too
	f.redirect[pageHost+"/items/a/plain"] = "http://pages.example.test/items/a/new"
	if _, err := s.Inspect(context.Background(), "https://pages.example.test/items/a/plain"); !errors.Is(err, ErrOffHost) {
		t.Errorf("http redirect: err = %v", err)
	}
	// the audio file may not redirect off the allowed hosts either
	f.files[mediaHost+"/sounds/moved.m4a"] = nil
	delete(f.files, mediaHost+"/sounds/moved.m4a")
	f.redirect[mediaHost+"/sounds/moved.m4a"] = "https://evil.example.test/sounds/moved.m4a"
	_, err := s.Fetch(context.Background(), &Page{URL: "https://pages.example.test/items/a/x", AudioURL: "https://media.example.test/sounds/moved.m4a"}, t.TempDir())
	if !errors.Is(err, ErrOffHost) {
		t.Errorf("audio redirect: err = %v", err)
	}
}

func TestFetchDownloadsOnce(t *testing.T) {
	f, s := newFake(t)
	m4a, err := os.ReadFile("../../audio/testdata/chapters.m4a")
	if err != nil {
		t.Fatal(err)
	}
	f.files[mediaHost+"/sounds/abc.m4a"] = m4a
	f.page("/items/someone/one", `<script>setMedia({m4a: "https://media.example.test/sounds/abc.m4a"})</script>`)
	ctx := context.Background()
	page, err := s.Inspect(ctx, "https://pages.example.test/items/someone/one")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file, err := s.Fetch(ctx, page, dir)
	if err != nil {
		t.Fatal(err)
	}
	if file.Ext != ".m4a" || file.Duration <= 0 || !strings.HasPrefix(file.Path, dir) {
		t.Fatalf("file = %+v", file)
	}
	if b, _ := os.ReadFile(file.Path); len(b) != len(m4a) {
		t.Fatalf("stored %d bytes of %d", len(b), len(m4a))
	}
	// one page request, one file request; the page URL goes along as Referer
	if len(f.requests) != 2 {
		t.Errorf("requests = %v", f.requests)
	}
	if got := f.referers[mediaHost+"/sounds/abc.m4a"]; got != "https://pages.example.test/items/someone/one" {
		t.Errorf("referer = %q", got)
	}
	if got := f.agents[mediaHost+"/sounds/abc.m4a"]; got != "audioloader-test" {
		t.Errorf("user agent = %q", got)
	}
}

func TestFetchRefusals(t *testing.T) {
	f, s := newFake(t)
	m4a, _ := os.ReadFile("../../audio/testdata/chapters.m4a")
	ctx := context.Background()
	page := &Page{URL: "https://pages.example.test/items/a/x", AudioURL: "https://media.example.test/sounds/a.m4a"}

	// the page's audio URL is checked again, whoever built the Page
	for _, u := range []string{"https://evil.example.test/a.m4a", "http://media.example.test/a.m4a", "https://media.example.test/page.html"} {
		if _, err := s.Fetch(ctx, &Page{URL: page.URL, AudioURL: u}, t.TempDir()); !errors.Is(err, ErrAudioHost) {
			t.Errorf("Fetch(%s) = %v", u, err)
		}
	}
	if len(f.requests) != 0 {
		t.Fatalf("requests = %v", f.requests)
	}

	// over the size cap, announced or not
	f.files[mediaHost+"/sounds/a.m4a"] = m4a
	s.MaxAudio = int64(len(m4a)) - 1
	dir := t.TempDir()
	if _, err := s.Fetch(ctx, page, dir); !errors.Is(err, ErrTooLarge) {
		t.Errorf("too large: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
	s.MaxAudio = 1 << 20

	// not audio, though it has the name of one; nothing is kept
	f.files[mediaHost+"/sounds/b.m4a"] = []byte("<html>not audio</html>")
	if _, err := s.Fetch(ctx, &Page{URL: page.URL, AudioURL: "https://media.example.test/sounds/b.m4a"}, dir); err == nil {
		t.Error("accepted a file that is not audio")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
	// a missing file
	if _, err := s.Fetch(ctx, &Page{URL: page.URL, AudioURL: "https://media.example.test/sounds/none.m4a"}, dir); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("missing file: %v", err)
	}
}

func TestPageSizeCap(t *testing.T) {
	f, s := newFake(t)
	s.MaxPage = 100
	url := f.page("/items/a/big", `<meta property="og:audio" content="https://media.example.test/a.mp3">`+strings.Repeat(" ", 200))
	if _, err := s.Inspect(context.Background(), url); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v", err)
	}
}

func TestSlowPageTimesOut(t *testing.T) {
	f, s := newFake(t)
	s.PageTimeout = 50 * time.Millisecond
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(time.Second) })
	if _, err := s.Inspect(context.Background(), "https://pages.example.test/items/a/slow"); err == nil {
		t.Error("no timeout")
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("AUDIOLOADER_SECRET_SOURCE", "")
	if FromEnv() != nil {
		t.Fatal("the feature is on without a source")
	}
	t.Setenv("AUDIOLOADER_SECRET_SOURCE", " https://Pages.Example.Test/ ")
	t.Setenv("AUDIOLOADER_SECRET_SOURCE_MEDIA", "media.example.test, ,Cdn.Example.Test")
	s := FromEnv()
	if s == nil || s.Host != "pages.example.test" || len(s.Media) != 2 || s.Media[1] != "cdn.example.test" {
		t.Fatalf("source = %+v", s)
	}
}
