# Plan 17: UI rework ("Console")

**Goal:** replace the flat top nav and generic zinc look with the "Console" design: a dark-first app shell with a left sidebar (library pages apart from work queues), a top strip with a ⌘K search, dense rows with monospace numbers, a restyled player, a bottom tab bar on phones, a **Vault** button that opens secret mode, and a secret-only **Tags** page ("tag lens").

**Mockup:** https://claude.ai/artifact/1p1f6goSWBsf1yV5GjYeKz (open it next to this plan; it shows every view, both modes, desktop and phone).

**Depends on:** plans 0–16.

## Status

Each step ticks its own box in the same commit, so `git log` and this list always agree. If work stops, the last ticked step is a working app: run `make build && ./audioloader` (or `make dev`). The next unticked step is where to resume; `git status` must be clean before starting it (discard a half-done step with `git checkout . && git clean -fd internal assets styles`).

- [x] 1. Tokens and theme
- [x] 2. App shell: sidebar and top strip
- [x] 3. Phone layout: tab bar and More sheet
- [x] 4. Vault button
- [x] 5. Player
- [x] 6. Library pages: tracks, releases, artists, playlists, podcasts
- [x] 7. Queue pages: inbox, tagging, downloads
- [x] 8. Forms: upload, batch, import, edit pages
- [x] 9. ⌘K search
- [ ] 10. Tags page (secret mode)
- [ ] 11. Docs

## Rules for this plan

- Every step is one commit. Before committing: `make build`, `go vet ./...` and `make test` pass, and the app starts and the touched pages render (open them, or `curl` them). Never commit a half-done step.
- Generated files (`*_templ.go`, `assets/css/app.css`) are committed with the step that changes their sources, as the repo does now.
- No new dependencies, no JS build step, no web fonts from a CDN: use the system font stacks below (the app is self-hosted and may run offline).
- Keep every `id` that `assets/js/*.js` looks up, and every `hx-*` attribute. Restyling must not change behaviour; if a JS file needs a new element, add it, don't rename old ones.
- Desktop (≥1024px) and phone (360–430px) both work after every step: no horizontal page scroll, touch targets ≥44px on phones.
- Commit messages match the log ("Add …", "Restyle …"), no co-author lines.

## Decisions

- **Tokens are CSS custom properties** in `styles/input.css`, registered in Tailwind's `@theme` so utilities like `bg-panel`, `text-fg-muted`, `border-line` exist. Pages use these semantic utilities; raw `zinc-*`, `sky-*` and `dark:` classes go away step by step (steps 6–8).
- **Dark first.** `<html data-theme="dark">` is the default. A small inline script in `<head>` applies `localStorage.theme` ("light"/"dark") before paint, wrapped in try/catch. Tailwind's `dark:` variant is redefined to follow `data-theme`, not the OS, so leftover `dark:` classes keep working until they're removed.
- **Secret mode keeps swapping only the accent group** (`body[data-mode="secret"]`), as today.
- **Secret mode nav: Tracks, Tags, Creators.** Playlists and Podcasts are hidden from the secret nav. Their routes stay as they are (removing secret playlists/shows is a separate decision, not part of this plan). "Artists" is labelled "Creators" in secret mode, as the upload forms already say.
- **Vault button** in normal mode only when the vault is configured (`theVault != nil`): a link to `GET /vault`, which already exists. In secret mode its place shows the existing Lock form. The template learns whether the vault is on through a package-level `components.VaultEnabled bool` set once in `api.Register`; no per-request plumbing.
- **Tags page** is `GET /tags`, registered with `handleSecret` (404 in normal mode, like `/upload/secret-source`). State lives in the URL (`?in=ambient&in=rain&ex=lofi&match=all&sort=added`), so a reload or a shared link keeps it, and htmx swaps one `#tag-lens` region. Counting and filtering are SQL in `internal/db`, not JS.
- **No schema change.** Tags already live in `track_tags`.

## Tokens (for step 1)

Values are dark / light. Fonts: body `ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`; mono `ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`.

| Token | Dark | Light |
|---|---|---|
| `--bg` | `#0e0f11` | `#f7f7f8` |
| `--panel` | `#15171a` | `#ffffff` |
| `--panel-2` (hover rows, inputs) | `#1c1f23` | `#f0f1f3` |
| `--line` (borders) | `#262a30` | `#e2e4e8` |
| `--fg` | `#e7e9ec` | `#16181b` |
| `--fg-muted` | `#9aa1ab` | `#5b636e` |
| `--fg-subtle` | `#6b727c` | `#8a919b` |
| `--accent` | `#3ddc97` | `#0f9d64` |
| `--accent-ink` (text on accent) | `#04140c` | `#ffffff` |
| `--accent-dim` | `rgba(61,220,151,.14)` | `rgba(15,157,100,.10)` |
| `--st-queued` | `#8a919b` | same |
| `--st-running` | `#5aa9ff` | same |
| `--st-done` | `#3ddc97` | `#0f9d64` |
| `--st-failed` | `#ff5d5d` | `#d93636` |
| `--st-review` | `#f5b84a` | `#b7791f` |

Secret mode overrides: `--accent #b48cff / #7c4dff`, `--accent-ink #140a26 / #ffffff`, `--accent-dim rgba(180,140,255,.16) / rgba(124,77,255,.10)`.

Type scale (rem): xs .6875, sm .8125, base .875, lg 1, xl 1.25, 2xl 1.625. Weights 400/500/600. Section labels: xs, 600, uppercase, letter-spacing .06em, `--fg-subtle`. Radius 4 / 6 / 10px. Popover shadow `0 8px 24px rgba(0,0,0,.45)` (light: `rgba(16,24,40,.12)`). Focus: 2px `--accent` outline, 2px offset. Row height 36px desktop, 44px phone. Sidebar 232px, top strip 48px, player 64px. Motion 120ms ease-out.

## Steps

### 1. Tokens and theme (`styles/input.css`, `layout.templ`)
- Add the tokens above on `:root` / `[data-theme="light"]` and the secret overrides on `[data-mode="secret"]` (both themes). Register them in `@theme` (`--color-bg`, `--color-panel`, `--color-panel-2`, `--color-line`, `--color-fg`, `--color-fg-muted`, `--color-fg-subtle`, `--color-accent`, `--color-accent-ink`, `--color-accent-dim`, `--color-st-*`, `--font-sans`, `--font-mono`, the text sizes) so they become utilities.
- `@custom-variant dark (&:where([data-theme=dark], [data-theme=dark] *));`
- `<html lang="en" data-theme="dark">` and the inline theme script in `<head>`. Set `color-scheme` per theme, and `scrollbar-color: var(--line) transparent` on `html`.
- Re-point `body`, `a`, `.btn`, `.input`, `.card` to the tokens. `.btn` = accent fill + accent-ink text; add `.btn-ghost` (panel-2 fill, fg text) for secondary actions. Replace the old `--accent*` variables with the new ones and remove the `[data-mode="secret"] .btn` override (the token swap covers it).
- **Done when:** every page renders dark by default with the green accent (violet in secret mode), and nothing is unreadable. Layout is still the old one.

### 2. App shell: sidebar and top strip (`layout.templ`)
- Body becomes a grid: `sidebar | main`, with the top strip and player in the main column (see mockup). Sidebar: wordmark, Upload as a full-width `.btn`, then labelled groups:
  - LIBRARY: Tracks, Releases (normal only), Artists ("Creators" in secret), Playlists and Podcasts (normal only).
  - QUEUE (normal only): Inbox, Tagging, Downloads, using the existing `navLinkBadge` loads, with badges right-aligned in mono.
  - Footer: theme toggle (Dark / Light, writes `localStorage.theme` and `data-theme`), then the version/library line if one exists.
- Active link: `--accent-dim` fill and `--accent` text, with `aria-current="page"` kept.
- Top strip: the search form (same `hx-*` as today), and on the right the Secret pill and Lock form in secret mode.
- Below 720px for now: the sidebar simply stacks above the content (step 3 replaces it).
- **Done when:** desktop matches the mockup's shell in both modes; the existing tests pass (they check nav links per mode, so update their expectations only where links moved, not where they disappeared by mistake).

### 3. Phone layout: tab bar and More sheet (`layout.templ`, `assets/js/nav.js`)
- Below 720px: hide the sidebar and show a fixed bottom tab bar (under the player).
  - Normal mode tabs: Tracks, Releases, Artists, Inbox (badge), More (badge = tagging + downloads).
  - Secret mode tabs: Tracks, Tags, Creators. No More tab.
- More opens a `<dialog>` bottom sheet listing Playlists, Podcasts, Tagging and Downloads with their badges. Tabs are ≥44px, with icon above label. Icons are inline SVG (16px, `stroke="currentColor"`), defined once in a templ component `icon(name string)`.
- The Upload button moves into the top strip as an icon button.
- `nav.js` opens and closes the sheet (Esc and a backdrop click close it). It is loaded `defer` from the layout.
- Tags in the secret tab bar points at `/tags`, which 404s until step 10. Hide that tab and the sidebar link behind a `const tagsPage = false` in `layout.templ` until step 10 flips it.
- **Done when:** at 390px wide, both modes have no horizontal scroll, every nav target is reachable, and the player is not covered.

### 4. Vault button (`layout.templ`, `internal/api/routes.go`, tests)
- `components.VaultEnabled` is set in `api.Register` from `v != nil`.
- Normal mode with the vault on: a "Vault" button with a lock icon at the bottom of the sidebar (above the theme toggle) and as an icon button in the phone top strip, linking to `/vault`. With the vault off, nothing is shown.
- Secret mode: in the same places, the existing Lock form (`POST /vault/lock`, `hx-boost="false"`, `onsubmit="window.player?.reset()"`).
- Restyle `VaultPage` to the new tokens: a centred panel with a lock icon, passphrase input, and Unlock button.
- Tests (`internal/api/modes_test.go` style):
  - normal mode with a vault shows `href="/vault"`;
  - normal mode without a vault doesn't;
  - secret mode shows the lock form and no `/vault` link.
- **Done when:** clicking Vault, entering the passphrase, and unlocking lands in secret mode, and Lock returns to normal.

### 5. Player (`layout.templ` `player()`, `assets/js/player.js`)
- Keep `<audio id="player-audio">` but drop its native `controls`.
- Add a play/pause button `#player-play` (round, accent fill), a seek `<input type="range" id="player-seek">`, and times `#player-time` / `#player-remaining` in mono. `player.js` syncs them on `timeupdate` / `loadedmetadata`, seeks on `input`, and toggles on click and on Space when focus isn't in a field.
- Layout per the mockup. Desktop: one 64px row (cover, title/artists, seek with times, shuffle/prev/play/next, queue). Phone: two rows above the tab bar (cover, title, play, next, queue; then a thin seek). The episode controls (−15/+30/speed) and chapters stay, restyled.
- Queue popover: panel background, popover shadow, rows 36px.
- Update the player tests in `tests/js` if any of them query changed elements.
- **Done when:** play, pause, seek, prev/next, shuffle, queue, chapters and speed all work on a track and on an episode, and playback survives navigating between pages (hx-preserve).

### 6. Library pages (`tracks.templ`, `releases.templ`, `artists.templ`, `playlists.templ`, `shows.templ`, `search.templ`)
- Track list: dense rows (`#` in mono, cover 28px pixelated, title/artist, album or source, added date and duration in mono), hover `panel-2`, current track in accent with the "playing" bars. Phone: stacked rows (cover, title/artist, duration), with tags or album as a muted second line.
- Secret track rows show creators and tags (small chips) instead of releases.
- Release page: header with cover, type label, title, artists, year · tracks · total length in mono; disc groups as section labels.
- Artists: grid of cards with round pixelated avatars.
- Playlists and podcasts: per the mockup.
- Replace every `zinc-*`, `sky-*` and `dark:` class in these files with the semantic utilities.
- **Done when:** these pages match the mockup at both widths and in both modes, and `grep -E 'zinc-|sky-|dark:' internal/components/{tracks,releases,artists,playlists,shows,search}.templ` is empty.

### 7. Queue pages (`tracking.templ`, `tagging.templ`, `downloads.templ`)
- A `statusChip(status string)` component: a mono label on a 14%-tinted background of its `--st-*` colour, for queued/running/done/failed/review.
- Downloads: running rows get a thin progress bar, and failed rows a Retry `.btn-ghost`.
- Inbox: release rows with cover, artist, type, and date, with "Add" (`.btn`) and "Ignore" (`.btn-ghost`).
- Same class sweep as step 6.
- **Done when:** these pages match the mockup at both widths, and the grep from step 6 is empty for these files.

### 8. Forms (`addsong.templ`, `batch.templ`, `import.templ`, `secretsource.templ`, the edit pages, `vault.templ`)
- Labels as small muted text above the inputs, `.input` everywhere, and primary/secondary buttons as `.btn` / `.btn-ghost`. Choice pills (the `has-[:checked]` labels) use accent-dim and accent when checked.
- Same class sweep; afterwards the grep over all of `internal/components/*.templ` is empty.
- **Done when:** the single, batch, import and edit flows still submit and show their results, at both widths.

### 9. ⌘K search (`layout.templ`, `assets/js/nav.js`)
- The top-strip search input gets a `⌘K` / `Ctrl K` hint. Pressing either shortcut, or `/` outside a field, focuses it.
- The `#search-suggest` results render as a popover panel under the input, with arrow keys moving through the links, Enter opening one, and Esc clearing and blurring.
- **Done when:** the shortcut works on every page, and the existing search suggest still loads.

### 10. Tags page, secret mode (`internal/db/tags.go`, `internal/api/tags.go`, `internal/components/tags.templ`, `assets/js/tag-lens.js`)
- **DB.** Name it `TrackTags`, not `TagCounts`, which already exists for tagging status:
  ```go
  // TagQuery selects tracks by their tags. Include is all-of when Any is
  // false, any-of when true; Exclude always removes.
  type TagQuery struct {
      Include, Exclude []string
      Any              bool
      Sort             string // "added" (default), "creator", "tag"
  }

  // TrackTags returns, for the tracks v can see that match q, each tag with
  // how many of those tracks have it. With an empty q, that's every tag's count.
  func TrackTags(v Viewer, q TagQuery) (map[string]int, error)

  // TracksByTags returns the tracks v can see that match q, with their tags.
  func TracksByTags(v Viewer, q TagQuery) ([]*audio.Track, error)
  ```
  Use `v.trackCond("t")` like the other queries, with `NormalizeTags` on the input. Tests: all/any/exclude, an empty query, case-insensitivity, and no normal-mode track leaking into secret counts.
- **Route.** `handleSecret(mux, "GET /tags", getTagsPage)`. It renders the whole page, or just `#tag-lens` when `HX-Request` is set. Add `/tags` wherever the leak and mode tests list secret-only routes.
- **Template** `TagsPage`:
  - **Sticky query bar.** Include tokens are accent-dim. Exclude tokens have a `--st-failed` border, a struck-through label and a "−" prefix. Each token has an × link. Then a text input, an all/any segmented control, and Clear.
  - **Tag field.** Each chip is the label plus a mono count. Chips step through three size tiers by count (sm/base/lg) and are sorted by count, then name. Chips with a 0 count sit after a "No matches" divider at 35% opacity.
  - **Results.** A mono header (`N tracks · m:ss`) and a sort control, then the track rows from step 6. Tags that match an include are highlighted in accent.
  - **Empty states.** With no tokens: "Pick tags to narrow down N tracks". With no results: "No track has all of these", with links to switch to any or to drop the last token.
- Every chip, token and control is an `<a href="/tags?…">` with `hx-get`, `hx-target="#tag-lens"`, `hx-select="#tag-lens"` and `hx-push-url="true"`, so the page works without JS. Clicking a chip cycles neutral → include → exclude → neutral; the template computes each link's URL.
- **tag-lens.js** (progressive):
  - Typing in the input filters the visible chips by prefix. Enter follows the top chip's include link, Shift+Enter its exclude link, and Backspace in an empty input follows the last token's × link.
  - Right-click, or a 500ms long-press, on a chip follows its exclude link.
- **Phone.** Tokens scroll sideways inside the bar. The field shows the top 12 chips plus "All tags (N)", which opens a `<dialog>` sheet with the full field. Chips are ≥44px tall.
- Flip `tagsPage` to `true` (step 3).
- **Done when:**
  - in secret mode, the mockup's include/exclude/any flows work with and without JS, and the URL reflects the state;
  - `/tags` is a 404 in normal mode;
  - the tests pass.

### 11. Docs
- `FEATURES.md`: add a "UI" section (shell, themes, phone tab bar, ⌘K, Vault button) and the Tags page under "Secret mode".
- `docs/plans/README.md`: add row 17, and set the status line to "plans 0–17".
- Tick step 11 here.
