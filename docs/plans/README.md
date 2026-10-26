# Implementation plans

Run the plans in this order. Each one assumes the ones before it are done.

**Status:** plans 0–9 are implemented. Optional items left out are listed under "Not implemented yet" in `FEATURES.md`. Plans 10–11 cover the rest of `REQUESTS.md` and aren't started.

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
