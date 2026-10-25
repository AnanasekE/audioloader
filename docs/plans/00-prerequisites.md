# Plan 0: Prerequisites

**Goal:** put in the groundwork that plans 1–5 depend on, and nothing more.

**Depends on:** nothing.

## Steps

### 1. Versioned migrations
- Keep the current `schema` const as migration 1 (`CREATE TABLE IF NOT EXISTS` is safe on existing DBs). Make `migrateFingerprints` migration 2.
- In `internal/db/migrate.go`, add `var migrations = []func(*sql.Tx) error{...}`. Use `PRAGMA user_version` to track how many have run. Run each pending one in its own transaction and bump `user_version` inside that same transaction.
- An existing `database.db` has `user_version = 0` but its tables already exist. Migrations 1 and 2 must be idempotent so that DB passes through them safely.
- **Verify:** start the app on a copy of the current `database.db` and on an empty file. Both end at the latest version, and no data is lost.

### 2. Testable DB package
- `connect` hardcodes `./database.db`. Change it to `Open(dsn string) (*sql.DB, error)`. `Init` keeps the old path.
- Add a test helper: `newTestDB(t)` opens a fresh file in `t.TempDir()`, runs migrations, and sets `db.DB`. Also point `TracksDir` into a temp dir.
- Add first tests: migrations run twice without error, and `AddTrack` duplicate → `ErrExists`.
- **Verify:** `go test ./internal/db/...` passes.

### 3. Unique artist names
- Migration: merge artists whose `lower(trim(name))` match into the oldest row. Repoint `track_artist_map` and `release_artist_map`, and drop the duplicate map rows the repointing creates (`release_artist_map` has a primary key). Then `CREATE UNIQUE INDEX artists_name ON artists(lower(name))`.
- `AddArtist`: on a unique violation, return a new `ErrArtistExists`. `createArtist` maps it to 409 and shows it in the form.
- Test: the merge migration on a seeded DB with "Foo", "foo " and "FOO".

### 4. Edit release metadata
- `GET /releases/{id}/edit` already exists for the tracklist. Add a metadata form there: title, type, date, artists. Reuse `validateRelease` and the artist picker.
- Add `db.UpdateRelease(id, title, typ, date, artistIDs)` in one transaction, and the route `POST /ui/releases/{id}`.
- Delete a release: `POST /ui/releases/{id}/delete`. `release_tracks` rows cascade. Tracks stay in the library.
- Test: update keeps the tracklist intact, and delete leaves the tracks in place.

### 5. Release cover storage
- Store covers at `data/covers/<release_uuid>.<ext>` and serve them via `GET /covers/{release_id}`. Allowlist jpg/png/webp, check the type with `http.DetectContentType`, and cap uploads at 10 MiB.
- Add `db.SetReleaseCover(id, r io.Reader, ext)`. It sets `releases.img_url` to `/covers/<id>?v=<unix>` to bust caches. Plan 1 calls the same function for Cover Art Archive images.
- Add an upload form on the release edit page.
- **Track cover rule:** a track shows the cover of its first release by type priority album > ep > single > compilation, falling back to `tracks.img_url`. Compute it in `attachReleases`, not in SQL.

### 6. Placeholder tracks and the central track filter
- Migration: add `tracks.has_file INTEGER NOT NULL DEFAULT 1`. Placeholder rows have `has_file = 0`, a NULL fingerprint, and `duration_seconds` taken from metadata (0 if unknown).
- Add `const visibleTracks = "t.has_file = 1"` (or a small helper) to `tracks.go`. Library lists (`ListTracks`, the track picker on the release edit page, `NextPositions` users) use it.
- Release pages show **all** tracks. Missing ones render greyed out without a play button. Add a `HasFile bool` field on `audio.Track`.
- `streamTrack` already returns 404 when the file is missing. Keep that behaviour.
- Add `db.AttachFile(trackID, file, ext)`. It fills in an existing placeholder: save the file, set the fingerprint, duration and `has_file = 1`. On a fingerprint collision with another track, return `ErrExists` along with the existing track ID, so the caller can merge the two.
- Test: a placeholder doesn't appear in `ListTracks`, does appear in `GetRelease`, and `AttachFile` turns it into a normal track.

## Out of scope
Track edit/delete, artist pages, upload hardening. They're listed in FEATURES.md and can be done anytime.
