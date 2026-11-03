# Plan 32: Queue whole releases and playlists

**Goal:** add a whole album, EP, playlist or artist's tracks to the queue, and make "Play next" behave as people expect.

**Depends on:** plan 24 (queue panel), plan 31 (repeat, both touch `player.js`).

## Status

- [x] 1. Play next keeps order
- [x] 2. Queue a release or a playlist (web)
- [x] 3. Mobile app

## Rules

As in plan 31.

## Decisions

- **Play next keeps order:** today each "Play next" is inserted right after the current track, so three in a row play in reverse. `player.js` keeps a `nextAt` counter: the first "Play next" goes to `pos + 1`, the next one after it, and so on; it resets when the current track changes. "Add to queue" still appends to the end.
- **Batches:** `playNext(tracks)` and `enqueue(tracks)` take an array (one track is still accepted). With nothing playing, both start the batch as `playList` does.
- **Where:** next to every `data-play-list` button (release pages, playlist pages, artist page sections), a ⋯ menu with "Play next" and "Add to queue", using `data-queue-list="<list id>"` and the same `tracksIn(list)` as Play. Placeholder tracks are skipped, as Play skips them.
- **Feedback:** a short toast "Added 11 tracks to the queue" (reuse an existing toast if there is one; otherwise a small `aria-live` line in the queue panel header).
- **Shuffle on:** a batch added with "Add to queue" is appended in its own order; shuffle only reorders on toggle, as today.

## Steps

### 1. Play next keeps order
- `nextAt` logic, ideally as a pure helper in `player-state.js` with node tests (insert position from `pos` and `nextAt`).

### 2. Queue a release or a playlist (web)
- Menu markup in the release, playlist and artist templ files; `player.js` handler.
- Headless Firefox screenshot of a release page with the menu open, in the scratchpad.
- `FEATURES.md`, Queue. `REQUESTS.md`: remove the queue entry.

### 3. Mobile app
- "Play next" / "Add to queue" for a release and a playlist (long-press or ⋯ on the screen header), same order rule in `queue.ts` with jest tests. `FEATURES.md` mobile section.
