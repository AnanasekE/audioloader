# Plan 26: One Add page, and covers from MusicBrainz

**Goal:**
1. Songs added from MusicBrainz (the Add page, plan 12 and 23) get their "Cover of" from MusicBrainz, as auto-tagged tracks do. Today "Bring Me to Life" (Little V. feat. Apricot) was added from its release link and shows no cover info, though MusicBrainz has it.
2. Upload and Add become one page: a single box takes search text or any supported link and sends it to the right place, with file upload on the same page.

**Depends on:** plans 9 (covers), 12, 14, 23 (Add page), 11 (secret source), 17/18 (layout and kit).

## Status

- [x] 1. Ask MusicBrainz for the cover link when adding
- [x] 2. Set the cover on added songs
- [x] 3. Backfill songs added before
- [ ] 4. One box that routes links
- [ ] 5. Merge the pages
- [ ] 6. Docs

## Rules

As in plan 19: one commit per step; `make build`, `go vet ./...` and `make test` pass; no attribution lines; never name the secret source site (tests use `secrettest`). MusicBrainz tests use `httptest` fakes.

## Decisions

### Covers
- **Why it's missing:** only auto-tagging (`tagging.fromRecording` → `CoverWorkID` → `JobCoverOf` → `SuggestCoverOf`) looks at the cover link. The Add path (`releaseSongs` / `queueSong` in `internal/api/addsong.go`) never does, and `mb.GetRelease`'s `inc` has `recording-level-rels` but not `work-rels`, so the recordings' `performance` relations to works aren't in the response at all.
- **Fix the lookup:** add `work-rels` to `GetRelease`'s `inc` (with `recording-level-rels` it returns each recording's work relations). Check `GetRecording` asks for `work-rels` too. Then `Recording.CoverWork()` works on tracks from a release.
- **Apply, don't suggest,** for songs added from MusicBrainz: the user picked that exact recording, so the cover info is as trusted as the recording itself. `queueSong` enqueues a new job `JobCoverOfApply` (same payload as `JobCoverOf`) when `CoverWork()` is set. The job finds the original the same way `SuggestCoverOf` does (factor that lookup into one function both use), then sets the cover as text (title, artists, MBID) through the existing `setCover` path, **or** links the library track when the library has a track with that recording MBID. It never overwrites a cover that is already set, and does nothing for tracks that are secret or skipped (`Skip`).
- **Backfill:** a one-off job, enqueued once at startup through a migration-tracked flag (or a `jobs` dedupe key, so it runs once). For every normal-mode track with a recording MBID, no cover set and no cover suggestion stored, it looks up the recording with `work-rels`. Covers become **suggestions** on the Tagging page (`SaveCoverSuggestion`), not applied, because those tracks may have been matched automatically. At 1 req/s it takes a few minutes for a few hundred tracks; it runs in the non-download lane.

### One Add page
- **One page, `/add`,** titled "Add". The sidebar "Upload" button becomes "Add" (same `upload` icon), the phone tab and More sheet entries too, and the Tracks page "Add song" button points here. `/upload` redirects to `/add` (keep query strings). `/upload/batch` and `/upload/import` stay as they are, linked from the page.
- **Layout, top to bottom:**
  1. **The box:** "Search, or paste a link". One text field and a button.
  2. **Results area** for whatever the box finds.
  3. **"Or upload a file"**: the existing upload forms (kind choice: track, release types, episode), unchanged, below a divider. Links to Batch mode and Import library.
- **Routing:** `POST /ui/add/lookup` with `q`. It classifies `q` and calls the existing handler logic, rendering that handler's result into the results area:

  | Input | Mode | Goes to |
  |---|---|---|
  | `musicbrainz.org/release/…`, `/recording/…` | normal | plan 23 listing (`searchSongsMusicBrainz`) |
  | other `musicbrainz.org` links | normal | its existing 400 message |
  | YouTube or YouTube Music video or playlist link (`youtube.com/watch`, `youtu.be`, `youtube.com/playlist`, `music.youtube.com/...`) | normal | the YouTube import preview (`previewYouTubeImport`) |
  | a link `secret.Default().Normalize` accepts | secret, with a source | secret source check (`checkSecretSource`) |
  | any other link | any | 400 "Audioloader can't add from that site." |
  | text | normal | MusicBrainz song search, then YouTube fallback (plan 12) |
  | text | secret | 400 "Paste a link from the secret source, or upload a file." (when a source is configured; otherwise "Upload a file.") |

  Move each handler's body into a function that takes the value instead of reading `r.FormValue`, so the old routes and the lookup share one code path. Keep the old `/ui/...` routes: the forms inside the results still post to them.
- **Never reveal the secret source in normal mode:** a secret source link in normal mode gets the same "can't add from that site" message as any unknown site. Add this to the leak test.
- **Old pages:** `/upload/youtube` and `/upload/secret-source` redirect to `/add?q=<url>` when they have a `url`, else to `/add`. `/add?q=…` fills the box and runs the lookup on load (`hx-trigger="load"` when `q` is set), so links from elsewhere (the uploader pages' "Download from URL" links, Search's link to Add) keep working. Update those links to `/add?q=` directly.
- **Secret mode:** the page exists in both modes (today `/add` is normal-only). In secret mode the box only takes secret source links, and the upload part shows what the old Upload page showed in secret mode.
- **Header text:** "Search MusicBrainz, paste a MusicBrainz, YouTube or YouTube Music link, or upload a file" in normal mode; in secret mode, "Paste a link, or upload a file" (no site named).

## Steps

### 1. Ask MusicBrainz for the cover link when adding (`internal/mb`)
- `work-rels` in `GetRelease` (and `GetRecording` if missing). Test with a fake release JSON whose track's recording has a `performance` relation with the `cover` attribute: `CoverWork()` returns the work.

### 2. Set the cover on added songs (`internal/tagging`, `internal/api/addsong.go`)
- Shared original lookup, `JobCoverOfApply`, enqueued from `queueSong`.
- Tests: adding a cover from a release link sets "Cover of" as text with the original's MBID; when the original is in the library, it links that track; an existing cover is never overwritten; a non-cover gets nothing; Add all on a release with one cover and one original sets only the cover.

### 3. Backfill songs added before
- The one-off job as decided. Tests: a track with a cover recording gets a suggestion; one with a cover set or a stored suggestion is skipped; it runs once.

### 4. One box that routes links (`internal/api/add.go`)
- `classifyAddInput(v, q)` with table tests for every row above, including uppercase hosts, `www.`/`m.` subdomains, trailing junk, and a secret source link in normal mode.
- `POST /ui/add/lookup` dispatching to the shared functions. API tests per row.

### 5. Merge the pages (`internal/components`, `internal/api`, `routes.go`)
- New `AddPage`, redirects, nav renames, `?q=` on load, links updated.
- Leak test: `/add` in both modes, and the lookup with a secret source link in normal mode.
- Headless Firefox screenshots of `/add` at 1440×900 and 390×844, in normal and secret mode, plus one with a MusicBrainz release link's results. Save in the scratchpad.

### 6. Docs
- `FEATURES.md`: replace the separate "Add a song" and upload entries with the one Add page and its routing table, and add the covers line ("songs added from MusicBrainz get 'Cover of' from MusicBrainz").
- `docs/plans/README.md`: row 26, status "plans 0–26".
