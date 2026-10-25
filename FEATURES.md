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

### Auto-tagging (MusicBrainz)

- Embedded MusicBrainz IDs, AcoustID fingerprint lookup (Chromaprint), text search fallback
- Background worker within the rate limits (MusicBrainz 1 req/s)
- Review page for uncertain matches, auto-apply above a threshold
- Cover Art Archive covers, MBIDs on artists and releases
- Skip podcasts and secret tracks; don't write tags into files (it breaks fingerprints)

### Artist release tracking

- Follow an artist (linked to their MusicBrainz artist ID) to track their full discography: albums, EPs, singles, compilations
- Background check for new releases within the MusicBrainz rate limit (1 req/s); compare against the library by MBID, ISRC and title
- Inbox of releases that aren't in the library yet, each with cover, type, date and tracklist; already-owned tracks marked
- One confirmation per release (or "confirm all") creates the release, its artists and tracklist, then queues the downloads
- Ignore a release once ("don't ask again"); per-artist filters (e.g. skip live albums, remixes, compilations)
- Auto-download confirmed tracks from the best available source, in order of quality: lossless before lossy, higher bitrate first
  - sources you have rights to: purchases (e.g. Bandcamp downloads), the artist's own free downloads, files already in a watched folder
  - pluggable source adapters, so each source is one small module
- Downloaded files go through the normal pipeline: fingerprint duplicate check, duration, placed on the confirmed release at the right disc and position
- Download queue with progress, retries, and a "couldn't find" list to resolve by hand (upload the file manually)
- Notifications for new releases waiting for confirmation

### Apps

- JSON API (`/api/v1/...`) for mobile and desktop apps
- Wrapped web app (Tauri, Wails, Capacitor) or native (React Native)
- Playback positions synced through the server

### Robustness and security

- File extension allowlist / content-type check on upload (an uploaded `.html` could run as the site), `nosniff`
- Upload size limits (`http.MaxBytesReader`), large uploads streamed to disk, server timeouts
- Faster MP3 duration from the Xing/VBRI header instead of walking every frame
- Prefill single and batch uploads from embedded tags
