# Features

## Implemented

### Library

- Track list with play buttons, artists and where each track comes from ("Album · also a single")
- Artists page: add artists, artist cards with a "Batch upload" link
- Releases: album, EP, single and compilation, always typed explicitly (never guessed from track count)
- Releases page with separate shelves per type; singles shown as song rows, not album covers
- Album page: discs, track numbers, total length; single page centred on the song, with B-sides and "Also on <album>"
- Multi-disc releases
- Edit tracklist: reorder, change disc, add from library, remove
- One track can be on several releases (e.g. the single and the album)
- Edit a release's title, type, date and artists; delete a release (its tracks stay)
- Release covers (JPEG, PNG, WebP); tracks show the cover of their album, else EP, single, compilation
- Unique artist names (case-insensitive); existing duplicates merged
- Placeholder tracks: known from a tracklist, shown greyed out until their file is added

### Uploading

- Single upload: Single (creates the single release) or Album/EP/Compilation (creates the release, then batch upload)
- Optional release date on singles
- Artist picker with checkboxes and fuzzy search, multiple artists per track
- Batch upload: pick many files, one row each, title from the filename
  - per-row release ("New single" or an existing release) and disc
  - "Artist for all tracks" and "Release for all tracks"
  - parallel uploads, per-track status, retry only the failed rows
  - add a new artist without leaving the page
- Library import: pick a whole folder
  - tags read in the browser: ID3v2, Vorbis/Opus, FLAC, MP4, WAV
  - grouped into releases by album tag, or by folder for numbered untagged files
  - type, disc and track numbers from tags; otherwise a suggested type, one click to change
  - track numbers kept, so partial albums leave gaps for the missing tracks
  - regroup: group into release, move to release, make singles, remove, split
  - artists panel: new vs in library, add, rename everywhere, merge into existing
  - uploads 3 at a time; failed tracks can be retried

### Duplicates

- Fingerprint: file size + SHA-256 of the first 64 KiB
- Checked in the browser before upload and enforced on the server (unique index, 409)
- Already-stored files are linked into albums instead of uploaded again

### Auto-tagging (MusicBrainz)

- Background job queue (one worker, retries with backoff), MusicBrainz at 1 request/s
- Matching by embedded MusicBrainz IDs (Picard tags, read in the browser and on the server), then AcoustID fingerprints (`ACOUSTID_KEY` + `fpcalc`), then text search
- Matches scoring 90%+ are applied; 50–90% wait on the Tagging page (apply, reject, apply all above a score)
- Fills in MBIDs on tracks, artists and releases, ISRCs and missing release dates; never overwrites what you set, never writes into files
- Release covers from the Cover Art Archive
- Placeholder tracks are skipped

### Artist release tracking

- Follow an artist (linked to their MusicBrainz ID, picked from search results) with filters: release types, secondary types to skip (live, remix, ...)
- Daily background check of followed artists' discographies; "Check now"
- Inbox of releases missing from the library: cover, type, date, tracklist with owned tracks marked; Confirm, Confirm all, Don't ask again
- Confirming creates the release, its artists and tracklist; owned tracks are linked, the rest become placeholders with queued downloads
- Download sources, best first (lossless, then bitrate, then `AUDIOLOADER_SOURCES` order): a watched folder (`AUDIOLOADER_WATCH`, matched by MusicBrainz ID, ISRC or tags), yt-dlp from pasted YouTube URLs
- Downloads page: progress, failed (with yt-dlp's error), couldn't find; paste a YouTube URL or upload the file by hand
- Fill a release's missing tracks from a YouTube playlist: videos matched by title and length, checked before downloading
- Import from YouTube: a video or playlist as new tracks, onto a release or as singles
- Downloaded files go through the usual fingerprint and duration checks; a file already in the library merges the placeholder into it

### Playback

- Global player bar that keeps playing across pages (hx-boost + hx-preserve)
- Resumes track and position after a reload (localStorage)
- OS media controls (Media Session API)
- Streaming with Range requests (seeking)
- Duration detection: WAV, MP3, FLAC, Ogg Opus, MP4/M4A

### Tooling and UI

- Tailwind via the standalone CLI (no Node), dark mode
- air live reload with browser refresh; Makefile (`build`, `css`, `css-watch`, `dev`)
- Favicon and default track/artist images
- SQLite foreign keys enforced
- Versioned migrations (`PRAGMA user_version`), DB tests (`go test ./...`)

## Not implemented yet

### Library and editing

- Edit and delete tracks
- Artist pages and profile pictures
- Playlists (the type exists, nothing uses it)
- Search
- Turn the existing tracks without a release into singles

### Playback

- Queue, next/previous, shuffle, "play release"
- Maybe a Preact island for the player if it grows complex

### Podcasts and long audio (over 30 min)

- `kind` on tracks (music or episode) and shows grouping episodes
- Resume position per episode, played/unplayed, "continue listening"
- Playback speed, skip ±15/30 s, chapters (ID3 CHAP, MP4 chapters)

### Secret tracks

- `secret` flag, unlocked through a second/secret UI
- Server-side enforcement: signed cookie, every track query filters secret tracks
- Cover every leak: stream endpoint, counts, artist pages, duplicate check, player state, search

### Apps

- JSON API (`/api/v1/...`) for mobile and desktop apps
- Wrapped web app (Tauri, Wails, Capacitor) or native (React Native)
- Playback positions synced through the server

### Robustness and security

- File extension allowlist / content-type check on upload (an uploaded `.html` could run as the site), `nosniff`
- Upload size limits (`http.MaxBytesReader`), large uploads streamed to disk, server timeouts
- Faster MP3 duration from the Xing/VBRI header instead of walking every frame
- Prefill single and batch uploads from embedded tags
