# Plan 18: Page redesign to match the mockup

**Goal:** plan 17 gave the app the Console shell (sidebar, top strip, tab bar, player) and tokens, but the pages kept their old structure: only their colour classes changed. This plan rebuilds the page markup so every page looks like `docs/design/console-mockup.html` (the published mockup: https://claude.ai/artifact/1p1f6goSWBsf1yV5GjYeKz).

**Depends on:** plan 17.

## Status

Each step ticks its own box in the same commit. The last ticked step is always a working app (`make build && ./audioloader`); resume at the first unticked box with a clean `git status`.

- [x] 1. Demo data and screenshot script
- [x] 2. Component kit
- [x] 3. Tracks and search
- [ ] 4. Release page
- [ ] 5. Releases and artists
- [ ] 6. Artist page
- [ ] 7. Playlists and podcasts
- [ ] 8. Inbox and follow
- [ ] 9. Downloads and tagging
- [ ] 10. Forms and edit pages
- [ ] 11. Tags page, secret track list
- [ ] 12. Final pass

## Rules for this plan

- Everything in plan 17's "Rules for this plan" still applies: one commit per step, `make build`, `go vet ./...` and `make test` pass, keep every `id` the JS uses and every `hx-*` attribute, no new dependencies, no attribution lines.
- **Structure, not just colour.** A step is not done because classes changed. It is done when its screenshots, put next to the mockup's matching view at the same width, show the same structure: the same regions in the same places, the same row anatomy, the same type sizes, mono numbers, chips, and spacing rhythm.
- **Verify by looking.** Every step from 3 on runs `tools/shots.sh` (step 1). Then it Reads the PNGs of the pages it touched at 1440 and 390, and the mockup's PNG of the same view, and fixes the differences before committing. Pages the mockup doesn't show (marked *derived* below) follow the component kit, so they look like they belong with the ones it does.
- Markup goes through the step-2 components. A page that hand-rolls a row, header, or chip that the kit has is not done.

## Mockup map

| App page | Mockup view (sidebar link in the mockup) |
|---|---|
| `TracksPage`, `SearchPage` | Tracks |
| `ReleasePage` | click a track's album → Release |
| `ReleasesPage`, `ArtistsPage`, `ArtistPage` | *derived* (the mockup's stub); use Release header + cover grid |
| `PlaylistsPage`, `PlaylistPage` | Playlists |
| `ShowsPage`, `ShowPage` | Podcasts |
| `InboxPage`, `FollowPage` | Inbox |
| `DownloadsPage`, `YouTubeImportPage` | Downloads |
| `TaggingPage` | *derived*: Inbox rows + status chips |
| Upload, batch, import, add song, edit pages, secret source, vault | *derived*: form kit (step 2) |
| `TagsPage`, secret `TracksPage` | Secret toggle → Tags, Tracks |

## Steps

### 1. Demo data and screenshot script (`tools/seed`, `tools/shots.sh`)
- `go run ./tools/seed -db <path> -files <dir>` fills an empty database through the `internal/db` functions (no raw SQL that skips their rules):
  - 4 artists, 3 albums (one with 2 discs), 2 EPs and 4 singles, about 40 tracks, 1 placeholder track, 2 playlists, 1 show with 4 episodes (one half-played);
  - inbox releases (new and ignored), 3 tagging reviews, downloads in every state;
  - 18 secret tracks with 1–5 tags each from about 25 tags.
  The audio files are 2-second silent WAVs written by the tool. Covers are tiny generated pixel PNGs. It's idempotent: if the DB isn't empty, it refuses.
- `tools/shots.sh [page…]`:
  1. starts `./audioloader` on `127.0.0.1:18090` with a seeded DB in `tmp/shots/` (the seed runs if that DB is missing);
  2. screenshots each listed path (default: every page in the map above) with headless Firefox (`firefox --headless --no-remote --profile <fresh tmp dir> --window-size=W,H --screenshot out.png URL`) at 1440×900 and 390×844, into `tmp/shots/<name>-<w>.png`;
  3. screenshots the mockup views the same way, from a copy of `docs/design/console-mockup.html` with a trailing `<script>` that calls the mockup's `go("<view>")` (and `secret(true)` for Tags), into `tmp/shots/mock-<view>-<w>.png`;
  4. stops the server.
  For secret-mode pages it sets `AUDIOLOADER_SECRET` to a known test hash. It also gets the vault cookie by POSTing `/vault` with `curl` and passes the cookie to Firefox: write it into the fresh profile's `cookies.sqlite`, or, if that proves fiddly, add a dev-only `AUDIOLOADER_DEV_SECRET=1` that makes every request secret, refused unless the address is loopback.
- `tmp/` is gitignored.
- **Done when:** `tools/shots.sh /tracks` produces app and mockup PNGs at both widths, and the app one shows the seeded tracks.

### 2. Component kit (`internal/components/kit.templ`, `styles/input.css`)
Port these from the mockup's CSS and markup, as templ components plus a few `@layer components` classes. Names are fixed so later steps share them:
- `pageHead(title, meta string) { actions }`: the 2xl title, a muted meta line, actions right-aligned, and a filter input slot.
- `list() { rows }`: the panel box with a `--line` border, radius 10, and row dividers. It has an optional `listHead(cols…)` header row: xs uppercase labels in `--fg-subtle`.
- `row`: a CSS grid row of 36px (44px on phones) with a hover `panel-2` fill.
  - Track row columns: `#`/playing indicator · 28px cover · title over muted artists · album/source · mono duration · menu.
  - At <720px it collapses to cover · title/artists (second muted line for album or tags) · duration.
- `cover(src, size)`: square, pixelated, radius 4, with the `--line` outline.
- `avatar(src, size)`: round, pixelated.
- `chip(label)`: the small rounded tag chip in `panel-2` and `fg-muted`. The `chipAccent` variant uses accent-dim and accent.
- `statusChip`: already in plan 17. Move it here.
- `sectionLabel(text)`: the sidebar-style uppercase label, used for disc groups and page sections.
- `mediaHeader(cover, kind, title, sub, meta)`: the release header from the mockup (big cover, type label, title, artists, mono meta "2024 · 12 tracks · 48:10"), plus an actions slot.
- `cardGrid` / `card(cover, title, sub, href)`: the playlist card grid from the mockup, used for releases, artists (with `avatar`) and shows.
- `progress(pct)`: the thin bar from the downloads and podcast rows.
- `segmented(options)`: the mockup's seg control, used for sort, all/any, and inbox tabs.
- Form kit: `field(label, hint) { input }`, `.input`, `.select`, `.btn`, `.btn-ghost`, `.btn-danger`. Choice pills (`has-[:checked]`) are rendered by `choice()`. The `formPanel` is a panel, max-width 640, with fields stacked 16px apart.
- Menus: `trackMenu` popovers use the popover panel and shadow from the mockup's queue popover.
- **Done when:** the kit renders on a temporary `GET /dev/kit` page, on loopback only, which shows every component in both themes. Screenshot that page. The route is removed in step 12.

### 3. Tracks and search (`tracks.templ`, `search.templ`)
- `TracksPage`: `pageHead("Tracks", "N in library")` with the filter input, then a "Continue listening" row of cards if there is any. Then `list` with `listHead` (#, Title, Album, Time) and track rows. Sort is a `segmented` control in the head.
- Current track: accent title plus the mockup's playing bars, driven by the same `aria-pressed`/state the player already sets.
- Placeholder tracks: the row at 45% opacity, with a "missing" chip.
- `SearchPage` and the `#search-suggest` popover: grouped sections (Tracks / Releases / Artists) with `sectionLabel`, built from the same rows.
- **Done when:** `tmp/shots/tracks-*` match `mock-tracks-*` structurally, and `search?q=…` looks like part of the same app.

### 4. Release page (`releases.templ`: `ReleasePage`, `albumView`, `singleView`)
- `mediaHeader` with Play and Shuffle buttons and the release actions menu.
- Discs as `sectionLabel("Disc 1")` above their own `list`. Rows use the release track numbers in the # column.
- The single page uses the same header. B-sides and "Also on" are lists below it.
- **Done when:** it matches `mock-release-*`.

### 5. Releases and artists (`ReleasesPage`, `ArtistsPage`)
- Releases: `pageHead` plus a `segmented` type filter (All / Albums / EPs / Singles / Compilations), then shelves of `sectionLabel` + `cardGrid`. Singles are `list` rows, as today.
- Artists: `pageHead` with Add artist and Add from MusicBrainz as actions (their forms open in a popover panel), then `cardGrid` with `avatar` cards: name and "N releases".
- **Done when:** both pages read as siblings of the mockup's Playlists grid at both widths.

### 6. Artist page (`ArtistPage` and its forms)
- `mediaHeader` with a round avatar, the MusicBrainz and Twitch links as chips, and Follow as the primary action.
- Releases by type as shelves. "Other tracks" is a `list`.
- The manage forms (rename, merge, picture, Twitch, MB link) move into a collapsible "Manage" `formPanel` at the bottom, closed by default.
- **Done when:** the header matches the release header's rhythm, and the forms still work.

### 7. Playlists and podcasts (`playlists.templ`, `shows.templ`)
- These match the mockup's Playlists and Podcasts views. Episode rows have `progress` and a mono "time left".
- The playlist and show pages use `mediaHeader` + `list`.
- The playlist edit page uses the form kit.
- **Done when:** they match `mock-playlists-*` and `mock-podcasts-*`.

### 8. Inbox and follow (`tracking.templ`)
- Inbox matches `mock-inbox-*`: a `segmented` control for the New / Ignored tabs and `followFilters` as chips. Each row has a cover, title, artist, type chip and mono date, then the existing actions as `.btn` / `.btn-ghost`. Keep their labels and `hx-*`.
- Follow page: `mediaHeader` for the artist, and candidates as `list` rows with "Expand" opening an inline sub-list.
- **Done when:** it matches `mock-inbox-*`.

### 9. Downloads and tagging (`downloads.templ`, `tagging.templ`)
- Downloads matches `mock-downloads-*`. Sections are Active / Needs pick / Failed / Not found, each a `sectionLabel` + `list`. Rows carry a `statusChip`; running rows also get `progress` and failed rows a Retry button. YouTube candidates are a nested `list` with a cover and a mono duration.
- YouTube import uses the same parts.
- Tagging is *derived*: a review row puts the current file next to the MusicBrainz match as two columns. On phones they stack. The row has a `statusChip`. Apply is `.btn`, and Skip/Wrong use `.btn-ghost`. Artist and cover suggestions are `list` rows.
- **Done when:** downloads matches the mockup and tagging matches its kit siblings.

### 10. Forms and edit pages (upload, batch, import, add song, track edit, release edit, playlist edit, secret source, vault)
- Every form uses `formPanel` + `field`. Upload kind choice uses `choice()` pills.
- Batch and import rows are `list` rows with a mono per-row status `statusChip`.
- Release edit: the tracklist editor uses `list` rows with drag handles, and the details/cover forms are `formPanel`s side by side on desktop, stacked on phones.
- Destructive actions use `.btn-danger` and sit at the bottom.
- **Done when:** every form page is screenshotted, every flow still submits (the API tests pass), and none of them looks like the old app.

### 11. Tags page and secret track list (`tags.templ`, secret parts of `tracks.templ`)
- Compare against `mock-tags-*` and the mockup's secret Tracks view, in secret mode. The query bar, tag field tiers, "No matches" divider and result rows with `chipAccent` must match.
- Secret track rows show creators plus tag chips in the album column. On phones the tags go on the second line.
- **Done when:** it matches at both widths.

### 12. Final pass
- Remove `/dev/kit`.
- Run `tools/shots.sh` with no arguments.
- Look at every page at both widths in both themes: add a `?theme=light` handling to the shots script, or a light run with `localStorage` preset via the trailing-script trick for the mockup and a cookie-less `data-theme` override flag for the app; keep whichever is simplest.
- Fix any page that still shows the old structure.
- Tick this plan in `docs/plans/README.md` (row 18, status "plans 0–18").
- **Done when:** no screenshot looks like the pre-plan-17 app.
