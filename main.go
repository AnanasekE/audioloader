package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"audioloader/internal/api"
	"audioloader/internal/db"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/tagging"
	"audioloader/internal/tracking"
)

func main() {
	db.Init()
	defer db.DB.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()

	api.Register(mux)

	tagging.Register()
	tracking.Register()
	go tracking.Schedule(ctx)
	downloads.Register()
	go downloads.Schedule(ctx)
	workerDone := make(chan struct{})
	go func() {
		jobs.Run(ctx)
		close(workerDone)
	}()

	srv := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	log.Println("Server running at http://localhost:8080")
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-workerDone
}
