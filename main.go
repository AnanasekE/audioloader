package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
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
	downloads.Default() // logs the yt-dlp version, or why it's unavailable
	go downloads.Schedule(ctx)
	if err := jobs.ResetRunning(); err != nil {
		log.Fatal(err)
	}
	// downloads get their own worker, so a long download and a long
	// MusicBrainz check don't hold each other up
	downloadJobs := []string{downloads.JobDownload, downloads.JobYouTubeImport}
	var workers sync.WaitGroup
	for _, lane := range []jobs.Lane{{Only: downloadJobs}, {Except: downloadJobs}} {
		workers.Go(func() { jobs.Run(ctx, lane) })
	}

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
	workers.Wait()
}
