# Plan 2: Artist release tracking

**Goal:** follow artists, detect their releases that aren't in the library, confirm them in an inbox, and fill them in automatically from legal sources.

**Depends on:** plan 0 (placeholder tracks, `AttachFile`) and plan 1 (jobs, `internal/mb`, MBIDs).

## Schema (one migration)

```sql
CREATE TABLE artist_follows (
  artist_id TEXT PRIMARY KEY REFERENCES artists(uuid) ON DELETE CASCADE,
  -- JSON: {"skip_secondary": ["Live","Remix","Compilation"], "types": ["album","ep","single"]}
  filters TEXT NOT NULL DEFAULT '{}',
  last_checked TEXT
);

CREATE TABLE inbox_releases (
  release_group_mbid TEXT PRIMARY KEY,
  artist_id TEXT NOT NULL REFERENCES artists(uuid) ON DELETE CASCADE,
  release_mbid TEXT,            -- the chosen concrete release
  title TEXT NOT NULL,
  type TEXT NOT NULL,
  date TEXT,
  data TEXT NOT NULL,           -- JSON: cover url, tracklist [{disc,pos,title,recording_mbid,isrc,length,owned_track_id}]
  status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new','confirmed','ignored')),
  found_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE downloads (
  track_id TEXT PRIMARY KEY REFERENCES tracks(uuid) ON DELETE CASCADE,  -- a placeholder track
  status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','searching','downloading','done','not_found','failed')),
  source TEXT,
  progress REAL,
  last_error TEXT,
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

## Steps

### 1. Follow an artist
- Add an artist page section or button: "Follow". If the artist has no `mbid`, search MusicBrainz artists by name and let the user pick (`GET /ws/2/artist?query=`).
- Store the follow plus filters. The filter UI uses checkboxes for types and for secondary types to skip.

### 2. `check_artist` job
- Browse `GET /ws/2/release-group?artist={mbid}&limit=100&offset=N&fmt=json` until all pages are fetched. That's one request per page under the shared 1 req/s limiter.
- Apply the filters: map `primary-type` to our types (Album → album, EP → ep, Single → single; "Compilation" as a secondary type → compilation).
- **Diff against the library:**
  1. a release with that `release_group_mbid`
  2. else a release with the same normalized title and artist
  3. Skip release groups already in `inbox_releases` (any status, so ignored ones stay ignored).
- For each new group, pick a concrete release: `GET /ws/2/release?release-group={id}&inc=recordings+isrcs+artist-credits&fmt=json`. Prefer status Official, then digital media, then the earliest date.
- **Mark owned tracks** by recording MBID, then ISRC, then normalized title + artist + duration ±3 s.
- **Scheduler:** at startup and every 24 h, enqueue `check_artist` for follows with `last_checked` older than 24 h. Spread the jobs out using `run_after`.
- Test with fixture JSON: pagination, filters, owned-track marking, and ignored releases not reappearing.

### 3. Inbox (`GET /inbox`)
- Cards showing the cover (Cover Art Archive URL, loaded directly by the browser), title, type, date and tracklist, with owned tracks marked.
- Actions: **Confirm**, **Ignore** ("don't ask again"), and **Confirm all**.
- Nav badge with the count of `status = 'new'`. That count is the notification; browser notifications are optional.

### 4. Confirm a release (`db.ConfirmInboxRelease`)
One transaction:
- Create the release (with `mbid` and `release_group_mbid`) and resolve the artists by MBID or name.
- For each tracklist entry: link the owned track if there is one, otherwise create a placeholder track (`has_file = 0`, title, length, `mbid`, `isrc`), placed at disc and position.
- Insert `downloads` rows for the placeholders, then enqueue `fetch_cover` and one `download` job per placeholder.
- Test: owned tracks are linked rather than duplicated, and placeholders end up at the right positions.

### 5. Source adapters (`internal/sources`)
```go
type Candidate struct { Source, Ref string; Format string; Lossless bool; Bitrate int }
type Source interface {
    Name() string
    Find(ctx context.Context, want Want) ([]Candidate, error) // Want: title, artists, release, mbid, isrc, duration
    Fetch(ctx context.Context, c Candidate) (io.ReadCloser, string /*ext*/, error)
}
```
- **Ranking:** lossless first, then higher bitrate, then source priority order from config.
- **Core adapter: yt-dlp** (`internal/sources/ytdlp`). Driven by a YouTube URL the user pastes; see step 5a.
- **Second adapter: watched folder.** The `AUDIOLOADER_WATCH` env var points to a directory that is scanned recursively. Match files by embedded MBID, then ISRC, then normalized tags plus duration. Reuse the tag reader from plan 1.
- **Third adapter (optional): Bandcamp purchases.** There's no official API: it needs the user's own cookie and their collection page. Keep it behind a flag, and only for items the user owns. If it gets fragile, drop it.

### 5a. yt-dlp source
- **Config:** the `YTDLP` env var gives the binary path (default `yt-dlp` on PATH). It needs `ffmpeg` on PATH. At startup, run `yt-dlp --version` and log it. If either binary is missing, disable the source, show that on `/downloads`, and don't crash.
- **URL check:** accept only `youtube.com`, `www.youtube.com`, `m.youtube.com`, `music.youtube.com` and `youtu.be`, parsed with `net/url`. Run `exec.CommandContext` with arguments only, never a shell, and put the URL after `--`. Never pass `--exec`, and never pass user-supplied flags.
- **Metadata first:** `yt-dlp -J --flat-playlist -- <url>` returns the video or playlist entries, each with `id`, `title`, `duration`, `uploader` and `chapters`.
- **Download:** `yt-dlp -f bestaudio -x --audio-format opus --no-playlist --newline --progress-template "%(progress._percent_str)s" -o "<tmpdir>/%(id)s.%(ext)s" -- <url>`.
  - For an Opus source, `-x` with `opus` only remuxes into Ogg, which `duration.go` already handles. Plain WebM output would not be readable by `duration.go`.
  - Parse the progress lines into `downloads.progress`.
  - Timeout: 10 min per video.
- **Ranking:** a candidate is lossy, with its bitrate taken from `abr` in the JSON. A lossless source for the same track wins.
- **Dedupe:** migration adds `tracks.source_url TEXT` and stores the canonical `https://www.youtube.com/watch?v=<id>`. Before downloading, check `source_url`, then rely on the fingerprint check as usual.
- **Where URLs come from:**
  1. **Download row:** "Paste YouTube URL" on a queued or "couldn't find" row. It downloads that video into the placeholder via `AttachFile`.
  2. **Inbox release:** "Paste playlist URL" on a confirmed release. Map the playlist entries to tracklist slots by normalized title plus duration ±5 s, show the mapping for one-click confirmation, then enqueue one `download` per slot. Unmatched entries are listed and ignored.
  3. **Upload page:** "Import from YouTube". Paste a video or playlist URL, then use the same per-row release, artist and disc controls as batch upload. Title comes from the video title, with a "Artist - " prefix stripped when it matches a chosen artist.
- **Optional, later:** automatic search via `ytsearch5:<artist> - <title>`. Wrong matches are likely, so these results always need confirmation on the downloads page and are never applied automatically.
- **Breakage:** YouTube changes often break yt-dlp. Show a failed download's stderr tail in `last_error`, and add a note on `/downloads` that updating yt-dlp usually fixes it.
- **Tests:** a fake `yt-dlp` shell script in `testdata/` that prints fixture JSON and writes a small Ogg file. Point `YTDLP` at it. No network.

### 6. `download` job
- Ask all sources, rank the candidates, then fetch the best one to a temp file. Run it through the normal pipeline: `audio` duration plus `db.AttachFile`.
- If `AttachFile` returns `ErrExists`, the file is already in the library under another track. Merge: move the placeholder's `release_tracks` rows onto the existing track, then delete the placeholder.
- No candidates: set `not_found`. That's final; there's no automatic retry, though the folder check reruns when the watch folder changes, or once per day.
- Update the `downloads` row at each state change.

### 7. Downloads page (`GET /downloads`)
- Tabs: active, failed, couldn't find.
- **Failed:** a retry button.
- **Couldn't find:** an upload field per row that posts to `POST /ui/tracks/{id}/file` (which uses `AttachFile`). Uploading the file by hand resolves the row.
- Poll every 2 s while there are active rows (`hx-trigger="every 2s"`).

## Verification
- Fixture-based tests for the steps marked above.
- **Manual check:** follow an artist with a known discography, set a watch folder containing two of their missing tracks, confirm one release, and see those two filled in. The rest appear under "couldn't find". Paste a YouTube URL on one and it becomes playable. Paste a playlist URL on another release and check the slot mapping.

## Out of scope
Purchasing, automatic yt-dlp downloads without confirmation, and push notifications to phones (that's the apps phase).
