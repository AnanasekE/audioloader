# Plan 1: MusicBrainz auto-tagging

**Goal:** attach MusicBrainz IDs (MBIDs) to tracks, releases and artists automatically. Fill in missing metadata and covers. Send uncertain matches to a review page.

**Depends on:** plan 0 (migrations, tests, release editing, cover storage).

## Design

- **Never write tags into files.** Doing so changes the head hash, and duplicate detection would break.
- **Matching order, cheapest first:**
  1. MBIDs embedded in the file's tags (confidence 1.0).
  2. AcoustID lookup from a Chromaprint fingerprint (`fpcalc`).
  3. MusicBrainz text search on title + artist + duration.
- **Auto-apply threshold:** score ≥ 0.9 is applied automatically. 0.5–0.9 goes to review. Below 0.5 is recorded as "no match".
- **Config via env vars:**
  - `AUDIOLOADER_CONTACT`: put into the User-Agent `audioloader/0.1 (<contact>)`, which MusicBrainz requires.
  - `ACOUSTID_KEY`: optional. Without it, step 2 is skipped.
  - `FPCALC`: path to the binary, default `fpcalc` on PATH. If it's missing, step 2 is skipped and a warning is logged once.

## Schema (one migration)

```sql
ALTER TABLE tracks   ADD COLUMN mbid TEXT;   -- recording MBID
ALTER TABLE tracks   ADD COLUMN isrc TEXT;
ALTER TABLE releases ADD COLUMN mbid TEXT;   -- release MBID
ALTER TABLE releases ADD COLUMN release_group_mbid TEXT;
ALTER TABLE artists  ADD COLUMN mbid TEXT;
CREATE UNIQUE INDEX artists_mbid  ON artists(mbid)  WHERE mbid IS NOT NULL;
CREATE UNIQUE INDEX releases_mbid ON releases(mbid) WHERE mbid IS NOT NULL;

CREATE TABLE jobs (
  id INTEGER PRIMARY KEY,
  type TEXT NOT NULL,            -- 'tag_track', later 'check_artist', 'download', ...
  payload TEXT NOT NULL,         -- JSON
  status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','done','failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  run_after TEXT NOT NULL DEFAULT (datetime('now')),
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX jobs_next ON jobs(status, run_after);

CREATE TABLE tag_matches (
  track_id TEXT NOT NULL REFERENCES tracks(uuid) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('tags','acoustid','search')),
  recording_mbid TEXT NOT NULL,
  release_mbid TEXT,
  score REAL NOT NULL,
  data TEXT NOT NULL,            -- JSON snapshot of what would be applied
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','applied','rejected')),
  PRIMARY KEY (track_id, recording_mbid)
);
```

## Steps

### 1. Job queue and worker (`internal/jobs`)
- `Enqueue(type, payload)`, `Register(type, handler func(ctx, payload) error)`, and `Run(ctx)`.
- `Run` claims one job: `UPDATE ... SET status='running' WHERE id = (SELECT ... LIMIT 1) RETURNING ...`. On error, retry with exponential backoff via `run_after`, up to 5 attempts, then set `failed`.
- At startup, reset jobs left in `running` back to `queued`. Start the worker in `main.go`, with a context that is cancelled on SIGINT.
- Test: success, retry with backoff, and the failure cap.

### 2. Rate-limited HTTP client (`internal/mb`)
- `type Client` with its own `http.Client` (10 s timeout), the User-Agent, and a limiter per host (MusicBrainz 1/s, AcoustID 3/s). Use a ticker or `time.Sleep` under a mutex; no dependency needed.
- On 503, back off and retry once.
- Typed responses for the endpoints used:
  - `GET /ws/2/recording/{mbid}?inc=artists+releases+isrcs&fmt=json`
  - `GET /ws/2/release/{mbid}?inc=recordings+artist-credits+release-groups&fmt=json`
  - `GET /ws/2/recording?query=...&fmt=json`
  - AcoustID `/v2/lookup?meta=recordings+releasegroups`
- Base URLs are struct fields, so tests can point them at `httptest`.
- Test: the limiter spaces calls ≥ 1 s apart (inject a fake clock or measure), and JSON decoding uses fixture files in `internal/mb/testdata/`.

### 3. Read embedded MBIDs
- **New uploads:** in `assets/js/tags.js`, also extract these fields and send them as form fields from `upload.js`, `batch.js` and `import.js`:
  - recording ID: ID3 `UFID:http://musicbrainz.org`, Vorbis `MUSICBRAINZ_TRACKID`, MP4 `----:com.apple.iTunes:MusicBrainz Track Id`
  - release ID: `MusicBrainz Album Id` / `MUSICBRAINZ_ALBUMID`
  - release group ID: `MusicBrainz Release Group Id`
  - artist ID: `MusicBrainz Artist Id`
  - ISRC: `TSRC` / `ISRC`
  - `addTrack` stores them. Validate each MBID as a UUID and drop invalid ones.
- **Existing tracks:** add a server-side reader using `github.com/dhowden/tag` (`Raw()` exposes TXXX/UFID/Vorbis keys). Use it only in the tagging job, never on the upload path.

### 4. `tag_track` job
- Enqueue one per track: on upload, and via an "Auto-tag library" button that enqueues every track with `mbid IS NULL`.
- Skip tracks with `has_file = 0`. Leave a `skipTrack(t)` hook; plans 4 and 5 add podcast and secret checks to it.
- Pipeline: embedded MBIDs, then AcoustID (`fpcalc -json <path>` gives a fingerprint and duration), then text search.
- **Scoring:** combine title similarity (normalized: lowercase, strip punctuation and "feat."), artist overlap, duration within ±3 s, and release title match. Take the best candidate.
- Insert into `tag_matches`, then auto-apply if score ≥ 0.9.

### 5. Apply a match (`db.ApplyMatch`)
One transaction:
- Set `tracks.mbid` and `isrc`.
- For each credited artist: find by `mbid`, else by `lower(name)` (then set its `mbid`), else create.
- **Release:** find by `mbid`. Otherwise match an existing release the track is on by title, and set its `mbid` and `release_group_mbid`. Fill in an empty date or type, but never overwrite values the user set.
- Don't create new releases or reorder tracklists here. That belongs to plan 2.
- Enqueue `fetch_cover` if the release still has the default cover.
- Test with fixtures: apply is idempotent, user-set fields aren't overwritten, and artists are matched by name and given MBIDs.

### 6. `fetch_cover` job
- `GET https://coverartarchive.org/release/{mbid}/front-500`, falling back to `/release-group/{rg}/front-500`. 404 means no cover; don't retry.
- Save with `db.SetReleaseCover`.

### 7. Review page (`GET /tagging`)
- Pending matches grouped by track. Show the current values next to the proposed ones, the score and source, and a link to the MusicBrainz page.
- Actions: **Apply** (`POST /ui/tagging/{track}/{mbid}/apply`), **Reject**, and "Apply all ≥ X".
- Header: queue status (queued, running, failed counts) and a "retry failed" button. Add a nav link with the pending count.

## Verification
- Unit tests for the scoring, the MBID extraction from tags (JS fixtures run in `node --test`, if available; otherwise test the Go reader), ApplyMatch, and the job queue.
- **Manual check:** with `AUDIOLOADER_CONTACT` set, tag 3 Picard-tagged files (instant), 3 untagged files (AcoustID), and 1 obscure file (goes to review). The log shows ≤ 1 MusicBrainz request per second.

## Out of scope
Writing tags into files, creating releases from MusicBrainz data (plan 2), artist discographies (plan 2).
