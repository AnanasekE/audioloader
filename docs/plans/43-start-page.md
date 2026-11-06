# Plan 43: Start page

**Goal:** `/` opens a home page — what waits in the inbox, the episodes to carry on with, and the releases that reached the library last — instead of redirecting to the track list. Closes the UX audit's "No start page" (REQUESTS "A start page").

**Depends on:** plan 4 (podcasts: playback positions and `ContinueListening`), plan 10 (inbox counts), plan 17 (Console look, sidebar), plan 18 (component kit), plan 25 (sidebar icons), plan 37 (release cards with their ⋯ menus), plan 41 (Apple Music follows in the inbox count).

## Status

- [x] 1. Recently added queries
- [ ] 2. The home page at `/`
- [ ] 3. The action palette and the web app start page
- [ ] 4. Screenshots and docs

Written after the change, from the finished diff; the four commits that follow it are the steps.

## Rules

As in plan 42: `make build`, `go vet ./...` and `make test` pass; one commit per step, app runnable after each; no attribution lines in commits; plain JS in `assets/js`, no build step; tests use `httptest` fakes. Never run anything on port 8080 (the user's own server). Secret mode must behave exactly as before.

## Decisions

- **Normal mode only.** Secret mode keeps redirecting `/` to `/tracks`: it has neither an inbox nor releases, and its track list is already its date-added view. `GET /{$}` stays registered with `handle` (both modes) and branches on `viewer(r).IsSecret()`, rather than becoming `handleNormal` — a 404 on `/` would be worse than a redirect. The sidebar offers Home in normal mode only, so secret mode never shows a link that only redirects.
- **Three sections, each from one query.** The inbox count, continue listening, recently added. No section has its own page state; the whole page is one GET with no parameters.
- **Continue listening is episodes only.** `db.ContinueListening` selects `kind = 'episode'`, and `assets/js/player.js` only saves positions for episodes (`if (!isEpisode(current) ...) return`), so music tracks have no resume point to offer. The limit is 5, as on the track list. Reuses the `ContinueListening` component unchanged.
- **"Recently added" is releases, newest-added first, by `rowid DESC`.** Neither `releases` nor `tracks` carries a timestamp, and `rowid` order is the order rows were added — the same proxy the track list's "Added" sort already uses. `release_date` is when the music came out, not when it reached the library, so `ListReleases`' ordering is wrong here and the page needs its own query. 12 of them, as release cards with their ⋯ menus (reuses `releaseGrid`).
- **No releases falls back to the tracks added last.** A library can hold tracks that are on no release (see "Turn the existing tracks without a release into singles" under FEATURES "Not implemented yet"), and `RemoveRelease`'s "delete the release only" leaves its tracks behind. Only queried when there are no releases. An empty library needs no third case: `TrackList` renders its own "No tracks yet. Add one." empty state.
- **The inbox section is the badge's number.** `db.InboxCount()` plus `db.AppleFollowsToPick()`, extracted out of `getInboxBadge` into one `inboxWaiting()` helper so the badge and the page can never disagree. Hidden entirely at zero — a section reading "0 new releases" is noise.
- **The inbox prompt is a `.card`, not a `.list` row.** `styles/input.css` has `.row a { color: inherit }`, which at `(0,1,1)` outspecifies `.btn`'s own `text-accent-ink` at `(0,1,0)` — a filled button link inside a `.row` renders with the row's text colour on an accent background. Every in-row button elsewhere in the app is a `<button>`, which the rule can't reach, so this is the first case that would have hit it.
- **Four ways in.** Home first in the sidebar's Library group; first in the phone Library sheet, because the tab bar is already full at five and Home is not worth displacing Tracks, Releases, Artists or Inbox; a `home` icon in `iconClass`; and a "Go to Home" action in the palette. The palette action cannot use the `page()` helper: it gates on `hasLink(href)` and every page links to `/` through the sidebar logo, including in secret mode, so it is written out with `when: () => !secret()` like `go-artists`.
- **`start_url` in the manifest moves from `/tracks` to `/`.** An installed web app should open on the start page like a browser does.
- **No new CSS.** The page reuses `.card`, `.list`, `.card-grid` and `.section-label`; verified by a byte-identical minified Tailwind build with and without the change, so `assets/css/app.css` is untouched.

## Steps

### 1. Recently added queries
- `internal/db/releases.go`: `RecentReleases(v, limit)`, `rowid DESC`, artists attached.
- `internal/db/tracks.go`: `RecentTracks(v, limit)`, `rowid DESC`, the library-track filter and the usual attach chain.
- Both take a `Viewer`, so the existing mode tests cover visibility.

### 2. The home page at `/`
- `internal/components/home.templ`: `PageHome`, `HomePage(inbox, continueListening, releases, tracks)`.
- `internal/api/pages.go`: `getIndex` gathers the three sections; `recentOnHome = 12`.
- `internal/api/tracking.go`: `inboxWaiting()` extracted; `getInboxBadge` calls it.
- `internal/components/layout.templ`: Home in `sidebar` (normal mode only) and `moreSheet`, the `home` icon, `PageHome` in the Library tab's active test.
- `internal/api/home_test.go`: the full page, an empty inbox hiding the section, the no-releases fallback, the secret-mode redirect. The cross-mode leak sweep in `leak_test.go` picks `/` up on its own.

### 3. The action palette and the web app start page
- `assets/js/commands.js`: `go-home`, before `go-tracks`.
- `assets/manifest.webmanifest`: `start_url` → `/`.

### 4. Screenshots and docs
- `tools/shots.sh`: `/` added to `DEFAULT` (it shoots as `index-<width>.png`).
- `FEATURES.md`: the home page under UI; the sidebar and phone sheet lists; the manifest's start page.
- `REQUESTS.md`: drop the start page from the UI "Large" list.
- `docs/ux-audit.md`: strike "No start page" and say what replaced it.
- `docs/plans/README.md`: the row and the Status line.
- Screenshots at 1440×900 and 390×844 were **not** taken: `tools/shots.sh` needs `firefox` and `sqlite3`, neither of which is on the dev box. The page was checked as rendered HTML instead; it still wants a look in a browser.
