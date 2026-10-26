# Plan 10: Inbox organization

**Goal:** the inbox stays usable with hundreds of releases from many artists.

**Covers (REQUESTS.md):** Better inbox organization.

**Depends on:** plan 2. Independent of plans 6–9.

## Decisions (the request leaves these open; this plan takes all six ideas)
- Compact rows by default; the cover and tracklist open on demand.
- Grouped by artist, collapsible, with counts. Sorting and filters apply inside the groups.
- State lives in the query string (`?tab=new&type=album&owned=none&q=...&sort=date`), so a filtered view can be reloaded or bookmarked. No schema change.

## Steps

### 1. Owned counts
- `db.ListInbox` computes "owned of total" per release from the stored tracklist JSON (`owned_track_id`). Ownership isn't rechecked on page load: the daily check and "Check now" refresh it.
- Test: counts for none, some and all owned.

### 2. Compact rows
- One row per release: small cover, title, type, date, "3 of 11 in library", Confirm and Ignore.
- "Show tracklist" loads the existing card body with `hx-get` (`GET /ui/inbox/{release_group}`).

### 3. Tabs and un-ignore
- Tabs "New" and "Ignored", with counts. Ignored rows get "Move back to New" (`POST /ui/inbox/{release_group}/restore`, sets `status = 'new'`).
- Test: restore works and the daily check doesn't duplicate the row.

### 4. Group by artist
- Group rows by the release's artist, sorted by name, each group a `<details>` with the artist's name and count. Remember collapsed groups in `localStorage` (wrapped in try/catch).
- "Confirm all" per group, next to the global one.

### 5. Filter, sort and search
- Filters: type (album / EP / single / compilation), "nothing owned yet". Sort: newest or oldest first.
- Search box filters by release title or artist name (`LIKE` on the inbox's own columns is enough at this size; no FTS).
- The form submits with `hx-get` and `hx-push-url`, replacing the list only.

## Verification
- Tests above.
- **Manual check:** follow three artists, filter to albums with nothing owned, collapse one artist, ignore a release and restore it.

## Out of scope
Notifications, inbox entries for artists you don't follow.
