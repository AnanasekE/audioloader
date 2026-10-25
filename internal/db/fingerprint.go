package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"os"
	"uuid"

	sqlite "github.com/glebarez/go-sqlite"
)

// HeadSize is how much of a file's start is hashed for its fingerprint.
// The browser hashes the same amount (assets/js/track-hash.js), so keep them in sync.
const HeadSize = 64 << 10

// ErrExists is returned when a track with the same fingerprint is already stored.
var ErrExists = errors.New("track already exists")

// Fingerprint identifies a file by its size and a hash of its first HeadSize
// bytes. The size guards against files that only share a header, such as
// MP3s with the same embedded cover art.
type Fingerprint struct {
	Size     int64  `json:"size"`
	HeadHash string `json:"hash"`
}

// FingerprintOf reads r's size and head, then rewinds it.
func FingerprintOf(r io.ReadSeeker) (Fingerprint, error) {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return Fingerprint{}, err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return Fingerprint{}, err
	}
	h := sha256.New()
	if _, err := io.CopyN(h, r, HeadSize); err != nil && !errors.Is(err, io.EOF) {
		return Fingerprint{}, err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return Fingerprint{}, err
	}
	return Fingerprint{Size: size, HeadHash: hex.EncodeToString(h.Sum(nil))}, nil
}

func TrackExists(fp Fingerprint) (bool, error) {
	var n int
	err := DB.QueryRow(
		`SELECT COUNT(*) FROM tracks WHERE file_size = ? AND head_hash = ?;`,
		fp.Size, fp.HeadHash,
	).Scan(&n)
	return n > 0, err
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	var e *sqlite.Error
	// 2067 = SQLITE_CONSTRAINT_UNIQUE
	return errors.As(err, &e) && e.Code() == 2067
}

// migrateFingerprints adds the fingerprint columns to older databases,
// fills them in for existing tracks, and then enforces uniqueness.
func migrateFingerprints(db *sql.DB) error {
	for _, col := range []string{"file_size INTEGER", "head_hash TEXT"} {
		_, err := db.Exec(`ALTER TABLE tracks ADD COLUMN ` + col + `;`)
		if err != nil && !isDuplicateColumn(err) {
			return err
		}
	}
	if err := backfillFingerprints(db); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS tracks_fingerprint ON tracks(file_size, head_hash);`)
	return err
}

func isDuplicateColumn(err error) bool {
	var e *sqlite.Error
	// SQLITE_ERROR with "duplicate column name: ..."
	return errors.As(err, &e) && e.Code() == 1
}

func backfillFingerprints(db *sql.DB) error {
	rows, err := db.Query(`SELECT uuid FROM tracks WHERE head_hash IS NULL;`)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		fp, err := fingerprintStored(id)
		if err != nil {
			// a missing file shouldn't stop startup; that track just can't be matched
			log.Printf("fingerprint %s: %v", id, err)
			continue
		}
		_, err = db.Exec(
			`UPDATE tracks SET file_size = ?, head_hash = ? WHERE uuid = ?;`,
			fp.Size, fp.HeadHash, id.String(),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func fingerprintStored(id uuid.UUID) (Fingerprint, error) {
	path, err := TrackFilePath(id)
	if err != nil {
		return Fingerprint{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Fingerprint{}, err
	}
	defer f.Close()
	return FingerprintOf(f)
}
