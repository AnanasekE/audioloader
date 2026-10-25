# Plan 3: Playback and discovery

**Goal:** a play queue (next/previous, shuffle, play release), library search, and playlists.

**Depends on:** plan 0 (placeholder tracks must never be queued).

## Steps

### 1. Queue in `assets/js/player.js`
- **State:** `queue` (array of track objects in the same shape as `current`), `index`, `shuffle` (bool), `order` (shuffled index list). Persist all of it in the existing localStorage entry.
- **API on `window.player`:**
  - `playList(tracks, startIndex)`
  - `enqueue(track)`
  - `playNext(track)`
  - `next()`, `prev()`
  - `toggleShuffle()`
- `ended` calls `next()`; at the end of the queue it stops. `prev()` restarts the track if `currentTime > 3`, otherwise goes to the previous one.
- **Shuffle:** a Fisher–Yates order with the current track first. Turning shuffle off goes back to the original order at the current track.
- **Media Session:** add the `nexttrack` and `previoustrack` handlers.
- **Player bar UI** in `layout.templ`: prev and next buttons, a shuffle toggle, and a queue popover listing upcoming tracks with remove and clear controls.
- **Restoring:** if a stored queue entry 404s on load, skip to the next one instead of clearing the whole player (this changes the current `error` handler).

### 2. Play release and play from list
- **Release page:** a "Play" and a "Shuffle" button. Render the tracklist as JSON in a `<script type="application/json">` tag, built from the same data as the rows. Leave out tracks with `has_file = 0`.
- **Track rows:** clicking play in a list (tracks page, release page) calls `playList(visibleRows, i)`, so the rest of the list continues. Add "Play next" and "Add to queue" to a row menu.
- Test manually: play an album, skip, reload mid-track, and the queue and position come back.

### 3. Search
- **First check FTS5 works:** `CREATE VIRTUAL TABLE t USING fts5(x)` on modernc sqlite. If it fails, fall back to `LIKE` on normalized columns and skip the triggers.
- **Migration:** `CREATE VIRTUAL TABLE search USING fts5(kind UNINDEXED, ref UNINDEXED, title, artists, tokenize='unicode61 remove_diacritics 2')`. Fill it with a rebuild function, `db.RebuildSearch()`, called after each write that changes titles or artists. A full rebuild is cheap at this library size and simpler than triggers across map tables.
- Exclude `has_file = 0` tracks. Plan 5 adds the secret filter here.
- **Query:** prefix matching on the last term (`foo bar*`), ranked with `bm25`, results grouped by kind.
- **UI:** a search box in the nav that does `hx-get="/search?q=" hx-trigger="input changed delay:200ms"`, with results in a dropdown and a full `/search` page.
- Test: diacritics ("Bjork" finds "Björk"), prefixes, and grouping.

### 4. Playlists
- **Migration:**
  ```sql
  CREATE TABLE playlists (uuid TEXT PRIMARY KEY, name TEXT NOT NULL, img_url TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')));
  CREATE TABLE playlist_tracks (
    playlist_id TEXT NOT NULL REFERENCES playlists(uuid) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    track_id TEXT NOT NULL REFERENCES tracks(uuid) ON DELETE CASCADE,
    PRIMARY KEY (playlist_id, position)
  );
  ```
  A track may appear more than once in a playlist, so `track_id` is not unique.
- Use the existing `audio.Playlist` type.
- **Pages:**
  - `/playlists` (list + create)
  - `/playlists/{id}` (play and shuffle like a release)
  - `/playlists/{id}/edit`: reuse the `release-edit.js` reorder pattern, without discs.
- **"Add to playlist"** in the track row menu: `POST /ui/playlists/{id}/tracks` appends at `MAX(position)+1`.
- **Saving the order:** delete and reinsert in one transaction, the same way `SetReleaseTracks` does.
- Test: reorder, duplicate tracks, and a deleted track disappearing from the playlist.

## Out of scope
A Preact island. Reconsider only if `player.js` grows past about 400 lines or the queue UI gets hard to keep in sync.
