# Plan 42: Phone player and queue, Apple Music style

**Goal:** on phones (below 720px) the web player works like Apple Music's: a floating mini player above the tab bar, tapped open into a full-screen Now Playing sheet with big artwork, and the queue ("Playing Next") inside that sheet with reordering and swipe to remove. Replaces the small ☰ popover (REQUESTS "Queue on phones").

**Depends on:** plan 17 (Console look), plan 24 (queue panel), plan 31 (repeat), plan 34 (autoplay), plan 37 (track menu).

## Status

- [ ] 1. Floating mini player and translucent tab bar
- [ ] 2. Now Playing sheet
- [ ] 3. Playing Next in the sheet: reorder and swipe to remove
- [ ] 4. Remove the ☰ popover, polish, docs

## Rules

As in plan 19: `make build`, `go vet ./...` and `make test` pass; one commit per step, app runnable after each; no attribution lines in commits; plain JS in `assets/js`, no build step; pure logic in small modules with node tests (as `player-state.js` has); tests use `httptest` fakes. Never run anything on port 8080 (the user's own server). Desktop (720px and up) must look and behave as before, except where a step says so.

## Decisions

- **Only the phone layout changes.** Everything is `max-[720px]:` / `min-[720px]:` scoped. `player.js` stays the one owner of playback state; the sheet is more views of the same state (as the side panel is), not a second player.
- **Mini player** (step 1): a rounded card (`rounded-xl`, 8px side inset, shadow) floating 6px above the tab bar, translucent (`bg-panel/80 backdrop-blur-xl`). Content: cover (40px, `rounded-md`), title and artist on one line each, play/pause and next. No seek bar; a 2px progress line along its bottom edge. Tapping anywhere but the buttons opens the sheet. The tab bar also becomes translucent with blur, and page content gets enough bottom padding to scroll clear of both.
- **Now Playing sheet** (step 2): a `<dialog id="now-playing">` that covers the screen, slides up (200 ms, `prefers-reduced-motion` = no slide).
  - Background: a dark gradient tinted from the cover's average colour. `assets/js/cover-tint.js` draws the cover into a 16×16 canvas and averages it; the pure `tintFrom(r, g, b)` (darken and clamp saturation so white text stays readable, contrast ≥ 4.5:1 against white) is node-tested. Pixel-art placeholder covers tint too. Falls back to `bg-panel` when the image can't be read.
  - Top: a grab handle bar; tap or drag down closes. Drag-to-close uses pointer events; pure `shouldClose(dy, velocity, height)` (closes past 25% of the height, or on a fast flick) is node-tested.
  - Artwork: square, as wide as the screen minus 32px, `rounded-xl`, shadow; it shrinks slightly (scale 0.85) while paused, as Apple does.
  - Below: title (bold, links to the release), artists (links), and the ⋯ track menu on the right (the existing `trackMenu`, opened via `context-menu.js`).
  - Scrubber: full-width thin bar with elapsed and remaining time under it; thickens while dragged. Reuses `#player-seek` logic.
  - Controls: large prev, play/pause, next, spaced evenly. Podcasts show −15 / +30 and the speed select in place of prev/next row extras, and the chapter title under the artist.
  - Volume slider with speaker icons on both ends.
  - Bottom row of icon buttons: Sleep timer (opens the existing sleep menu), and Queue (toggles the queue view, highlighted when on).
  - Back: opening pushes `history.pushState({nowPlaying: true}, "", "#now-playing")`; the browser/phone back gesture closes it; closing by hand pops that entry. An hx-boost navigation (e.g. tapping the artist link) closes it first.
- **Playing Next** (step 3): the Queue button swaps the artwork area for the queue, with the current track shrunk to a row at the top (cover 56px, title, artist), as Apple does.
  - Pill toggles under it: Shuffle, Repeat (off / all / one, icon shows which), Autoplay. Same state as the existing buttons.
  - "Playing Next" heading with "Clear" on the right; then the upcoming tracks. Above them, scrolled out of view at first, a "History" list of what was played (tap to play again).
  - Each row has a drag handle (≡) on the right: pointer drag reorders. Pure `moveItem(list, from, to)` and the play-position adjustment `posAfterMove(pos, from, to)` go in `player-state.js` with node tests; the move is saved like every queue change.
  - Swipe a row left past 40% to remove it (red "Remove" shows behind it); plus a visible ✕ on long-press menu for accessibility. Pure `swipeAction(dx, width)` is node-tested.
  - Podcasts with chapters show "Chapters" in place of Playing Next while an episode plays, as the side panel does.
  - The wide-screen side panel gets the same drag handles (it renders from the same code); no swipe there.
- **Step 4:** the ☰ `<details id="player-queue">` popover is removed on phones (it stays where it is used between 720 and 1200px, where there is no side panel). `REQUESTS.md`: remove "Queue on phones". `FEATURES.md`: the phone player.

## Steps

### 1. Floating mini player and translucent tab bar
- `internal/components/layout.templ` (`player()`, `tabBar`), `styles/input.css` if needed, `player.js` (progress line, tap opens a stub sheet). Screenshot 390×844 and 1440×900 (desktop unchanged).

### 2. Now Playing sheet
- `layout.templ` (`nowPlaying()` dialog), `assets/js/now-playing.js` (open/close, drag, history), `assets/js/cover-tint.js`, `player.js` hooks (render title/artist/time/state into the sheet too). Node tests: `tintFrom`, `shouldClose`. Screenshots at 390×844: a music track playing, paused, a podcast episode.

### 3. Playing Next
- `player.js` `renderQueue` gains the sheet target, handles and swipe; `player-state.js` `moveItem`, `posAfterMove`, `swipeAction` with node tests. Screenshots at 390×844 (queue view, a row mid-swipe if practical) and 1440×900 (side panel with handles).

### 4. Remove the popover, polish, docs
- Remove the phone popover; check the tab bar, mini player and sheet in light and dark themes and in secret mode. Docs as above.
