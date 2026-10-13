package api

import (
	"fmt"
	"net/http"
)

func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/hello", hello)
	mux.HandleFunc("POST /api/hello", hello)
}

func hello(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<p>Hello from the API! <strong>HTMX works.</strong></p>`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"message":"Hello from the API!"}`)
}
