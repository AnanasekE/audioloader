package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"audioloader/internal/api"
	"audioloader/internal/db"
	"audioloader/internal/downloads"
	"audioloader/internal/jobs"
	"audioloader/internal/tagging"
	"audioloader/internal/tracking"
	"audioloader/internal/vault"
)

func main() {
	hashSecret := flag.Bool("hash-secret", false, "read a passphrase from stdin and print its hash for AUDIOLOADER_SECRET")
	flag.Parse()
	if *hashSecret {
		printSecretHash()
		return
	}

	db.Init()
	defer db.DB.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	v, err := vault.FromEnv("data/secret.key")
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()

	api.Register(mux, v)

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

	srv := &http.Server{Addr: ":8080", Handler: api.WithViewer(mux)}
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

// printSecretHash reads a passphrase from stdin and prints its bcrypt hash,
// to put into AUDIOLOADER_SECRET.
func printSecretHash() {
	fmt.Fprint(os.Stderr, "Passphrase: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		log.Fatal(err)
	}
	hash, err := vault.Hash(strings.TrimRight(line, "\r\n"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(hash)
}
