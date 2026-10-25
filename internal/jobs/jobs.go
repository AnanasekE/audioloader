// Package jobs is the background job queue: jobs are rows in the jobs table,
// worked through by worker goroutines (Run), one job at a time each. A
// worker can be limited to some job types, so long downloads and long
// MusicBrainz checks run side by side instead of waiting on each other.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"audioloader/internal/db"
)

// Handler does one job. Returning an error retries the job with backoff,
// unless it is wrapped with Permanent.
type Handler func(ctx context.Context, payload json.RawMessage) error

// MaxAttempts is how often a job is tried before it is marked failed.
const MaxAttempts = 5

// BaseBackoff is the wait before the first retry; it doubles each attempt.
var BaseBackoff = 30 * time.Second

// PollInterval is how often an idle worker looks for due jobs.
var PollInterval = 2 * time.Second

var (
	mu       sync.RWMutex
	handlers = map[string]Handler{}
	// wake makes idle workers look for work right after an Enqueue
	wakeMu  sync.Mutex
	wakeChs []chan struct{}
)

func wakeAll() {
	wakeMu.Lock()
	defer wakeMu.Unlock()
	for _, ch := range wakeChs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func wakeChan() chan struct{} {
	ch := make(chan struct{}, 1)
	wakeMu.Lock()
	wakeChs = append(wakeChs, ch)
	wakeMu.Unlock()
	return ch
}

// Register sets the handler for jobs of type typ.
func Register(typ string, h Handler) {
	mu.Lock()
	defer mu.Unlock()
	handlers[typ] = h
}

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks err as not worth retrying.
func Permanent(err error) error { return permanentError{err} }

// Enqueue adds a job that runs as soon as the worker gets to it.
func Enqueue(typ string, payload any) error {
	return EnqueueAt(typ, "", payload, time.Time{})
}

// EnqueueAt adds a job that runs no earlier than at (zero: now). With a
// non-empty key, it does nothing if a job of the same type and key is
// already queued or running.
func EnqueueAt(typ, key string, payload any, at time.Time) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var keyArg any
	if key != "" {
		keyArg = key
	}
	runAfter := sqlTime(time.Now())
	if !at.IsZero() {
		runAfter = sqlTime(at)
	}
	_, err = db.DB.Exec(
		`INSERT OR IGNORE INTO jobs(type, payload, dedupe_key, run_after) VALUES(?, ?, ?, ?);`,
		typ, string(data), keyArg, runAfter,
	)
	if err == nil {
		wakeAll()
	}
	return err
}

// sqlTime formats t like SQLite's datetime(), so it compares as text.
func sqlTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

// Lane picks the job types a worker runs: Only those, or all but Except.
// The zero Lane runs every type.
type Lane struct {
	Only   []string
	Except []string
}

func (l Lane) where() (string, []any) {
	var args []any
	switch {
	case len(l.Only) > 0:
		for _, t := range l.Only {
			args = append(args, t)
		}
		return ` AND type IN (?` + strings.Repeat(", ?", len(l.Only)-1) + `)`, args
	case len(l.Except) > 0:
		for _, t := range l.Except {
			args = append(args, t)
		}
		return ` AND type NOT IN (?` + strings.Repeat(", ?", len(l.Except)-1) + `)`, args
	}
	return "", nil
}

// ResetRunning queues jobs left running by workers that were stopped
// mid-job. Call it once at startup, before starting workers.
func ResetRunning() error {
	_, err := db.DB.Exec(`UPDATE jobs SET status = 'queued' WHERE status = 'running';`)
	return err
}

// Run works through the lane's due jobs until ctx is cancelled.
func Run(ctx context.Context, lane Lane) {
	wake := wakeChan()
	for ctx.Err() == nil {
		ran, err := RunOnce(ctx, lane)
		if err != nil {
			log.Println("jobs:", err)
		}
		if ran {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-time.After(PollInterval):
		}
	}
}

// RunOnce runs the lane's next due job, if any, and reports whether it ran
// one. The error is about the queue itself; a failing job is recorded on its row.
func RunOnce(ctx context.Context, lane Lane) (bool, error) {
	var (
		id       int64
		typ      string
		payload  string
		attempts int
	)
	cond, args := lane.where()
	err := db.DB.QueryRow(`
		UPDATE jobs SET status = 'running', attempts = attempts + 1
		WHERE id = (
			SELECT id FROM jobs
			WHERE status = 'queued' AND run_after <= ?`+cond+`
			ORDER BY run_after, id LIMIT 1
		)
		RETURNING id, type, payload, attempts;`, append([]any{sqlTime(time.Now())}, args...)...,
	).Scan(&id, &typ, &payload, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	mu.RLock()
	h := handlers[typ]
	mu.RUnlock()

	var jobErr error
	if h == nil {
		jobErr = Permanent(fmt.Errorf("no handler for job type %q", typ))
	} else {
		jobErr = runHandler(ctx, h, json.RawMessage(payload))
	}
	if ctx.Err() != nil && jobErr != nil {
		// shutting down: put it back without counting the attempt
		_, err := db.DB.Exec(`UPDATE jobs SET status = 'queued', attempts = attempts - 1 WHERE id = ?;`, id)
		return true, err
	}
	return true, finish(id, attempts, jobErr)
}

// runHandler calls h, turning a panic into an error so one bad job can't
// stop the worker.
func runHandler(ctx context.Context, h Handler, payload json.RawMessage) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = Permanent(fmt.Errorf("panic: %v", p))
		}
	}()
	return h(ctx, payload)
}

func finish(id int64, attempts int, jobErr error) error {
	if jobErr == nil {
		_, err := db.DB.Exec(`UPDATE jobs SET status = 'done', last_error = NULL WHERE id = ?;`, id)
		return err
	}
	var perm permanentError
	if errors.As(jobErr, &perm) || attempts >= MaxAttempts {
		_, err := db.DB.Exec(`UPDATE jobs SET status = 'failed', last_error = ? WHERE id = ?;`, jobErr.Error(), id)
		return err
	}
	backoff := BaseBackoff << (attempts - 1)
	_, err := db.DB.Exec(
		`UPDATE jobs SET status = 'queued', last_error = ?, run_after = ? WHERE id = ?;`,
		jobErr.Error(), sqlTime(time.Now().Add(backoff)), id,
	)
	return err
}

// Counts returns the number of jobs per status, optionally only of the given types.
func Counts(types ...string) (map[string]int, error) {
	q := `SELECT status, COUNT(*) FROM jobs`
	args := make([]any, len(types))
	if len(types) > 0 {
		q += ` WHERE type IN (?` + strings.Repeat(", ?", len(types)-1) + `)`
		for i, t := range types {
			args[i] = t
		}
	}
	rows, err := db.DB.Query(q+` GROUP BY status;`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		counts[s] = n
	}
	return counts, rows.Err()
}

// RetryFailed queues the failed jobs of the given types again.
func RetryFailed(types ...string) error {
	q := `UPDATE jobs SET status = 'queued', attempts = 0, run_after = ? WHERE status = 'failed'`
	args := []any{sqlTime(time.Now())}
	if len(types) > 0 {
		q += ` AND type IN (?` + strings.Repeat(", ?", len(types)-1) + `)`
		for _, t := range types {
			args = append(args, t)
		}
	}
	// a retried job may collide with a newer queued one of the same key
	_, err := db.DB.Exec(`UPDATE OR IGNORE`+q[len(`UPDATE`):]+`;`, args...)
	if err == nil {
		wakeAll()
	}
	return err
}

// Job is a row of the queue, for status pages.
type Job struct {
	ID        int64
	Status    string
	Payload   json.RawMessage
	LastError string
	CreatedAt string
}

// Recent returns the newest jobs of a type.
func Recent(typ string, limit int) ([]Job, error) {
	rows, err := db.DB.Query(`
		SELECT id, status, payload, COALESCE(last_error, ''), created_at FROM jobs
		WHERE type = ? ORDER BY id DESC LIMIT ?;`, typ, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		var payload string
		if err := rows.Scan(&j.ID, &j.Status, &payload, &j.LastError, &j.CreatedAt); err != nil {
			return nil, err
		}
		j.Payload = json.RawMessage(payload)
		out = append(out, j)
	}
	return out, rows.Err()
}
