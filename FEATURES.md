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
- Edit a track (title, artists; an episode can become a song) from its menu; search follows the new title
- Delete a track from its menu, with a confirm dialog naming its releases; its file, playlist entries, positions, downloads and tagging rows go with it, the releases stay
- Artist pages (`/artists/<id>`): picture, MusicBrainz link, follow status, releases by type, and tracks not on any of their releases; artist names in track rows and on release pages link there
- Artist profile pictures (JPEG, PNG, WebP, checked by content), "Remove picture" returns to the default
- Rename an artist (still unique, case-insensitive) or merge it into another from the artist page; follow, MusicBrainz ID, Twitch channel and picture move over when the target has none
- Artist links to MusicBrainz: the top 5 candidates show disambiguation, type, country and active years, and "Expand" lists a candidate's newest release groups (fetched only on request, under the 1 req/s limit); used on the Follow page and by "Link to MusicBrainz" on an artist page without an ID (pick, or paste an ID or `musicbrainz.org/artist/<id>` URL; an ID another artist has offers a merge instead)
- Tagging page: "Link artists?" lists library artists without an ID that applied matches credit under a close but different name; the ID is only ever set from your pick, an exact embedded tag or an exact name match
- Twitch channel per artist (login or `twitch.tv/<login>` URL, stored lowercase), shown as a link on the artist page; saving it fetches the channel's profile picture for an artist who still has the default one, "Update picture from Twitch" replaces it on request, never automatically. Needs `TWITCH_CLIENT_ID` and `TWITCH_CLIENT_SECRET`; without them the field still saves

### Uploading

- Track files must be MP3, FLAC, Ogg, Opus, M4A, MP4 or WAV (by extension, and the audio header is read), at most 4 GiB a request; every response is sent `nosniff`
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

- Trim silence at the end (track menu, both modes; needs `ffmpeg`): finds the last stretch of silence (-50 dB for 2 s or more) when it runs to the end of the file, shows "4:12 → 3:31" and, after a confirm, cuts the file 1 s after the silence starts. Stream copy where the cut lands within 0.5 s, otherwise re-encoded; savings under 1 s are not offered. Manual only: downloads and uploads are never trimmed. The stored length and saved positions follow the file; the fingerprint stays the original's, so the untrimmed file is still caught as a duplicate, and length matching against MusicBrainz uses the stored length. The original isn't kept

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

- Follow an artist (linked to their MusicBrainz ID, picked from the top 5 search results) with filters: release types, secondary types to skip (live, remix, ...)
- Daily background check of followed artists' discographies; "Check now"
- Inbox of releases missing from the library, one compact row each with "3 of 11 in library"; cover and tracklist (owned tracks marked) open on demand
- Inbox tabs New and Ignored (move an ignored release back), grouped by artist in collapsible groups (remembered in the browser); Confirm, Confirm all (also per artist), Don't ask again
- Inbox filters by type and "nothing owned yet", sort by date, search by title or artist; all kept in the URL
- Confirming creates the release, its artists and tracklist; owned tracks are linked, the rest become placeholders with queued downloads
- Download sources, tried in a fixed order (the first with a candidate wins; quality only breaks ties within a source): your own files (a watched folder, `AUDIOLOADER_WATCH`, matched by MusicBrainz ID, ISRC or tags; or a manual upload), then spotDL, then YouTube Music, then YouTube (yt-dlp)
- spotDL: uses the Spotify album or track link from MusicBrainz when there is one (an album link is mapped onto the release's placeholders like a YouTube playlist); otherwise searches by artist and title and accepts the top result only if its title matches and its length is within 5 s
- YouTube Music (yt-dlp, `music.youtube.com` search with the songs filter, so official audio): searched by "artist title" for tracks with a known length, and downloaded automatically only on a confident match: length within 2 s, title score 0.9 or more (same cleaning as the playlist mapping), and one of the track's artists credited. The closest length wins; no match falls through to "Needs a pick". The flat search result has no lengths, so the top 3 are looked up one by one. A video already downloaded is placed, not fetched again
- YouTube search for missing tracks: when nothing else has the track, the download waits as "Needs a pick" (also counted in the nav). "Pick from YouTube" shows the top 5 results (thumbnail, channel, length and difference from the expected length, Topic and verified badges) with Use this, Preview and None of these; also on release pages
- Downloads page: progress, needs a pick, failed (with the tool's error), couldn't find; paste a YouTube URL or upload the file by hand
- Fill a release's missing tracks from a YouTube playlist: videos matched by title and length, checked before downloading
- Import from YouTube: a video or playlist as new tracks, onto a release or as singles
- Add a song by search (`/add`, normal mode only; "Add song" on the Tracks page, and a link on the Search page): "Artist - Title" or free words. MusicBrainz's top 5 recordings show length, release and year, and the cover; "Add" puts a placeholder on its real release (reused if the library has it, else created with its artists and IDs) and queues the download in the usual source order. A song already in the library links to it, a queued one links to Downloads. "None of these", or no results, shows the top 5 YouTube results: "Use this" opens a form with the title and artist guessed and editable, and imports the video as a single
- Downloaded files go through the usual fingerprint and duration checks; a file already in the library merges the placeholder into it

### Playback

- Global player bar that keeps playing across pages (hx-boost + hx-preserve)
- Resumes track and position after a reload (localStorage)
- OS media controls (Media Session API)
- Streaming with Range requests (seeking)
- Duration detection: WAV, MP3, FLAC, Ogg Opus, MP4/M4A
- Queue: playing a track in a list queues the rest; next/previous, shuffle, Play and Shuffle on releases and playlists
- Play next / Add to queue from a track's menu; "Up next" list with remove and clear; queue survives a reload

### Podcasts and long audio

- Podcast episodes are tracks of their own kind, grouped into shows; kept out of the music library, tagging and track search (shows are searchable)
- Upload an episode into a new or existing show; files over 30 minutes switch to the episode form; the library import detects podcast tags (genre, ID3 PCST/TGID, MP4 pcst)
- Position per episode saved on the server (every 10 s, on pause, on leaving the page); resumes there; played/unplayed, mark by hand
- "Continue listening" on the Tracks and Podcasts pages
- Player for episodes: −15 s / +30 s, speed 0.8–2× remembered per show, pitch kept
- Chapters from ID3 CHAP frames and MP4 Nero chapters: list to jump to, current chapter under the title

### Secret mode

- Vault at `/vault`: passphrase whose bcrypt hash is `AUDIOLOADER_SECRET` (`audioloader -hash-secret` makes one); signed, HttpOnly, SameSite=Strict cookie for 12 hours. Unlocking switches the whole app into secret mode; Lock in the nav returns to normal mode
- Two modes, never mixed: normal mode shows only normal tracks, secret mode only secret ones. Tracks, artists, shows, playlists, search, continue listening, counts, positions, chapters and the stream all follow the mode, and so does the duplicate check (a file on the other side gives a generic "upload failed")
- A release, artist or show belongs to a mode when it has a track of that mode; an empty one belongs to normal mode, except an artist or show made in secret mode, which waits there for its first track. A playlist belongs to the mode it was created in and takes only that mode's tracks
- Secret tracks are simpler: on no release, no "Cover of", never auto-tagged or matched as owned in the inbox. They have creators, a description, tags and an optional picture (JPEG, PNG, WebP; a placeholder otherwise)
- Secret mode's track list sorts by date added (default), creator or tag
- Releases, Inbox, Tagging, Downloads, Follow, "From YouTube" and the other background pages don't exist in secret mode: no nav link, and the routes answer 404
- Another look in secret mode: violet accent, a "Secret" badge in the nav
- Uploads in secret mode (single, batch, import, episodes) create secret tracks; the server sets the flag from the mode, never from the form. The forms show title, creators, description and tags, with no release controls
- "Make secret" / "Make public" in the track menu moves a track to the other mode. Going secret takes it off its releases (the confirm dialog lists them; a release left empty is deleted) and off playlists of the other mode; going public leaves it without a release
- The player's saved state and queue belong to the mode they were saved in: a page of the other mode drops them, and Lock clears the player at once, with its lock screen metadata
- Download from URL (secret mode, needs `AUDIOLOADER_SECRET_SOURCE`): paste a page of the secret source, Check fills in title and description to edit, you pick the creators (at least one, checked on the server). One page request and one file request, only to the configured host and media hosts, https only, redirects off those hosts refused; the page URL is the track's source URL and an existing one is reported first; the usual fingerprint and duration checks apply. It runs as a background job that the page polls and that the Downloads page never lists
  - tags are prefilled from `[Tag]` markers in the title and description
  - addresses as pasted from a browser work too: http, no scheme, or `www.` in front; they're always fetched over https
  - a page without audio, such as an uploader's page, lists the pages it links one level below it: the ones not in the library first, each with a Download button, the ones already downloaded folded away. Only the listing is fetched until one is picked
  - "Other pages from this uploader" (the page one level up) on a downloaded track's edit page, its creator's page and the "already in the library" message
- The migration that introduced the modes deleted releases made only of secret tracks (the tracks stayed); it first copied the database to `database.db.bak-18` and logged each removed release
- A test requests every GET route in both modes and fails on any trace of the other mode's seed; the routes that only exist in normal mode (or only with a secret source) are checked to answer 404 elsewhere

### Discovery

- Search (SQLite FTS5) over tracks, releases and artists: accents ignored, results while typing
- Playlists: create, add from a track's menu, reorder, remove, rename, delete; a track can be on a playlist more than once

### Cover songs

- A cover can point to its original: a library track, or text (title, artists, optional MusicBrainz recording ID). Reference only; the cover keeps its own credits
- "Cover of" on the track edit form (pick from the library, text, or a MusicBrainz recording URL, or remove) and "Set cover of..." in the track menu
- "Cover of X by Y" on track rows, release pages and in the player; deleting the original just unlinks the cover
- An original on the other side of the mode (secret or normal) is not shown, linked, indexed or listed
- Search also matches the original's title and artists (ranked lower); artist pages list "Covered by"
- Auto-tagging suggests the original from MusicBrainz (a recording's `cover` performance relation on a work); Apply or Reject on the Tagging page, never overwrites a set original

### Tooling and UI

- Tailwind via the standalone CLI (no Node), dark mode
- air live reload with browser refresh; Makefile (`build`, `css`, `css-watch`, `dev`)
- Favicon and default track/artist images
- SQLite foreign keys enforced
- Versioned migrations (`PRAGMA user_version`), DB tests (`go test ./...`)

## Not implemented yet

### Library and editing

- Turn the existing tracks without a release into singles

### Playback

- Maybe a Preact island for the player if it grows complex

### Downloads and podcasts

- Remove spotDL once YouTube Music has filled a good number of real releases without wrong picks: drop it from the source order, the downloader and the Downloads page, delete `internal/sources/spotdl`, remove Python and spotDL from the `Dockerfile` (about 400 MB) and `SPOTDL` from the README, and decide whether the stored Spotify links still serve anything
- Bandcamp purchases as a download source (behind a flag, with the user's own cookie)
- Library import: treat untagged files over 30 minutes as episodes (the single upload form already does)

### Apps

- JSON API (`/api/v1/...`) for mobile and desktop apps
- Wrapped web app (Tauri, Wails, Capacitor) or native (React Native)
- Playback positions synced through the server

### Robustness and security

- Faster MP3 duration from the Xing/VBRI header instead of walking every frame
- Prefill single and batch uploads from embedded tags
