# Plan 41: Sync Apple Music playlists

**Goal:** follow a public Apple Music playlist (for example a shared Favourite Songs) and download what gets added to it, without pasting the link again.

**Depends on:** plan 36 (Apple Music import), plan 2 (scheduled checks).

## Status

- [x] 1. Followed playlists in the database
- [ ] 2. The check job and its timer
- [ ] 3. "Keep in sync" and the followed list
- [ ] 4. Songs to pick in the Inbox

## Rules

As in plan 36: `make build`, `go vet ./...` and `make test` pass; one commit per step, app runnable after each; no attribution lines; schema only through the migration list; Apple Music calls only through `internal/applemusic` (its limiter), MusicBrainz through `internal/mb`; fake `httptest` servers in tests. Never run anything on port 8080. Normal mode only.

## Decisions

- **Open questions, decided:**
  - Favourite Songs can be shared as a public playlist link (plan 36 imported one), so no Apple API, MusicKit token or developer account is needed. A link that stops being public marks the follow "Not public any more" and keeps it.
  - Plan 36 found a user playlist page embeds every song (244 of 244). If the parsed count is below `trackCount`, the follow shows "Apple Music shows the first N of M songs" and still syncs what it sees. No paging.
  - New songs are found by Apple song ID, not position, so it doesn't matter whether Apple adds at the top or bottom.
- **Tables** (one migration): `apple_follows(id, url, name, import_id, last_checked_at, last_error, created_at)` and `apple_follow_songs(follow_id, apple_song_id, PRIMARY KEY(follow_id, apple_song_id))` — the songs already seen. `apple_import_tracks` gets `apple_song_id TEXT` (the import stores it from now on).
- **Follow** from a finished import page (`/add/apple/<id>`): "Keep in sync" saves the follow, linked to that import, and seeds the seen set with the import's songs. Following the same URL twice reuses the follow.
- **Check job** `apple_sync` in the non-download lane, one per follow (dedupe key = follow ID). `appleimport.Schedule(ctx)` mirrors `tracking.Schedule`: every minute, enqueue follows whose `last_checked_at` is older than `SyncEvery`. `SyncEvery` comes from `AUDIOLOADER_APPLE_SYNC_EVERY` (Go duration, default `1h`), read in `main.go` like the other env settings.
- **New songs** are appended to the follow's import as new `apple_import_tracks` rows (positions continue), matched with the existing matcher, then:
  - a confident match ("Best match") is added as a placeholder with a queued download, using the same per-track helper as "Add selected" — move that helper from `internal/api/appleimport.go` into `internal/appleimport` so the job can call it — and appended to the audioloader playlist if the import made one;
  - unsure and not-found songs stay on the import page to pick, as today.
  - All new IDs go into the seen set in the same transaction.
- **Removals** do nothing: a song removed from the Apple playlist stays in the library and the playlist.
- **Followed list** on the Add page under recent imports: name, last check ("5 min ago" or the error), songs to pick count, "Check now", "Stop syncing" (deletes the follow and its seen set, not the import).
- **Inbox:** a section "Apple Music: N songs to pick" per follow with unsure or not-found songs, linking to its import page; counted in the Inbox badge.

## Steps

### 1. Followed playlists in the database
- Migration, `internal/db/applefollow.go` with tests; store `apple_song_id` on import tracks.

### 2. The check job and its timer
- `internal/appleimport/sync.go`: job, `Schedule`, move the add helper; `main.go` env setting and `go appleimport.Schedule(ctx)`. Tests with fake Apple and MB servers: new confident song added and in the playlist, unsure stays, seen songs skipped, removal ignored, unreadable page sets the error.

### 3. "Keep in sync" and the followed list
- Routes, `appleimport.templ` button, Add page list, Check now / Stop syncing. API tests.

### 4. Songs to pick in the Inbox
- Inbox section and badge count. Screenshots at 1440×900 and 390×844 in the scratchpad. `FEATURES.md`; remove the request from `REQUESTS.md`.
