// Package dbtest sets up a temporary database for tests outside internal/db.
package dbtest

import (
	"testing"

	"audioloader/internal/db"
)

// New points the db package at a fresh, migrated database for t.
func New(t testing.TB) {
	t.Helper()
	restore, err := db.UseTemp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
}
