package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"audioloader/internal/db"
	"audioloader/internal/dbtest"
)

type jobRow struct {
	status   string
	attempts int
	lastErr  string
	due      bool
}

func jobState(t *testing.T, id int) jobRow {
	t.Helper()
	var r jobRow
	var lastErr *string
	err := db.DB.QueryRow(
		`SELECT status, attempts, last_error, run_after <= datetime('now') FROM jobs WHERE id = ?;`, id,
	).Scan(&r.status, &r.attempts, &lastErr, &r.due)
	if err != nil {
		t.Fatal(err)
	}
	if lastErr != nil {
		r.lastErr = *lastErr
	}
	return r
}

func TestRunOnceSuccess(t *testing.T) {
	dbtest.New(t)
	var got string
	Register("test_ok", func(ctx context.Context, p json.RawMessage) error {
		return json.Unmarshal(p, &got)
	})
	if err := Enqueue("test_ok", "hello"); err != nil {
		t.Fatal(err)
	}
	ran, err := RunOnce(context.Background(), Lane{})
	if err != nil || !ran {
		t.Fatalf("ran = %v, err = %v", ran, err)
	}
	if got != "hello" {
		t.Fatalf("payload = %q", got)
	}
	if s := jobState(t, 1); s.status != "done" {
		t.Fatalf("status = %s", s.status)
	}
	if ran, _ := RunOnce(context.Background(), Lane{}); ran {
		t.Fatal("ran a job twice")
	}
}

func TestRetryWithBackoff(t *testing.T) {
	dbtest.New(t)
	Register("test_fail", func(ctx context.Context, p json.RawMessage) error {
		return errors.New("boom")
	})
	Enqueue("test_fail", nil)
	RunOnce(context.Background(), Lane{})
	s := jobState(t, 1)
	if s.status != "queued" || s.attempts != 1 || s.lastErr != "boom" || s.due {
		t.Fatalf("after one failure: %+v", s)
	}
	// not due yet, so nothing runs
	if ran, _ := RunOnce(context.Background(), Lane{}); ran {
		t.Fatal("ran before its backoff ended")
	}

	// the last allowed attempt marks it failed
	db.DB.Exec(`UPDATE jobs SET attempts = ?, run_after = datetime('now', '-1 second');`, MaxAttempts-1)
	RunOnce(context.Background(), Lane{})
	if s := jobState(t, 1); s.status != "failed" || s.attempts != MaxAttempts {
		t.Fatalf("after max attempts: %+v", s)
	}

	if err := RetryFailed("test_fail"); err != nil {
		t.Fatal(err)
	}
	if s := jobState(t, 1); s.status != "queued" || s.attempts != 0 {
		t.Fatalf("after retry: %+v", s)
	}
}

func TestPermanentAndUnknown(t *testing.T) {
	dbtest.New(t)
	Register("test_perm", func(ctx context.Context, p json.RawMessage) error {
		return Permanent(errors.New("no"))
	})
	Enqueue("test_perm", nil)
	Enqueue("test_unknown", nil)
	RunOnce(context.Background(), Lane{})
	RunOnce(context.Background(), Lane{})
	if s := jobState(t, 1); s.status != "failed" {
		t.Fatalf("permanent: %+v", s)
	}
	if s := jobState(t, 2); s.status != "failed" {
		t.Fatalf("unknown type: %+v", s)
	}
}

func TestDedupe(t *testing.T) {
	dbtest.New(t)
	EnqueueAt("test_ok", "k", nil, time.Time{})
	EnqueueAt("test_ok", "k", nil, time.Time{})
	counts, err := Counts("test_ok")
	if err != nil {
		t.Fatal(err)
	}
	if counts["queued"] != 1 {
		t.Fatalf("counts = %v", counts)
	}
}

func TestRunStopsWhenCancelled(t *testing.T) {
	dbtest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	Register("test_ctx", func(ctx context.Context, p json.RawMessage) error {
		cancel()
		return ctx.Err()
	})
	Enqueue("test_ctx", nil)
	done := make(chan struct{})
	go func() {
		Run(ctx, Lane{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run kept going after cancel")
	}
	// the interrupted job is queued again without using up an attempt
	if s := jobState(t, 1); s.status != "queued" || s.attempts != 0 {
		t.Fatalf("job = %+v", s)
	}
}

func TestLanes(t *testing.T) {
	dbtest.New(t)
	Register("test_a", func(ctx context.Context, p json.RawMessage) error { return nil })
	Register("test_b", func(ctx context.Context, p json.RawMessage) error { return nil })
	Enqueue("test_a", nil)
	Enqueue("test_b", nil)
	if ran, _ := RunOnce(context.Background(), Lane{Only: []string{"test_b"}}); !ran {
		t.Fatal("lane didn't run its job")
	}
	if s := jobState(t, 2); s.status != "done" {
		t.Fatalf("test_b = %+v", s)
	}
	if ran, _ := RunOnce(context.Background(), Lane{Except: []string{"test_a"}}); ran {
		t.Fatal("lane ran an excluded job")
	}
	if s := jobState(t, 1); s.status != "queued" {
		t.Fatalf("test_a = %+v", s)
	}
}
