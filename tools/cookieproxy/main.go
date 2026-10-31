// Command cookieproxy forwards to an upstream app and adds a Cookie header
// to every request, so a headless browser can see the vault unlocked.
//
//	cookieproxy -listen 127.0.0.1:18091 -upstream http://127.0.0.1:18090 -cookie 'al_vault=...'
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:18091", "address to listen on")
	upstream := flag.String("upstream", "http://127.0.0.1:18090", "app to forward to")
	cookie := flag.String("cookie", "", "Cookie header value to add")
	flag.Parse()
	u, err := url.Parse(*upstream)
	if err != nil {
		log.Fatal(err)
	}
	p := httputil.NewSingleHostReverseProxy(u)
	orig := p.Director
	p.Director = func(r *http.Request) {
		orig(r)
		r.Header.Set("Cookie", *cookie)
	}
	log.Fatal(http.ListenAndServe(*listen, p))
}
