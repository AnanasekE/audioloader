# Plan 35: React web app

**Goal:** rebuild the web UI in React, keeping the Console look (plans 17–18), so it is easy to find where every action lives: right-click and long-press menus on tracks, releases, artists and playlists, one consistent action menu, and a command palette that can run actions, not just navigate.

**Depends on:** plans 21 (JSON API), 18 (Console kit), 30–34 (they change the current UI first, and their logic is ported).

**This plan changes a shared rule:** the web UI gets a build step (Vite). The Go server stays the only process at runtime: it serves the built files embedded with `embed.FS`.

## Status

- [ ] 1. App shell next to the old UI
- [ ] 2. Write API for what the pages do
- [ ] 3. Player and queue
- [ ] 4. Library pages, read-only
- [ ] 5. Action registry, context menus, palette
- [ ] 6. Edit and manage pages
- [ ] 7. Add, Downloads, Inbox, Tagging
- [ ] 8. Secret mode
- [ ] 9. Switch over, remove the templ UI

## Rules

- Steps 1–8 keep the templ UI working unchanged at `/`; the React app lives at `/app/` until step 9. `make build`, `go vet ./...`, `make test`, and in `web/` `npm run typecheck`, `npm test` and `npm run build` pass after each step. One commit per step (big steps may be split into several commits listed under the step), no attribution lines.
- Every new endpoint is in `docs/api.md` with a Go test, as in plan 21.

## Decisions

- **Stack:** `web/` with Vite, React 19, TypeScript (strict), React Router, TanStack Query for server state, Tailwind with the tokens from `styles/input.css` (same names, so the look carries over), Vitest + Testing Library. No component library: the Console kit is ported as our own components (`Button`, `Row`, `Menu`, `Sheet`, `Cover`, `Badge`…).
- **Shared API types:** the shapes in `mobile/src/api/types.ts` move to a small `shared/api` folder both apps import (path alias), so the web and the phone can't drift.
- **Actions in one place:** an action registry (`web/src/actions/`), each action `{id, label, icon, appliesTo(entity), run(entity)}`. The ⋯ button, the right-click menu, long-press on touch, and the ⌘K palette all read it. This is the fix for "I never know where to go": every action of a thing is on the thing.
- **Context menu:** right-click on a row/card opens the menu at the pointer; long-press (500 ms) on touch opens it as a bottom sheet; the keyboard opens it with the Menu key or Shift+F10. Multi-select (Shift/Ctrl-click) applies queue actions and "Add to playlist" to all selected tracks.
- **Player:** one React provider owns the `<audio>` element; the pure logic modules (`player-state.js`, `sleep-timer.js`, repeat, play-next order, autoplay) are ported to TS with their tests, not rewritten.
- **Writes go JSON:** the htmx handlers return HTML. Each page's writes get `/api/v1` JSON endpoints (step 2 lists them by page) that call the same `internal/db` functions; the HTML handlers stay until step 9.
- **Auth and secret mode** keep the vault cookie and the server-side viewer; the React app only asks `/api/v1/me`.
- **Routing:** the same paths as today (`/artists/<id>`, `/releases/<id>`…), under `/app/` until step 9, so links and the share Shortcut keep working after the switch.

## Steps

### 1. App shell next to the old UI
- `web/` scaffold, Makefile targets (`make web`, `make build` builds it first), embed and serve at `/app/` with SPA fallback; sidebar, phone tab bar, theme tokens; CI-style checks in `make test`.

### 2. Write API for what the pages do
- Inventory every `hx-post`/`hx-delete` in `internal/components` into a table in this plan, grouped by page, and add JSON endpoints for them in batches (one commit per page group), with tests.

### 3. Player and queue
- Provider, bar, queue panel, sleep timer, volume, repeat, autoplay; ported logic tests pass.

### 4. Library pages, read-only
- Tracks, Releases, Release, Artists, Artist, Playlists, Shows, Search.

### 5. Action registry, context menus, palette
- Registry with track/release/artist/playlist actions, the context menu and sheet, multi-select, ⌘K running actions. Tests for keyboard and long-press.

### 6. Edit and manage pages
- Track, release, artist and playlist editing, delete flows, cover-of, artist links, trim.

### 7. Add, Downloads, Inbox, Tagging
- The Add box with its routing, YouTube picks, batch and library import (the tag readers in `assets/js/import.js` ported to TS), downloads, inbox, tagging.

### 8. Secret mode
- Vault unlock, secret tracks, creators, tags lens, secret source.

### 9. Switch over, remove the templ UI
- Serve the React app at `/`; delete the templ components, htmx, page handlers and `assets/js` that nothing uses; update `README.md`, `FEATURES.md`, the plan rules in `README.md` (build step, `web/`), and `REQUESTS.md` (remove the UI entry).
