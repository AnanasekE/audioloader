package main

import (
	"log"
	"net/http"
	"text/template"

	"audioloader/internal/api"
)

func main() {
	mux := http.NewServeMux()

	// Static assets
	mux.Handle("GET /assets/", http.StripPrefix(
		"/assets/",
		http.FileServer(http.Dir("assets")),
	))

	// HTML templates
	tmpl := template.Must(template.ParseGlob("templates/*.html"))

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		if err := tmpl.ExecuteTemplate(w, "layout.html", nil); err != nil {
			log.Printf("render template: %v", err)
		}
	})

	// API routes
	api.Register(mux)

	log.Println("Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
