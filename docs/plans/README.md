# Implementation plans

Run the plans in this order. Each one assumes the ones before it are done.

**Status:** plans 0–34, 36 and 37 are implemented; 35 is on hold. Optional items left out are listed under "Not implemented yet" in `FEATURES.md`.

| # | Plan | Covers (FEATURES.md) |
|---|------|----------------------|
| 0 | [Prerequisites](00-prerequisites.md) | migrations, tests, unique artists, release editing, covers, placeholder tracks |
| 1 | [MusicBrainz auto-tagging](01-musicbrainz-tagging.md) | Auto-tagging (MusicBrainz) |
| 2 | [Artist release tracking](02-release-tracking.md) | Artist release tracking |
| 3 | [Playback and discovery](03-playback-discovery.md) | Queue, search, playlists |
| 4 | [Podcasts and long audio](04-podcasts.md) | Podcasts and long audio |
| 5 | [Secret tracks](05-secret-tracks.md) | Secret tracks |
| 6 | [Download sources](06-download-sources.md) | spotDL, source order, pick from YouTube (REQUESTS.md) |
| 7 | [Track editing and artist pages](07-editing-and-artist-pages.md) | Edit and delete tracks, artist pages and pictures |
| 8 | [Artist links](08-artist-links.md) | MusicBrainz artist pick, Twitch channel (REQUESTS.md) |
| 9 | [Cover songs](09-cover-songs.md) | Covers: link to the original (REQUESTS.md) |
| 10 | [Inbox organization](10-inbox-organization.md) | Better inbox organization (REQUESTS.md) |
| 11 | [Secret mode](11-secret-mode.md) | Secret mode, secret source (REQUESTS.md) |
| 12 | [Add a song by search](12-add-song-by-search.md) | Add a song by search (REQUESTS.md) |
| 13 | [Trim silence](13-trim-silence.md) | Trim silence at the end (REQUESTS.md) |
| 14 | [YouTube Music source](14-youtube-music-source.md) | YouTube Music instead of spotDL, without removing spotDL (REQUESTS.md) |
| 15 | [Remove a release](15-remove-release.md) | Remove a release I don't want (REQUESTS.md) |
| 16 | [Add an artist from MusicBrainz](16-add-artist-from-musicbrainz.md) | Add an artist from MusicBrainz (REQUESTS.md) |
| 17 | [UI rework](17-ui-rework.md) | The "Console" look: sidebar, phone tab bar, player, Vault button, ⌘K, Tags page |
| 18 | [Page redesign](18-page-redesign.md) | Every page rebuilt on a component kit to match the Console mockup |
| 19 | [Creator from the secret source URL](19-secret-creator-from-url.md) | The Download from URL form suggests the creator from the uploader in the address |
| 20 | [Share to the secret source](20-share-to-secret-source.md) | Share a page from iOS (Shortcut) to queue it as a secret track (REQUESTS.md) |
| 21 | [JSON API](21-json-api.md) | Read-only JSON endpoints for the mobile app (`docs/api.md`) |
| 22 | [Mobile app](22-mobile-app.md) | Expo app for iOS and Android: browse, play, queue, background audio, secret mode |
| 23 | [Add songs from a MusicBrainz link](23-add-from-musicbrainz-url.md) | Paste a MusicBrainz release or recording link on the Add page to list its tracks; add one or all |
| 24 | [Queue panel](24-queue-panel.md) | On wide screens the queue is always shown on the right (now playing, up next, chapters) |
| 25 | [Icons](25-icons.md) | Tab and header icons and an app icon on mobile; sidebar icons and a home-screen icon on the web |
| 26 | [One Add page, and covers from MusicBrainz](26-one-add-page.md) | Upload and Add merged into one page with a box that routes links; songs added from MusicBrainz get "Cover of" |
| 27 | [Volume control](27-volume.md) | Volume slider and mute in the web player |
| 28 | [Artist placeholder](28-artist-placeholder.md) | Artists without a picture show their newest release cover, else a pixel initials tile |
| 29 | [Sleep timer](29-sleep-timer.md) | Sleep timer in the web player and the mobile app |
| 30 | [Artist link fixes](30-artist-link-fixes.md) | MusicBrainz artist search by alias, visible manual link field (REQUESTS.md) |
| 31 | [Repeat](31-repeat.md) | Repeat off / all / one in the web player and the mobile app (REQUESTS.md) |
| 32 | [Queue whole releases](32-queue-more.md) | Play next / Add to queue for releases and playlists; Play next keeps order (REQUESTS.md) |
| 33 | [Cover on YouTube add](33-cover-on-youtube-add.md) | "This is a cover" when adding from YouTube (REQUESTS.md) |
| 34 | [Autoplay](34-autoplay.md) | Similar library tracks when the queue ends (REQUESTS.md) |
| 35 | [React web app](35-react-web.md) | The web UI rebuilt in React with context menus and an action palette (REQUESTS.md) |
| 36 | [Apple Music links](36-apple-music.md) | Import public Apple Music playlists and song links onto MusicBrainz releases (REQUESTS.md) |
| 37 | [Context menus and actions](37-context-menus.md) | Right-click / long-press menus, menus for releases, artists and playlists, ⌘K actions, UX audit (REQUESTS.md) |

## Rules for every plan

- Each numbered step is one commit. `make build`, `go vet ./...` and `go test ./...` pass after each one.
- Match the existing style: plain `database/sql` with SQL in `internal/db`, handlers in `internal/api`, templ components in `internal/components`, plain JS files in `assets/js` with no build step.
- Schema changes go only through the migration list from plan 0. Never edit an applied migration.
- Network calls to MusicBrainz, AcoustID and Cover Art Archive go through one client package. Tests use `httptest` fakes, never the real services.
- When a step finishes a feature, move its bullet in `FEATURES.md` from "Not implemented yet" to "Implemented".

## Decisions shared across plans

- **Placeholder tracks** (plan 0, step 6). A track can exist without a file (`tracks.has_file = 0`). Only plan 2 creates them, but every track query must handle them from plan 0 on.
- **Background workers** (plan 1). A `jobs` table worked by two goroutines: one for downloads, one for everything else (so a long download and a long MusicBrainz check don't block each other). New job types join one of these lanes; they don't start their own goroutines.
- **One rate limiter per external host** (plan 1). MusicBrainz is 1 req/s, shared by tagging and release tracking.
- **yt-dlp is the core download source** (plan 2, step 5a). It's driven by YouTube URLs the user pastes, and needs `yt-dlp` and `ffmpeg` on PATH.
- **Central track filter** (plan 0, step 6). All track reads use one SQL fragment or helper. Plan 5 extends it for secret tracks.
