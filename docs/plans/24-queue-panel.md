# Plan 24: Queue panel on the right (desktop)

**Goal:** on wide screens the queue is always visible in a column on the right of every page, instead of hiding behind the ☰ popover in the player bar. Phones and narrow windows keep the popover as it is; how the queue should look on phones is still open (REQUESTS.md).

**Depends on:** plan 17/18 (Console layout, player bar).

## Status

- [x] 1. The panel in the layout
- [ ] 2. Render the queue into it
- [ ] 3. Docs

## Rules

As in plan 19: one commit per step; `make build`, `go vet ./...` and `make test` pass; no attribution lines. Plain JS in `assets/js`, no build step; Tailwind classes as the rest of `layout.templ`.

## Decisions

- **Breakpoint `min-[1200px]`.** At 1200px and wider the layout grid becomes `232px | minmax(0,1fr) | 300px`, and the ☰ button and its popover are hidden. Between 720px and 1199px nothing changes. Under 720px nothing changes.
- **Always shown** at that width, on every page, in both modes, even when nothing is playing ("Nothing playing" and "Nothing queued"). No toggle.
- **The panel is `<aside id="queue-panel" hx-preserve>`**, next to `#player` in the layout, so htmx page swaps never rebuild it, as with the player. It is `sticky top-0 h-dvh overflow-y-auto`, with bottom padding for the 64px player bar. The player bar's right edge stays at the screen edge (it runs under the panel's padded bottom). Check the panel scrolls on its own and the page doesn't get a horizontal scrollbar at 1200px.
- **Contents, top to bottom:**
  1. Now playing: cover, title and artists of the current track.
  2. "Up next" with Clear. Each row shows the title and artists, a ✕ to remove it, and a click on the row plays that track now (jump: the tracks before it in the play order stay in history, as if Next was pressed that many times).
  3. Chapters, when the current track has them, as in the popover.
- **One render path.** `renderQueue` and `renderChapters` in `player.js` render into every container marked `data-queue-list` / `data-chapter-list` (the popover's and the panel's), instead of one element by ID. The same for the empty-state texts and the Clear buttons. No second copy of the queue logic.
- **Jump** is a new `playAt(p)` in `player.js` for a play-order position `p > pos`. It sets `pos = p`, loads that track, saves state and re-renders. The popover rows get the same click.
- **Secret mode** has nothing new to hide. The panel shows only what the player already holds, and the player is reset on mode change.

## Steps

### 1. The panel in the layout (`internal/components/layout.templ`)
- Add the third grid column and the `queue-panel` aside with the structure above and empty lists. Hide `#player-queue` at `min-[1200px]`.
- Templ test or API test: the layout renders `#queue-panel` with `hx-preserve` on a normal page and a secret-mode page.

### 2. Render the queue into it (`assets/js/player.js`)
- Multi-container rendering, now playing, `playAt`, row click in both places.
- If the repo has JS tests for the player (`make test` runs node tests), add tests for `playAt` and the multi-container render. Otherwise keep the logic small and covered by a manual check.
- Manual check: `make build`, run the app, and take headless Firefox screenshots at 1440×900, 1000×800 and 390×844 with a few tracks queued. Confirm the panel appears only at 1440, the popover still works at the other two, and a page change (hx-boost) keeps the panel and its scroll position. Save the screenshots in the scratchpad and report their paths.

### 3. Docs
- `FEATURES.md`, Playback: "On wide screens the queue is always shown on the right: now playing, up next (click to play, ✕ to remove) and chapters".
- `REQUESTS.md`: add "Queue on phones: the ☰ popover is small; decide whether it becomes a full-screen sheet or part of an expanded player".
- `docs/plans/README.md`: row 24, and update the status line.
