# Plan 40: Move YouTube imports onto their MusicBrainz release

**Goal:** when a YouTube import's applied MusicBrainz match puts its recording on a different release (often the album rather than the single made at import), the Tagging page offers "Move to <release>", and the track moves only when the user accepts.

**Depends on:** plan 1 (tagging), plan 15 (remove a release), plan 23 (releases from MusicBrainz).

## Status

- [x] 1. Find tracks whose match is on another release
- [ ] 2. Move a track to its MusicBrainz release
- [ ] 3. "Move to" on the Tagging page

## Rules

As in plan 19: `make build`, `go vet ./...` and `make test` pass; one commit per step, app runnable after each; no attribution lines; schema changes only through the migration list; MusicBrainz calls through `internal/mb`, `httptest` fakes in tests. Never run anything on port 8080.

## Decisions

- **Offer, never overwrite.** No automatic move, and no setting for one: the "never overwrites what you set" rule stays. (The REQUESTS open question is decided: no auto-move above 90%.)
- **Which tracks:** tracks with a file whose source is YouTube (video or playlist import), with an applied match whose release MBID differs from the track's current release MBID (or whose current release has none). One DB query `MoveSuggestions()` returns track, current release (title, track count, created-by-import flag), target release MBID and title, recording MBID. A dismissed suggestion is stored (migration: `tracks.move_dismissed_release TEXT`) and not shown again for the same target.
- **"Created by the import"**: a release is import-made when it has no MusicBrainz release ID and exactly the one track — or use an existing marker if the import already sets one (check `internal/api/add.go`/`internal/sources`; prefer an explicit flag column if none exists: migration `releases.created_by_import INTEGER NOT NULL DEFAULT 0`, set by the YouTube import from now on, and backfilled for one-track releases without an MBID whose track's source is YouTube).
- **The move** (`db.MoveTrackToRelease` in one transaction, called from a small `internal/tagging` function that does the MB part):
  - Target release reused if the library has its MBID; else created from `mb.GetRelease` as plan 23 does (artists, IDs, cover). Its other tracks are not added as placeholders — the release page already shows unowned tracks as missing.
  - If the library already has a placeholder for that recording on the target, the file moves onto the placeholder and the moved track row is deleted (playlists, plays and tags re-pointed); else the track's release, disc and position are set from the MB tracklist.
  - The old release is deleted if it is now empty and import-made (plan 15's delete, without deleting files).
  - Tags in the file are rewritten as when a match is applied.
- **Tagging page:** a section "On another release" above the matches, one row per suggestion: track, "Single – X" → "Album – Y (2019)", buttons "Move" and "Keep here" (dismiss). Also a "Move to <release>" item in the track's ⋯ menu when it has a suggestion.

## Steps

### 1. Find tracks whose match is on another release
- Migration(s), `MoveSuggestions`, dismissal; DB tests (suggested, same release not, dismissed not, non-YouTube not).

### 2. Move a track
- `db.MoveTrackToRelease`, `tagging.Move(ctx, mbc, trackID)`; tests: new release created, existing reused, placeholder merged, old import release deleted, old release with other tracks kept.

### 3. "Move to" on the Tagging page
- Routes `POST /ui/tagging/{track_id}/move` and `/move/dismiss`, `tagging.templ` section, track menu item. API tests. Screenshot at 1440×900 and 390×844 in the scratchpad. `FEATURES.md`; remove the request from `REQUESTS.md`.
