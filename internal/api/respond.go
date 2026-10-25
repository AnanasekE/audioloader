package api

import (
	"bytes"
	"log"
	"net/http"

	"github.com/a-h/templ"
)

// render writes c as HTML. It renders into a buffer first, so a failing
// component returns a clean 500 instead of a half-written page.
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

// serverError logs err and sends a generic 500, keeping internals out of the response.
func serverError(w http.ResponseWriter, err error) {
	log.Println("internal error:", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
