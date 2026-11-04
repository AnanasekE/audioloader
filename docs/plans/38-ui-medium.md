# Plan 38: UX audit, medium fixes

**Goal:** the "Medium" items of the UI request (`REQUESTS.md`, from `docs/ux-audit.md`): sort on Tracks, filter boxes on the Releases and Artists grids, a type filter on Search, Downloads rows linked to their release and artist, and the phone pages behind "More" easier to find.

**Depends on:** plan 18 (component kit), plan 37 (UX audit).

## Status

- [x] 1. Sort control on the Tracks page
- [x] 2. Filter box on the Releases and Artists grids
- [x] 3. Type filter on the Search page
- [x] 4. Downloads rows link to their release and artist
- [ ] 5. Phone navigation: "More" becomes "Library"

## Rules

As in plan 19: `make build`, `go vet ./...` and `make test` pass; one commit per step, app runnable after each; no attribution lines in commits; plain JS in `assets/js`, no build step; pure logic in small modules with node tests; tests use `httptest` fakes. Never run anything on port 8080 (the user's own server).

## Decisions

- **Tracks sort** works like the secret Tracks page's sort (reuse its control and its SQL ordering, don't write a second one): Title, Artist, Recently added (default stays what the page uses today). The choice is a `?sort=` query param so links and back/forward keep it; the htmx-boosted page reloads with the param. API test for each order.
- **Grid filter** is client-side: a small `assets/js/grid-filter.js` filters cards whose `data-filter-text` (title + artist names, lower-cased) contains every typed word. The box sits in the page header next to the primary action, `/` doesn't steal focus from it. Pure `matches(text, query)` is node-tested. If a grid is paginated server-side, the filter goes to the server as `?q=` instead (check which; use the existing search SQL in `internal/db/search.go`).
- **Search type filter:** chips All / Tracks / Releases / Artists / Playlists above the results, as `?type=` on `/search`; "All" is today's page. The filter only hides sections server-side; no new SQL beyond limits (a single type shows up to 50 instead of the section's preview count).
- **Downloads rows:** the title links to the release page, the artist names to their artist pages (use the same artist-credit link component as track rows). Placeholders without a release link only the artist.
- **Phone navigation:** the last tab "More" is renamed "Library" with a library icon; its sheet lists Playlists, Podcasts, Tagging, Downloads, Inbox, Tags, Add as large rows (icon, label, count badge where the sidebar has one, reusing the `/ui/badges/*` endpoints). The badge total shows as a dot on the "Library" tab.

## Steps

### 1. Sort control on the Tracks page
- `internal/api/tracks.go`, `internal/db/tracks.go`, `internal/components/tracks.templ`. Tests per sort order.

### 2. Filter box on the Releases and Artists grids
- `assets/js/grid-filter.js` (+ `tests/js/grid-filter.test.mjs`), `releases.templ`, `artists.templ`, `layout.templ` (script tag).

### 3. Type filter on the Search page
- `internal/api/pages.go` (or wherever `getSearchPage` is), `search.templ`. API test for `?type=releases`.

### 4. Downloads rows link to their release and artist
- `downloads.templ` (+ the view struct in `internal/api/downloads.go` if it lacks the IDs). Test that the page contains the release link.

### 5. Phone navigation
- `layout.templ`. Headless Firefox screenshots at 390×844 of the sheet open, in the scratchpad. `FEATURES.md` updated; the Medium bullets removed from `REQUESTS.md`.
