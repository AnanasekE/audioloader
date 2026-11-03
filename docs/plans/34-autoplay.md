# Plan 34: Autoplay

**Goal:** when the queue runs out, keep playing similar tracks from the library.

**Depends on:** plans 31 and 32 (`player.js` queue changes).

## Status

- [x] 1. Suggestions endpoint
- [ ] 2. Autoplay in the web player
- [ ] 3. Mobile app

## Rules

As in plan 31. SQL in `internal/db`, handler in `internal/api`, all reads through the central track filter.

## Decisions

- **Library only:** no external service. Suggestions are owned tracks (`has_file = 1`) visible to the viewer (secret mode gets secret tracks, normal mode normal ones).
- **Scoring**, from the last 5 played tracks (the seeds): same artist +3; an artist who shares a release with a seed artist +2; same release type and a release year within 5 years +1; for secret tracks, each shared tag +2 and same creator +3. Ties and the rest break at random, so it isn't the same list every time. Tracks played in the last 50 (sent by the player) and the seeds are excluded. When fewer than N score above 0, random owned tracks fill the rest.
- **Endpoint:** `GET /api/v1/autoplay?seed=<id>&seed=…&exclude=<id>&n=10`, JSON in the `apiv1` track shape, so the mobile app uses the same call. Episodes are never suggested, and an episode seed gives no suggestions.
- **Web player:** an "Autoplay" switch in the queue panel header, on by default, saved in the player state. When `next()` reaches the end (repeat off) and autoplay is on, the player fetches 10, appends them and plays on. Autoplay tracks show under an "Autoplay" divider in the queue and can be removed like any upcoming track. Adding anything to the queue puts it before the autoplay tracks. A failed fetch just stops, as today.
- **Recent history** for `exclude`: the last 50 played IDs, kept in the player state (not on the server).

## Steps

### 1. Suggestions endpoint
- `db.AutoplaySuggestions(viewer, seeds, exclude, n)` with tests on a test DB (scoring, mode separation, no placeholders, no episodes, fill with random); the handler and its test; `docs/api.md`.

### 2. Autoplay in the web player
- Switch, divider, fetch on queue end, history in the saved state (helpers in `player-state.js` with node tests).
- Headless Firefox screenshot of the queue panel with autoplay tracks, in the scratchpad.
- `FEATURES.md`, Playback. `REQUESTS.md`: remove the Autoplay entry.

### 3. Mobile app
- Same switch in the queue screen, same endpoint when the queue ends. `FEATURES.md` mobile section.
