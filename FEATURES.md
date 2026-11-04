# Features

## Implemented

### Library

- Track list with play buttons, artists and where each track comes from ("Album · also a single")
- Artists page: add artists, artist cards with a "Batch upload" link
- Add an artist from MusicBrainz (normal mode): search by name, the top 5 with the same details and Expand as the Follow page; picking adds it with its MusicBrainz name and ID, links a same-name artist without an ID, or links to the one already there; then Follow
- Releases: album, EP, single and compilation, always typed explicitly (never guessed from track count)
- Releases page with separate shelves per type; singles shown as song rows, not album covers
- Album page: discs, track numbers, total length; single page centred on the song, with B-sides and "Also on <album>"
- Multi-disc releases
- Edit tracklist: reorder, change disc, add from library, remove
- One track can be on several releases (e.g. the single and the album)
- Edit a release's title, type, date and artists; delete a release (its tracks stay), or "Remove release" on the release and edit pages: deletes the release with its tracks that are on no other release (files and queued downloads too, after a confirm with the counts) and moves its release group to Ignored in the inbox
- Release covers (JPEG, PNG, WebP); tracks show the cover of their album, else EP, single, compilation
- Unique artist names (case-insensitive); existing duplicates merged
- Placeholder tracks: known from a tracklist, shown greyed out until their file is added
- Edit a track (title, artists; an episode can become a song) from its menu; search follows the new title
- Delete a track from its menu, with a confirm dialog naming its releases; its file, playlist entries, positions, downloads and tagging rows go with it, the releases stay
- Artist pages (`/artists/<id>`): picture, MusicBrainz link, follow status, releases by type, and tracks not on any of their releases; artist names in track rows and on release pages link there
- Artist profile pictures (JPEG, PNG, WebP, checked by content), "Remove picture" drops it. An artist without one shows the cover of their newest release (normal mode only), else a pixel initials tile in a colour picked from the name
- Rename an artist (still unique, case-insensitive) or merge it into another from the artist page; follow, MusicBrainz ID, Twitch channel and picture move over when the target has none
- Artist links to MusicBrainz: the top 5 candidates show disambiguation, type, country and active years, and "Expand" lists a candidate's newest release groups (fetched only on request, under the 1 req/s limit); used on the Follow page and by "Link to MusicBrainz" on an artist page without an ID (finds artists by alias and sort name too, so "Hatsune Miku" finds 初音ミク, and shows the alias that matched; pick, or paste an ID or `musicbrainz.org/artist/<id>` URL; an ID another artist has offers a merge instead)
- Tagging page: "Link artists?" lists library artists without an ID that applied matches credit under a close but different name; the ID is only ever set from your pick, an exact embedded tag or an exact name match
- Twitch channel per artist (login or `twitch.tv/<login>` URL, stored lowercase), shown as a link on the artist page; saving it fetches the channel's profile picture for an artist with no picture of their own, "Update picture from Twitch" replaces it on request, never automatically. Needs `TWITCH_CLIENT_ID` and `TWITCH_CLIENT_SECRET`; without them the field still saves

### Uploading

- Track files must be MP3, FLAC, Ogg, Opus, M4A, MP4 or WAV (by extension, and the audio header is read), at most 4 GiB a request; every response is sent `nosniff`
- Upload a file (on the Add page): Single (creates the single release) or Album/EP/Compilation (creates the release, then batch upload)
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
- YouTube imports whose applied match is on another release (e.g. the album rather than the single made at import) get "On another release" on the Tagging page and "Move to <release>" in the track menu; Move creates the release from MusicBrainz (or reuses the library's), takes the placeholder's place if there is one, and removes the import-made single when it is left empty; Keep here hides the offer for that release. Nothing moves without a click, whatever the score

### Artist release tracking

- Follow an artist (linked to their MusicBrainz ID, picked from the top 5 search results) with filters: release types, secondary types to skip (live, remix, ...)
- Daily background check of followed artists' discographies; "Check now"
- Inbox of releases missing from the library, one compact row each with "3 of 11 in library"; cover and tracklist (owned tracks marked) open on demand
- Inbox tabs New and Ignored (move an ignored release back), grouped by artist in collapsible groups (remembered in the browser); Confirm, Confirm all (also per artist), Don't ask again (also works on confirmed releases, which Remove release uses)
- Inbox filters by type and "nothing owned yet", sort by date, search by title or artist; all kept in the URL
- Confirming creates the release, its artists and tracklist; owned tracks are linked, the rest become placeholders with queued downloads
- Download sources, tried in a fixed order (the first with a candidate wins; quality only breaks ties within a source): your own files (a watched folder, `AUDIOLOADER_WATCH`, matched by MusicBrainz ID, ISRC or tags; or a manual upload), then YouTube Music, then spotDL, then YouTube (yt-dlp)
- Downloads run in parallel: `AUDIOLOADER_DOWNLOAD_WORKERS` workers (default 2, at most 4); other jobs (tagging, release checks) keep their own worker
- spotDL: uses the Spotify album or track link from MusicBrainz when there is one (an album link is mapped onto the release's placeholders like a YouTube playlist); otherwise searches by artist and title and accepts the top result only if its title matches and its length is within 5 s
- YouTube Music (yt-dlp, `music.youtube.com` search with the songs filter, so official audio): searched by "artist title" for tracks with a known length, and downloaded automatically only on a confident match: length within 2 s, title score 0.9 or more (same cleaning as the playlist mapping), and one of the track's artists credited. The closest length wins; no match falls through to "Needs a pick". The flat search result has no lengths, so the top 3 are looked up one by one. A video already downloaded is placed, not fetched again
- YouTube search for missing tracks: when nothing else has the track, the download waits as "Needs a pick" (also counted in the nav). "Pick from YouTube" shows the top 5 results (thumbnail, channel, length and difference from the expected length, Topic and verified badges) with Use this, Preview and None of these; also on release pages
- Downloads page: progress, needs a pick, failed (with the tool's error), couldn't find; titles link to the release and artist names to their pages; paste a YouTube URL or upload the file by hand
- Fill a release's missing tracks from a YouTube playlist: videos matched by title and length, checked before downloading
- Import from YouTube (paste the link into the Add box): a video or playlist as new tracks, onto a release or as singles; the latest imports are listed on the Add page
- One Add page (`/add`, both modes; the sidebar "Add" button, the phone header icon and "Add" on the Tracks page; `/upload` redirects here, and `/upload/youtube` and `/upload/secret-source` redirect to `/add?q=<address>`): one box, "Search, or paste a link", sends what you type to the right place, and the result shows right below it. `/add?q=...` fills the box and looks it up on load, so links from the Search page and the uploader pages work. Below a divider, "Or upload a file" holds the upload forms (see below), with links to Batch mode and Import library. Where the box sends things:

  | Input | Mode | Goes to |
  |---|---|---|
  | MusicBrainz release or recording link | normal | the track list of that release or recording |
  | other MusicBrainz link | normal | "Paste a release or recording link." |
  | YouTube or YouTube Music video or playlist link | normal | the import preview (Import from YouTube) |
  | Apple Music song link | normal | the MusicBrainz recordings it may be, the sure one marked "Best match" |
  | Apple Music playlist link | normal | an import (see Apple Music import) |
  | a link of the secret source | secret | Download from URL's check |
  | any other link (in normal mode that includes the secret source) | any | "Audioloader can't add from that site." |
  | text | normal | MusicBrainz song search, then YouTube |
  | text | secret | "Paste a link from the secret source, or upload a file." (no source configured: "Upload a file.", and the box is hidden) |
  Hosts match in any case, with `www.` or `m.`, with or without `https://`.
- Song search: "Artist - Title" or free words. MusicBrainz's top 5 recordings show length, release and year, and the cover; "Add" puts a placeholder on its real release (reused if the library has it, else created with its artists and IDs) and queues the download in the usual source order. A song already in the library links to it, a queued one links to Downloads. "None of these", or no results, shows the top 5 YouTube results: "Use this" opens a form with the title and artist guessed and editable, and imports the video as a single
- Add from a MusicBrainz link: paste a release or recording link into the Add box to list exactly its tracks (disc and position shown, owned ones marked); "Add" per track puts it on that release, "Add all (n)" queues every track the library doesn't have (at most 100). Track and release-group links are refused; a bare ID is still a text search
- Find music (`/add/find`, normal mode; "Search MusicBrainz…" on the Add page, "Find music to download" in ⌘K): search MusicBrainz by song, release or artist. Song and release take optional Artist, Type (Album, EP, Single) and Year (`1994` or `1990-1999`) filters; the search is in the address, so a result page can be reloaded. Songs list length, release and year with "Add" ("Added — queued" and a link to the release), or "In your library" / "Already queued" (checked by recording ID); releases are cards with the library's count ("3/12 owned"); artists show disambiguation and "Following". An artist opens its release groups by type, newest first, 100 at a time ("More"); a release group opens the tracklist of its official release with every missing track ticked: "Add selected" or "Add whole release" (at most 100) put each on that MusicBrainz release as a placeholder with the usual queued download and "Cover of", and tracks the library has are marked and skipped. "Follow artist" adds the artist and leads to the Follow page
- Apple Music import (no Apple account or token; public pages only): paste a song link (`music.apple.com/<storefront>/song/...`, or an album link with `?i=<song>`) and its MusicBrainz candidates are listed, the sure one first as "Best match" (title score 0.9 or more, an artist credited, length within 3 s); "Add" is the usual one. Through the share Shortcut a song with a sure match is added right away; otherwise the answer is "Open the Add page to pick." with the link. Paste a playlist link and an import starts: a background job reads the page and matches every song on MusicBrainz (about one second each), and `/add/apple/<id>` shows the progress and then Matched (ticked), Unsure (unticked, with the best candidate and "Change" to pick another) and Not found ("Search" fills the Add box); songs the library has say "In your library". "Add selected" puts each song on its MusicBrainz release as a placeholder with a queued download, and "Also create playlist" makes (or, run again, completes) the audioloader playlist in Apple's order. The latest 5 imports are listed on the Add page. A public page lists only what Apple embeds: a playlist longer than that says "Apple Music shows the first N of M songs on its public page." An album link without a song is refused
- Apple Music sync: a finished import has "Keep in sync", which follows the playlist and marks its songs seen (by Apple's song ID, so it doesn't matter where Apple adds new ones). A job checks each follow every `AUDIOLOADER_APPLE_SYNC_EVERY` (a Go duration, default `1h`, at least a minute); a new song with a sure match is added like "Add selected" does (and goes into the import's playlist if it made one), the rest wait on the import page. Removed songs are ignored. The Add page lists the followed playlists with the last check or its error ("Not public any more", or "Apple Music shows the first N of M songs"), "Check now" and "Stop syncing" (the import stays). The Inbox has "Apple Music: N songs to pick" per follow, counted in its badge.
- Downloaded files go through the usual fingerprint and duration checks; a file already in the library merges the placeholder into it

### Playback

- Global player bar that keeps playing across pages (hx-boost + hx-preserve)
- Resumes track and position after a reload (localStorage)
- OS media controls (Media Session API)
- Streaming with Range requests (seeking)
- Duration detection: WAV, MP3, FLAC, Ogg Opus, MP4/M4A
- Queue: playing a track in a list queues the rest; next/previous, shuffle, Play and Shuffle on releases and playlists
- Play, Shuffle, Play next and Add to queue for a whole release, playlist or artist (placeholders skipped), from its ⋯ menu, with a short "Added n tracks" note; on cards and rows the tracks are fetched from the JSON API
- Autoplay: when the queue runs out, 10 similar tracks from the library (same artist, shared releases, release type and year, shared tags in secret mode; never recently played ones) are appended under an "Autoplay" divider and play on. A switch in the queue header, on by default, saved with the queue; things you add go before the autoplay tracks; `GET /api/v1/autoplay`
- Repeat button next to shuffle: off, all (back to the start, reshuffled when shuffling) or one (the track plays again; Next still moves on); saved with the queue
- Play next / Add to queue from a track's menu (several Play nexts in a row play in the order added); "Up next" list with remove and clear; queue survives a reload
- On wide screens the queue is always shown on the right: now playing, up next (click to play, ✕ to remove) and chapters
- Phone player (under 720px), Apple Music style: a floating mini player above the translucent tab bar (cover, title, play, next, a thin progress line; tap it to open the sheet), and a full-screen Now Playing sheet that slides up over a gradient tinted from the cover (it follows the back gesture; drag the handle down to close). It has big artwork that shrinks while paused, the title (linking to the release) and artists, the ⋯ track menu, a scrubber, previous / play / next, volume, the sleep timer, and for episodes −15 / +30, speed and the chapter. The Queue button swaps the artwork for Playing Next: the current track, Shuffle / Repeat / Autoplay pills, a History list above (scrolled out of view), and the upcoming tracks, which drag by their ≡ handle to reorder and swipe left to remove (a ✕ stays for the keyboard and screen readers); for an episode with chapters, Chapters take its place. The wide-screen queue panel has the same drag handles
- Volume slider and mute (desktop), remembered across reloads

### Podcasts and long audio

- Podcast episodes are tracks of their own kind, grouped into shows; kept out of the music library, tagging and track search (shows are searchable)
- Upload an episode into a new or existing show; files over 30 minutes switch to the episode form; the library import detects podcast tags (genre, ID3 PCST/TGID, MP4 pcst)
- Position per episode saved on the server (every 10 s, on pause, on leaving the page); resumes there; played/unplayed, mark by hand
- "Continue listening" on the Tracks and Podcasts pages
- Player for episodes: −15 s / +30 s, speed 0.8–2× remembered per show, pitch kept
- Chapters from ID3 CHAP frames and MP4 Nero chapters: list to jump to, current chapter under the title
- Sleep timer (moon button, web player): one click starts 30 minutes; 15–120 minutes or the end of the track/episode; volume fades over the last 10 s, then it pauses; counts wall-clock time and survives a reload

### Secret mode

- Vault at `/vault`: passphrase whose bcrypt hash is `AUDIOLOADER_SECRET` (`audioloader -hash-secret` makes one); signed, HttpOnly, SameSite=Strict cookie for 12 hours. Unlocking switches the whole app into secret mode; Lock in the nav returns to normal mode
- Two modes, never mixed: normal mode shows only normal tracks, secret mode only secret ones. Tracks, artists, shows, playlists, search, continue listening, counts, positions, chapters and the stream all follow the mode, and so does the duplicate check (a file on the other side gives a generic "upload failed")
- A release, artist or show belongs to a mode when it has a track of that mode; an empty one belongs to normal mode, except an artist or show made in secret mode, which waits there for its first track. A playlist belongs to the mode it was created in and takes only that mode's tracks
- Secret tracks are simpler: on no release, no "Cover of", never auto-tagged or matched as owned in the inbox. They have creators, a description, tags and an optional picture (JPEG, PNG, WebP; a placeholder otherwise)
- Secret mode's track list groups by date added (default), creator or tag. The normal Tracks page sorts by added (default), title or artist (`?sort=`)
- Filter box on the Releases and Artists grids (`assets/js/grid-filter.js`): hides the cards that lack any typed word of the title or artist names
- Releases, Inbox, Tagging, Downloads, Follow, "From YouTube" and the other background pages don't exist in secret mode: no nav link, and the routes answer 404
- Another look in secret mode: violet accent, a "Secret" badge in the top strip
- Tags page at `/tags` (secret mode only, 404 in normal mode): pick tags to include or exclude, match all or any, and see the tracks and the tags that are left with their counts. The state is in the URL (`?in=…&ex=…&match=any&sort=creator`), every control is a plain link that htmx upgrades to a swap of one region, and counting and filtering are SQL. Typing filters the chips (Enter includes the top one, Shift+Enter excludes it, Backspace in an empty box drops the last token); right-click or a long press excludes. On a phone the field shows the top 12 tags and an "All tags" sheet
- Uploads in secret mode (single, batch, import, episodes) create secret tracks; the server sets the flag from the mode, never from the form. The forms show title, creators, description and tags, with no release controls
- The player's saved state and queue belong to the mode they were saved in: a page of the other mode drops them, and Lock clears the player at once, with its lock screen metadata
- Download from URL (the Add box in secret mode, needs `AUDIOLOADER_SECRET_SOURCE`): paste a page of the secret source, Check fills in title and description to edit, the creator is filled in from the uploader in the address (an earlier track from the same uploader's page, else a creator with the uploader's name, else a "New: <name>" option created only when the download is submitted with it still checked); it can be changed, and at least one creator is required (checked on the server). One page request and one file request, only to the configured host and media hosts, https only, redirects off those hosts refused; the page URL is the track's source URL and an existing one is reported first; the usual fingerprint and duration checks apply. It runs as a background job that the page polls and that the Downloads page never lists
  - tags are prefilled from `[Tag]` markers in the title and description
  - addresses as pasted from a browser work too: http, no scheme, or `www.` in front; they're always fetched over https
  - a page without audio, such as an uploader's page, lists the pages it links one level below it: the ones not in the library first, each with a Download button, the ones already downloaded folded away. Only the listing is fetched until one is picked
- Share a page from iOS (Shortcut) to queue it as a secret track: `POST /api/share` with `Authorization: Bearer <AUDIOLOADER_SHARE_TOKEN>` (at least 32 characters; off without it, without the vault or without a secret source) and a `url` form field or JSON body. The vault cookie plays no part. It does what Download from URL does with the page's own title, description and tags, and the creator from the uploader in the address; a URL without an uploader is not queued, the answer links the form instead. Answers are one line of plain text, at most 30 shares a minute. See `docs/ios-share.md`
  - "Other pages from this uploader" (the page one level up) on a downloaded track's edit page, its creator's page and the "already in the library" message
- The migration that introduced the modes deleted releases made only of secret tracks (the tracks stayed); it first copied the database to `database.db.bak-18` and logged each removed release
- A test requests every GET route in both modes and fails on any trace of the other mode's seed; the routes that only exist in normal mode (or only with a secret source) are checked to answer 404 elsewhere

### Discovery

- Search (SQLite FTS5) over tracks, releases and artists: accents ignored, results while typing; chips on the page narrow it to Tracks, Releases, Artists or Podcasts (`?type=`)
- Playlists: create, add from a track's menu, reorder, remove, rename, delete; a track can be on a playlist more than once

### Cover songs

- A cover can point to its original: a library track, or text (title, artists, optional MusicBrainz recording ID). Reference only; the cover keeps its own credits
- "Cover of" on the track edit form (pick from the library, text, or a MusicBrainz recording URL, or remove) and "Set cover of..." in the track menu
- "Cover of X by Y" on track rows, release pages and in the player; deleting the original just unlinks the cover
- An original on the other side of the mode (secret or normal) is not shown, linked, indexed or listed
- Search also matches the original's title and artists (ranked lower); artist pages list "Covered by"
- Auto-tagging suggests the original from MusicBrainz (a recording's `cover` performance relation on a work); Apply or Reject on the Tagging page, never overwrites a set original
- Songs added from MusicBrainz (Add page, release or recording link, search) get "Cover of" from MusicBrainz: when the picked recording is a cover, a background job sets the original as text (title, artists, MusicBrainz ID), or links the library track that has that recording; it never overwrites a set original and skips secret tracks. Songs matched or added before are checked once at startup (a one-off job, in the non-download lane) and their covers become suggestions on the Tagging page
- Adding a song from YouTube (the "Use this" form, or a pasted video link) has a "This is a cover" checkbox that shows the same "Cover of" choices as the edit form; a title with "cover" in it ticks it and fills in the title without that part. The original is saved with the new track in one transaction. Not shown for playlists or in secret mode

### UI

- "Console" look: dark by default with a light theme (toggle in the sidebar, remembered in `localStorage`), all colours as tokens in `styles/input.css`, system fonts only. Secret mode swaps just the accent (violet)
- Left sidebar with Library (Tracks, Releases, Artists, Playlists, Podcasts) and Queue (Inbox, Tagging, Downloads with counts), a top strip with the search, dense track rows with monospace numbers, status chips on the queue pages
- Phone (under 720px): bottom tab bar (Tracks, Releases, Artists, Inbox, Library; in secret mode Tracks, Tags, Creators) and a Library sheet of large rows for Playlists, Podcasts, Tagging, Downloads, Inbox and Add, with the sidebar's count badges and a dot on the tab while tagging reviews or downloads wait
- Search: `Ctrl K` / `⌘K` or `/` focuses it, arrow keys move through the suggestions, Esc closes them
- Actions in the search box (`assets/js/commands.js`): matching commands are listed under "Actions" above the results, and `>` first lists only commands. Add / upload, go to each page, check followed artists now, shuffle, repeat, autoplay, sleep timer, dark / light theme, open or lock the vault, keyboard shortcuts. Enter or a click runs one
- Context menus: right-click (long-press on touch, the Menu key or Shift+F10 on a focused row) on a track, release, artist or playlist opens its ⋯ menu at the pointer, or as a bottom sheet on phones; Shift+right-click keeps the browser's menu. One menu is open at a time; arrow keys move, Esc closes
- Menus for releases (Play, Shuffle, Play next, Add to queue, Edit, Edit tracklist, Remove), artists (Play all, Play next, Add to queue, Edit / rename, Follow, Link to MusicBrainz) and playlists (Play, Shuffle, Play next, Add to queue, Edit, Delete), on cards, single rows and page headers
- `?` opens a dialog with the keyboard shortcuts
- Vault button in the sidebar and the phone top strip (when a vault is configured); in secret mode the same place has Lock
- Icons next to every sidebar link, the Add button, the phone tab bar and the More sheet, one icon per page
- Home-screen icon and web app manifest (Safari "Add to Home Screen", standalone, start page `/tracks`)
- Player with a play button, seek bar and elapsed/remaining time

### Tooling

- Tailwind via the standalone CLI (no Node)
- air live reload with browser refresh; Makefile (`build`, `css`, `css-watch`, `dev`)
- Favicon and default track/artist images
- `tools/make-icons.sh` regenerates the web and mobile app icons from `assets/img/favicon.png`
- SQLite foreign keys enforced
- Versioned migrations (`PRAGMA user_version`), DB tests (`go test ./...`)
- JSON API for the mobile app (read-only, both modes) under `/api/v1/`, with the same visibility rules as the pages; see `docs/api.md`

### Mobile app

- Expo app in `mobile/` (iOS and Android, runs in Expo Go over Tailscale): server address on first launch, tabs for tracks, artists, releases, playlists, podcasts and search, detail screens, paged lists
- Queue (play now, play next, add to end, remove, reorder) with a mini player and a full-screen player, background audio and lock-screen / Control Center controls
- Queue… on a release or playlist screen: Play next / Add to end of queue for the whole list (placeholders skipped); several Play nexts in a row play in the order added
- Autoplay switch in the full-screen player (on by default, saved with the queue): when the queue ends, 10 similar tracks from `GET /api/v1/autoplay` are appended under an "Autoplay" divider and play on; added tracks go before them
- Repeat button in the full-screen player: off, all or one (expo-audio's loop mode); the sleep timer's "end of this track" wins over repeat one; saved with the queue
- Episodes resume at their saved position and save it every 15 s, on pause and at the end
- Sleep timer in the full-screen player (first tap starts 30 minutes; 15–120 minutes or the end of the track/episode; fades over the last 10 s, then pauses), with a "23m" badge on the mini player. Background behaviour not verified on a device yet
- Normal and secret mode: unlock and lock the vault in Settings; switching mode clears the cache, stops playback and empties the queue

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

- Wrapped web app (Tauri, Wails, Capacitor) or native (React Native)
- Playback positions synced through the server

### Robustness and security

- Faster MP3 duration from the Xing/VBRI header instead of walking every frame
- Prefill single and batch uploads from embedded tags
