# Plan 15: Remove a release I don't want

**Goal:** one "Remove release" action that deletes a release together with its tracks (and their files and queued downloads), keeps tracks that are also on another release, and marks the release group "ignored" in the inbox so the daily check doesn't bring it back.

**Covers (REQUESTS.md):** Remove a release I don't want.

**Depends on:** plan 2 (inbox, placeholders, downloads), plan 7 (`DeleteTrack`), plan 10 (Ignored tab, "Move back to New").

## Decisions
- **Normal mode only.** The route uses `handleNormal`, like every release route. Track reads use `v.trackCond`, so a secret track on the release is never deleted or counted: it only loses its slot (the `release_tracks` cascade).
- **Which tracks go:** the release's tracks visible to the viewer that are on no other release. Each uploaded track has its own single, so removing an album normally only deletes its placeholders and tracks that came with it; that's what "tracks that are also on another release stay" means.
- **Same cleanup as `DeleteTrack`.** Track rows cascade to downloads, tagging, playlist and position rows. Queued jobs that mention a removed track or the release are dropped (same `instr(payload, ?)` rule). Files, track images and the release cover go after the commit, so a failed remove never loses audio.
- **Inbox:** with a release group MBID, the inbox row becomes `ignored` (from `new` or `confirmed`). With no row (the release came from an upload or "Add a song"), an `ignored` row is inserted from the release itself, because `tracking.Check` would otherwise list it as new on its next run. "Move back to New" then works as for any ignored release. No MBID: nothing is written.
- **Confirm text** is computed in the template from `GetRelease`'s data (`t.Releases` is already attached to each track), with the same rule as the SQL: "Remove Mezzanine? 3 tracks go, 2 of them with files, which are deleted. 1 track on another release stays."
- **The old "Delete release" (keep tracks) stays** on the edit page, relabelled, for fixing a wrong release by hand.
- **No schema change.**

## Steps

### 1. Ignore from "confirmed" too (`internal/db/inbox.go`)
- `IgnoreInboxRelease`: `WHERE release_group_mbid = ? AND status IN ('new', 'confirmed')`. Update its comment.
- **Test** (`internal/db/inbox_test.go`, extend `TestRestoreInboxRelease` or a new `TestIgnoreConfirmed`): confirm a release (`ConfirmInboxRelease`), ignore it, `InboxCounts` gives `0, 1`; `RestoreInboxRelease` brings it back as `new`.

### 2. `RemoveRelease` (`internal/db/releases.go`)
- Split the post-commit part of `DeleteTrack` into a helper in `tracks.go` and call it from `DeleteTrack`:
  ```go
  // removeTrackFiles deletes a removed track's audio file and images.
  func removeTrackFiles(id uuid.UUID)
  ```
  and the jobs line into
  ```go
  // dropQueuedJobsTx removes queued jobs whose payload mentions id.
  func dropQueuedJobsTx(tx *sql.Tx, id string) error
  ```
- Add:
  ```go
  // ReleaseRemoval is what removing a release takes with it.
  type ReleaseRemoval struct {
      Tracks    int // deleted: visible, and on no other release
      WithFiles int // of those, how many had a file
  }

  // RemoveRelease deletes a release, its tracks that are on no other
  // release (with their files and queued jobs), and marks its release group
  // ignored in the inbox. coverArtURL is the Cover Art Archive base URL
  // (mb.Client.CoverArtURL), for an inbox row it has to create. Secret
  // tracks a normal viewer can't see only lose their slot.
  func RemoveRelease(v Viewer, id, coverArtURL string) (ReleaseRemoval, error)
  ```
- Steps (2–4 in one transaction):
  1. `GetRelease(v, id)` first (before `Begin`) for the title, type, date, artists, release group MBID and tracklist; `ErrNotFound` passes through.
  2. Select the tracks to delete:
     ```sql
     SELECT t.uuid, t.has_file FROM release_tracks rt JOIN tracks t ON t.uuid = rt.track_id
     WHERE rt.release_id = ? AND <v.trackCond("t")>
       AND NOT EXISTS (SELECT 1 FROM release_tracks o WHERE o.track_id = t.uuid AND o.release_id <> rt.release_id);
     ```
  3. For each: `DELETE FROM tracks WHERE uuid = ?`, `dropQueuedJobsTx(tx, trackID)`. Then `dropQueuedJobsTx(tx, id)` (cover jobs) and `DELETE FROM releases WHERE uuid = ?`.
  4. Inbox, if `ReleaseGroupMBID != ""`: `UPDATE inbox_releases SET status = 'ignored' WHERE release_group_mbid = ?`. If no row changed and the release has an artist, `INSERT OR IGNORE INTO inbox_releases(release_group_mbid, artist_id, release_mbid, title, type, date, data, status) VALUES(..., 'ignored')` with `artist_id` = first release artist, and `InboxData{CoverURL: coverArtURL + "/release-group/" + rg + "/front-250", Artists: ..., Tracks: ...}` (the cover format `tracking.Check` stores; artists as `MatchArtist{ID: a.MBID, Name: a.Name}`; tracks built from the release's tracks (disc, position, title, `RecordingMBID: t.MBID`, ISRC, `LengthMs`, `OwnedTrackID` for tracks that stay)). Marshal with `json.Marshal` as `AddInboxRelease` does.
  5. Commit, then `removeTrackFiles` for each deleted track and `removeCover(releaseUUID)`.
- **Tests** (`internal/db/releases_test.go`, `TestRemoveRelease`):
  - A release with: a placeholder (from `ConfirmInboxRelease`), a track with a file only on this release (`AddTrack` with a `ReleaseLink`), and a track also on its own single (`addTestTrack` + `SetReleaseTracks`). Queue a `download` job for the placeholder (`jobs.EnqueueAt` or a raw insert).
  - `RemoveRelease` returns `{Tracks: 2, WithFiles: 1}`; release gone; the two tracks gone and the file deleted (`os.Stat`); the single's track and its single stay; no queued job mentions the placeholder; the inbox row is `ignored`.
  - A release with a `release_group_mbid` and no inbox row (set it with a raw `UPDATE`): afterwards `InboxKnows` is true and the row is `ignored`.
  - A secret track on the release survives `RemoveRelease(Normal, ...)` and is no longer on it.
  - Unknown ID gives `ErrNotFound`.

### 3. Route, handler and buttons
- **Route** (`internal/api/routes.go`): `handleNormal(mux, "POST /ui/releases/{release_id}/remove", removeRelease)`, next to `/delete`.
- **Handler** (`internal/api/releases.go`), modelled on `deleteRelease`:
  ```go
  func removeRelease(w http.ResponseWriter, r *http.Request)
  ```
  `pathUUID` + `visibleRelease`, then `db.RemoveRelease(viewer(r), id.String(), mb.Default().CoverArtURL)`. `ErrNotFound` → 404, other errors → `serverError`, success → `hxLocation(w, "/releases")`.
- **Components** (`internal/components/releases.templ`):
  - `func removeReleaseConfirm(r *audio.Release) string`: counts tracks with `len(rt.Track.Releases) <= 1` and how many of them have `HasFile`, plus those that stay. Wording: "Remove <title>? N tracks go, M of them with files, which are deleted." + " K tracks on other releases stay." when K > 0. Handle singular ("1 track goes"). With N = 0: "Remove <title>? Its tracks are all on other releases and stay."
  - `templ removeReleaseButton(r *audio.Release)`: `<button type="button" class="btn" hx-post={"/ui/releases/"+id+"/remove"} hx-confirm={ removeReleaseConfirm(r) }>Remove release</button>`.
  - `releaseActions`: add `@removeReleaseButton(r)` after "Edit release".
  - `ReleaseEditPage`: the "Delete release" section becomes "Remove release" with the text "Deletes the release and its tracks that aren't on another release, with their files. It moves to Ignored in the inbox." and `@removeReleaseButton(r)`; below it the old button relabelled "Delete the release only" with the existing confirm ("Its tracks stay in your library.").
- **Tests** (`internal/api/releases_test.go`, new; use `dbtest.New`, `Register(mux, nil)`, `WithViewer`, the `do` helper and `placeholder(t)` from `downloads_test.go`):
  - `GET /releases/<id>` contains "Remove release" and the confirm text "1 track goes" for the `placeholder` release.
  - `POST /ui/releases/<id>/remove` → 204 with `HX-Location` to `/releases`; the placeholder is gone (`db.GetTrack` → `ErrNotFound`), `db.GetDownload` → not found; `GET /inbox?tab=ignored` lists "Mezzanine".
  - A second POST → 404.
  - Secret mode: covered by the `NormalOnly` loop in `leak_test.go`; run it.
- `make generate` (templ) before `make build`.

### 4. Docs
- `FEATURES.md` (Library): change "delete a release (its tracks stay)" to also mention "Remove release" on the release and edit pages: deletes the release with its tracks that are on no other release (files and queued downloads too, after a confirm with the counts), and moves its release group to Ignored in the inbox. Under Artist release tracking, note that "Don't ask again" also works on confirmed releases (used by Remove).
- `docs/plans/README.md`: add the row `| 15 | [Remove a release](15-remove-release.md) | Remove a release I don't want (REQUESTS.md) |` and update the status line once implemented.
- `REQUESTS.md`: remove the "Remove a release I don't want" section.

## Verification
- `make build`, `go vet ./...` and `make test` pass after each step.
- **Manual check:** confirm an inbox release with two missing tracks and upload one of them by hand; on its page press "Remove release": the dialog says "2 tracks go, 1 of them with files". After confirming, the release, both tracks, the file and the queued download are gone, the Downloads page doesn't list it, and it's under Inbox → Ignored. "Move back to New" then "Confirm" brings it back as placeholders. Remove an album whose songs are also singles: only the album goes.

## Out of scope
Removing several releases at once, an undo that restores files, removing a release from the artist page.
