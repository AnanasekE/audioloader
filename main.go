package main

import (
	"log"
	"net/http"

	"audioloader/internal/api"
	"audioloader/internal/db"
)

func main() {
	db.Init()
	defer db.DB.Close()

	mux := http.NewServeMux()

	api.Register(mux)

	log.Println("Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
