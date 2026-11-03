# Plan 36: Import from Apple Music links

**Goal:** paste a public Apple Music playlist link (for example a shared "Favourite Songs" playlist) or a song link on the Add page, and get its songs as placeholders on their MusicBrainz releases, with downloads queued in the usual source order. Optionally create the playlist in audioloader too.

**Depends on:** plan 12 (add a song by search), plan 23 (add from a MusicBrainz link), plan 26 (one Add box), plan 20 (share Shortcut).

## Status

- [x] 1. Read Apple Music pages (`internal/applemusic`)
- [x] 2. Song links in the Add box and the share Shortcut
- [ ] 3. Playlist import job
- [ ] 4. Playlist import page

## Rules

As in plan 19. Network calls to music.apple.com go through `internal/applemusic` with its own rate limiter (1 req/s) and a fake `httptest` server in tests; fixtures are trimmed real pages in `internal/applemusic/testdata`.

## Decisions

- **No Apple account, no API token.** Public pages embed their data as JSON in `<script type="application/json" id="serialized-server-data">`. In a playlist page, the section with `"itemKind": "trackLockup"` lists the songs: `title`, `artistName`, `duration` (ms), the album in `tertiaryLinks[0]` (`title`, album ID from its `contentDescriptor.identifiers.storeAdamID`), and the song ID in `contentDescriptor.identifiers.storeAdamID`. The playlist's name and `trackCount` are in the first section's item. Song pages (`/song/<slug>/<id>` and `/album/<slug>/<id>?i=<songID>`) carry the same fields for one song. Parse defensively: walk the JSON for `trackLockup` sections rather than fixed indexes, and fail with "Apple Music changed its page; this link can't be read." when nothing is found.
- **No ISRCs** are in the page, so matching is by text: MusicBrainz recording search `artist:"A" AND recording:"T"` (as `mb.SearchSongs` does for "A - T"), scored with the same title/artist cleaning as the YouTube Music source, plus the album title as a tie-breaker and length. **Confident** = title score ≥ 0.9, an artist credited, length within 3 s. The release is picked with `tagging.PickRelease`, preferring a release whose title matches the Apple album.
- **Long playlists:** a user playlist page embeds every track (a 244-song "Favourite Songs" page had all 244). If the parsed count is below `trackCount`, the import says "Apple Music shows the first N of M songs on its public page." Paging needs Apple's API token and is out of scope.
- **Links accepted:** `music.apple.com/<storefront>/playlist/<slug>/pl.<id>`, `.../song/<slug>/<id>`, `.../album/<slug>/<id>?i=<songID>`, any storefront, with or without `www.`, http or https. A bare album link (no `?i=`) answers 400 "Paste a song or playlist link." for now.
- **A song link** is looked up, then shown as the MusicBrainz candidates for "Artist - Title" with the confident one first and marked "Best match". Adding it is the existing `POST /ui/add/musicbrainz`. Through `POST /api/share` (normal mode), a song link with a confident match is added right away; without one, it answers 422 with "Open the Add page to pick." so the Shortcut shows it.
- **A playlist link** starts an import, because 100 songs at 1 req/s to MusicBrainz takes minutes. A job in the non-download lane fetches the page, then searches each song. Tables (one migration): `apple_imports(id, url, name, track_count, status, error, created_at)` and `apple_import_tracks(import_id, position, title, artist, album, duration_ms, recording_id, release_id, score, state)` where state is `pending`, `matched`, `unsure`, `not_found`, `added`, `owned`.
- **The import page** `/add/apple/<id>` shows progress while it runs (htmx poll every 2 s, stops when done), then three groups: Matched (ticked), Unsure (unticked, with the top candidate and Change), Not found (with "Search" that fills the Add box with "Artist - Title"). Already-owned songs show "In your library". "Add selected" uses the plan 23 per-track helper, so each song becomes a placeholder on its MusicBrainz release with a queued download. A checkbox "Also create playlist '<name>'" (on by default) creates the audioloader playlist in Apple's order with the owned and added tracks; running it again on the same import adds only what's missing.
- **Normal mode only,** like the rest of the Add page. Recent imports are listed under the Add box (latest 5) so a running one can be reopened.

## Steps

### 1. Read Apple Music pages (`internal/applemusic`)
- `ParseLink(s) (kind, id, storefront string, ok bool)`, `GetPlaylist(ctx, url) (Playlist, error)`, `GetSong(ctx, url) (Song, error)`, with table tests for links and fixture tests for both page kinds (including a page with no track section).

### 2. Song links in the Add box and the share Shortcut
- `classifyAddInput` routes `music.apple.com`; the song path; `/api/share` handling. Tests with fake Apple and MusicBrainz servers.

### 3. Playlist import job
- Migration, `internal/db` functions, the job type, the matcher with tests (confident, unsure, not found, owned, album tie-break).

### 4. Playlist import page
- Templ page, poll, Add selected, create playlist, recent imports list. API tests for add-selected and the playlist being created once.
- Headless Firefox screenshot of a finished import (fake data) at 1440×900 and 390×844, in the scratchpad.
- `FEATURES.md`: Apple Music import. `REQUESTS.md`: remove the Apple Music entry. `docs/ios-share.md`: Apple Music song links.
