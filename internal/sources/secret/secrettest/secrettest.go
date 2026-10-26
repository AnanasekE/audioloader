// Package secrettest is a pretend secret source for tests: pages and audio
// files served over https by one test server, under made-up host names, so
// code that talks to the source can run without any network.
package secrettest

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"audioloader/internal/sources/secret"
)

// The made-up hosts of the pretend source.
const (
	PageHost  = "pages.example.test"
	MediaHost = "media.example.test"
)

// Fake serves what its maps hold. Fill them before the first request.
type Fake struct {
	Srv *httptest.Server
	// Files maps "host/path" to a body; Redirect maps "host/path" to a Location.
	Files    map[string][]byte
	Redirect map[string]string
	// Pages maps a path on PageHost to HTML.
	Pages map[string]string

	mu       sync.Mutex
	requests []string
	Referers map[string]string
	Agents   map[string]string
}

// New starts the server and returns it with a Source (page host PageHost,
// media host MediaHost) whose connections all go to it.
func New(t testing.TB) (*Fake, *secret.Source) {
	f := &Fake{Files: map[string][]byte{}, Redirect: map[string]string{}, Pages: map[string]string{}, Referers: map[string]string{}, Agents: map[string]string{}}
	f.Srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Srv.Close)
	s := secret.New(PageHost)
	s.Media = []string{MediaHost}
	s.UserAgent = "audioloader-test"
	s.Client = &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // the test server's own certificate
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, f.Srv.Listener.Addr().String())
		},
	}}
	return f, s
}

// Requests lists what was asked for, as "host/path", in order.
func (f *Fake) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

// Page adds an HTML page and returns its URL.
func (f *Fake) Page(path, html string) string {
	f.Pages[path] = html
	return "https://" + PageHost + path
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	key := r.Host + r.URL.Path
	f.mu.Lock()
	f.requests = append(f.requests, key)
	f.Referers[key] = r.Header.Get("Referer")
	f.Agents[key] = r.Header.Get("User-Agent")
	f.mu.Unlock()
	if loc, ok := f.Redirect[key]; ok {
		http.Redirect(w, r, loc, http.StatusFound)
		return
	}
	if body, ok := f.Files[key]; ok {
		w.Write(body)
		return
	}
	if r.Host == PageHost {
		if page, ok := f.Pages[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(page))
			return
		}
	}
	http.NotFound(w, r)
}
