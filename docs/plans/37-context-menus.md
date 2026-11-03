# Plan 37: Context menus, actions and a UX pass

**Goal:** every action for a thing is reachable from the thing itself: right-click (or long-press, or the keyboard) on a track, release, artist or playlist opens its menu, and ⌘K can run actions, not only jump to pages. A UX audit finds the places where it isn't clear where to go, and the cheap fixes are made. Stays on templ + htmx (plan 35 is the larger alternative and stays on hold).

**Depends on:** plan 18 (component kit), plan 32 (queue a whole list).

## Status

- [x] 1. Context menu on track rows
- [x] 2. Menus for releases, artists and playlists
- [x] 3. Actions in ⌘K, and a shortcuts dialog
- [x] 4. UX audit and quick fixes

## Rules

As in plan 19: `make build`, `go vet ./...` and `make test` pass; one commit per step; no attribution lines; plain JS in `assets/js`, no build step. Pure logic in small modules with node tests, as `player-state.js` has. Don't run anything on port 8080 (the user's own server runs there).

## Decisions

- **One menu, many ways in.** The server-rendered ⋯ menu stays the single source of a thing's actions. A new `assets/js/context-menu.js` opens that same `<details data-menu>` for the element under the pointer, so htmx attributes keep working and nothing is duplicated. Hosts are marked `data-menu-host` (track rows, release cards and rows, artist cards, playlist cards, and the header of release, artist and playlist pages).
- **Ways in:**
  - Right-click on a host opens its menu at the pointer (fixed position, flipped to stay inside the viewport; `placeMenu(x, y, w, h, vw, vh)` is pure and node-tested).
  - Shift+right-click, and right-click inside inputs, text selections or on `<audio>`, keep the browser's menu.
  - Long-press (500 ms, cancelled by 10 px of movement or scroll) on touch opens it; below 720px the menu shows as a bottom sheet with a backdrop, like the phone "More" sheet.
  - The ContextMenu key or Shift+F10 on a focused host opens it at the host; arrow keys move through items, Esc closes and returns focus.
  - Only one menu is open at a time; a click outside, Esc, scroll or an hx-boost navigation closes it.
- **Track rows** get the row itself focusable (`tabindex="0"`, roving is not needed) so the keyboard way works.
- **Menus for releases, artists and playlists** (new, built like `trackMenu`):
  - Release: Play, Shuffle, Play next, Add to queue, Edit, Edit tracklist, Remove release… (normal mode).
  - Artist: Play all, Play next, Add to queue, Edit / rename, Follow (or Following ✓), Link to MusicBrainz (when it has no ID).
  - Playlist: Play, Shuffle, Play next, Add to queue, Edit, Delete….
  - On cards, where the tracks are not on the page, queue actions fetch `/api/v1/releases/{id}`, `/api/v1/playlists/{id}` or `/api/v1/artists/{id}` and turn the API tracks into the player's track shape with one tested converter (`trackFromAPI`). Placeholders are skipped as Play skips them. An artist's "Play all" is its releases' tracks in release-date order plus its loose tracks.
- **⌘K actions.** The search box also lists commands, filtered in the browser (word-prefix match) above the server's results under an "Actions" heading; typing `>` first shows commands only. Commands: Add / upload, Go to each sidebar page, Check followed artists now, Shuffle on/off, Repeat mode, Autoplay on/off, Sleep 30 min / off, Dark / light theme, Lock the vault (secret mode). Each is `{id, label, keywords, when(), run()}` in one list in `assets/js/commands.js`; the matching function is node-tested. Enter runs the highlighted one.
- **Shortcuts dialog:** `?` outside a field opens a small `<dialog>` listing the keys (⌘K, /, space, m, ?, Shift+F10) and the right-click / long-press hint.
- **UX audit:** walk every page in both modes at 1440×900 and 390×844 (headless Firefox screenshots) and write `docs/ux-audit.md`: per page, what a user would come to do, whether the way to do it is visible, and a proposed fix, each rated quick / medium / large. Step 4 makes only the **quick** ones: labels, button wording, empty states that say what to do next with a link, a visible primary action in page headers, and links between related pages. Medium and large ones stay in the audit and go into `REQUESTS.md` under UI, replacing the current UI entry.

## Steps

### 1. Context menu on track rows
- `context-menu.js` (loaded in `layout.templ`), `placeMenu` with node tests, `data-menu`/`data-menu-host` on `trackMenu` and track rows, sheet styling below 720px.
- Headless Firefox screenshot with a menu forced open at a pointer position (set by a test hook, e.g. `window.contextMenu.openAt(el, x, y)`), 1440×900 and 390×844, in the scratchpad.

### 2. Menus for releases, artists and playlists
- The three menus, hosts on cards, rows and page headers, `trackFromAPI` with node tests. Screenshot of a release card's menu.

### 3. Actions in ⌘K, and a shortcuts dialog
- `commands.js` with node tests, the "Actions" group in the suggestion list (keyboard navigation as for links today), the `?` dialog. Screenshot of ⌘K with `>sh` typed.

### 4. UX audit and quick fixes
- `docs/ux-audit.md` with screenshot paths, the quick fixes (one commit per page group if large), `REQUESTS.md` UI entry replaced by the open medium/large items.
- `FEATURES.md`: context menus, menus for releases/artists/playlists, ⌘K actions, shortcuts dialog.
