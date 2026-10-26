# Plan 6: Download sources

**Goal:** confirmed releases fill themselves in from the best source that has each track: your own file, then spotDL, then YouTube. YouTube is never used on a guess: its top 5 results wait for a pick.

**Covers (REQUESTS.md):** Auto-download with spotDL, Download source order, Pick the right match from the top 5 (YouTube).

**Depends on:** plan 2 (sources, downloads, yt-dlp, playlist mapping).

## Decisions
- **Fixed source order** replaces quality-first ranking: `folder` (and manual uploads), then `spotdl`, then `yt-dlp`. Quality only breaks ties between candidates of the same source. `AUDIOLOADER_SOURCES` goes away (log a warning at startup if it's still set).
- **yt-dlp as last resort means "search, then wait for a pick"**, not "download result #1". The two requests conflict on this; the top-5 request wins, since a wrong download is worse than a missing one.
- **spotDL is only trusted when the match is exact or checked:** a Spotify link from MusicBrainz is exact; a spotDL search result must match title and length (±5 s, the playlist mapping rule).

## Schema (one migration)
```sql
-- downloads.status gains 'needs_pick': only YouTube results are left, waiting for a choice.
-- SQLite can't alter a CHECK: rebuild the table (create new, copy, drop, rename) in the migration.
ALTER TABLE releases ADD COLUMN spotify_url TEXT;   -- album link from MusicBrainz url-rels
ALTER TABLE tracks ADD COLUMN spotify_url TEXT;     -- track link, from url-rels or the album mapping
```

## Steps

### 1. Fixed source order
- Replace `sources.Rank` with an order by source position in a fixed list, then lossless, then bitrate. Keep it a pure function with a table test.
- `Downloader.Run` stops at the first source (in order) whose `Find` returns candidates, instead of asking all of them. That saves a slow spotDL search when the folder already has the file.
- Update `README.md` (drop `AUDIOLOADER_SOURCES`) and `FEATURES.md`.

### 2. Streaming links from MusicBrainz
- Add `url-rels` to the release lookup in `internal/mb` (`GetRelease`, and the `BrowseReleases` call that picks the concrete release for the inbox). Add a `Relations []Relation` field (`type`, `url.resource`) to `mb.Release` and `mb.Recording`.
- Keep only `open.spotify.com/album/<id>` and `open.spotify.com/track/<id>` links, parsed with `net/url` (strip `?si=` and any `intl-xx` path prefix). Store the canonical URL.
- Save them when the inbox release is built (in its `data` JSON) and copy them onto `releases.spotify_url` / `tracks.spotify_url` in `ConfirmInboxRelease`.
- Test with a fixture release JSON that has free-streaming, streaming and purchase relations.

### 3. spotDL adapter (`internal/sources/spotdl`)
- **Config:** `SPOTDL` env var (default `spotdl` on PATH), plus the `FFMPEG` already used. At startup run `spotdl --version` and log it; if it's missing, disable the source and say so on `/downloads`, like yt-dlp.
- **Safety:** same as yt-dlp: `exec.CommandContext` with arguments only, URLs after `--`, never user-supplied flags. Accept only `open.spotify.com` URLs of type track or album.
- **Metadata without downloading:** `spotdl save <url-or-query> --save-file <tmp>.spotdl` writes JSON with each song's name, artists, duration and Spotify URL. Use it for both search and album mapping. Check the exact JSON fields against the installed spotDL version first and pin them in a fixture.
- **Download:** `spotdl download <track-url> --format opus --output "<tmpdir>/{track-id}.{output-ext}"` (check the template variable name too). Opus keeps `duration.go` working. Take the one file it writes; anything else is an error. Timeout 10 min.
- **`Find`:**
  1. The track has `spotify_url`: one candidate, no search.
  2. Else search with the query `"<artist> - <title>"`. Accept the top result only if its normalized title matches and its length is within ±5 s. Reuse `internal/match` for the normalization.
- **Album mapping:** when a release has `spotify_url`, one `save` call gives the album's tracks. Map them onto the placeholders with the same function as YouTube playlists (`downloads.MapPlaylist`, generalized to take `{title, duration}` entries). Store each mapped track's `spotify_url` so its download needs no search.
- **Tests:** a fake `spotdl` script in `testdata/` that writes fixture `.spotdl` JSON and a small Ogg file. Point `SPOTDL` at it. Cover: exact link, search accepted, search rejected on length, album mapping.

### 4. YouTube search candidates
- `ytdlp.Client.Search(ctx, query, n)` runs `yt-dlp -J --flat-playlist -- "ytsearch5:<artist> - <title>"`. The query goes after `--` as one argument. Return `[]Entry` with thumbnail, channel and duration (add `Thumbnail` and `ChannelIsVerified`/`Channel` fields to `Entry` as the JSON gives them).
- The yt-dlp source's `Find` stays empty: YouTube is never picked automatically. Instead, when every other source finds nothing and yt-dlp is available, `Run` sets the download to `needs_pick` rather than `not_found`.
- Test with fixture search JSON.

### 5. Pick from YouTube
- `GET /ui/downloads/{track_id}/youtube/search` returns a fragment with up to 5 candidates, fetched when opened (an `hx-get` on the button), never ahead of time.
- **Per candidate:** thumbnail (loaded by the browser from YouTube's image host), video title, channel, length, and the difference from the expected length (e.g. "+2 s"). A badge for channels ending in " - Topic" and for verified/official channels.
- **Actions:**
  - "Use this" posts the video ID to the existing `POST /ui/downloads/{track_id}/youtube` (rebuild the canonical URL server-side, run the same URL check).
  - "Preview" links to `https://www.youtube.com/watch?v=<id>` in a new tab.
  - "None of these" sets `not_found`.
- **Where:** a "Pick from YouTube" button on Downloads rows (`needs_pick` and `not_found`), and on release pages next to each missing track.
- Add a "Needs a pick" tab to `/downloads` and include it in the nav badge count.

### 6. Docs
- `README.md`: `SPOTDL`, the new order, remove `AUDIOLOADER_SOURCES`.
- `FEATURES.md`: move "YouTube search for missing tracks" to Implemented and describe the source order.

## Verification
- Tests above, plus a `Run` test per order case: folder wins over spotDL; spotDL wins over YouTube; nothing but YouTube gives `needs_pick`; yt-dlp missing gives `not_found`.
- **Manual check:** confirm a release that has a Spotify album link on MusicBrainz. Tracks fill from spotDL with no search. Confirm one without links: some tracks match by search, the rest show "Needs a pick". Pick one, and it plays.

## Out of scope
Bandcamp, Spotify API credentials (spotDL handles its own), YouTube Music search (try it only if `ytsearch` results are poor).
