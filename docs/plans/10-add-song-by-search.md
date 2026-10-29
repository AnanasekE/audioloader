# Plan 10: Add a song by search

**Goal:** type "Ella Red Muse" into one box and get the song: pick it from MusicBrainz's top 5 (a placeholder on its real release, downloaded in the usual source order), or, if MusicBrainz has nothing fitting, pick a YouTube video that becomes its own single.

**Covers (REQUESTS.md):** Add a song by search.

**Depends on:** plan 6 (source order, YouTube top-5 pick), plan 1 (MusicBrainz client, `db.Match` shapes).

## Decisions
- **Normal mode only.** Every new route uses `handleNormal`: secret tracks never come from MusicBrainz or YouTube.
- **One box, two query forms.** Text with ` - ` ("Ella Red - Muse") becomes the fielded query `artist:"Ella Red" AND recording:"Muse"`; anything else goes to MusicBrainz as free text with `dismax=true`, which matches the words against the recording and artist names.
- **The track goes on its release, not the whole release.** Picking a recording adds one placeholder at its real disc and position. The release is reused if the library already has it (by release MBID, then release group MBID, then `LibraryReleaseFor`), else created with its artists and MBIDs. The rest of the tracklist isn't added: that's what following and the inbox do.
- **Release choice:** the candidate shows the release `tagging.PickRelease` picks from the search result's releases (official, album first, earliest). The POST sends both IDs; the server re-reads that release and checks the recording is on it.
- **Slot taken** (the library release already has another track at that disc/position): the placeholder goes at the next free position of that disc instead of failing.
- **YouTube result = own single**, as YouTube import already does (`db.AddTrack` with no link). "Without a release" in REQUESTS.md means no MusicBrainz release; every track in this app sits on a release. Title and artist are pre-filled (cleaned video title; the `Artist - ` prefix or the channel without ` - Topic`) and editable, since one box can't be split reliably. Auto-tagging runs afterwards, as for any import.
- **Covers in the candidate list** are loaded by the browser from the Cover Art Archive (`<CoverArtURL>/release/<id>/front-250`, fallback image on error), as the inbox does. The stored cover is fetched by the existing cover job only after a pick.
- **No schema change.**

## Steps

### 1. Free-text recording search (`internal/mb`)
- `internal/mb/api.go`: add
  ```go
  // SearchSongs finds recordings for what a person typed: "Artist - Title"
  // as a fielded query, anything else as free text (dismax).
  func (c *Client) SearchSongs(ctx context.Context, text string, limit int) ([]Recording, error)
  ```
  Trim and collapse spaces; empty text returns `nil, nil`. Split on the first ` - `; with both sides non-empty: `query = "artist:" + Phrase(a) + " AND recording:" + Phrase(t)`. Otherwise `query = text`, plus `dismax=true`. Same `getJSON(ctx, "musicbrainz", c.mbURL("recording", q), &res)` path as `SearchRecordings`, so the 1 req/s limiter applies.
- `internal/tagging/tagging.go`: export the release choice and credits without changing behaviour:
  ```go
  func PickRelease(releases []mb.Release) *mb.Release { return pickRelease(releases, &audio.Track{}, "") }
  func MatchArtists(credits mb.Credits) []db.MatchArtist { return matchArtists(credits) }
  ```
- **Tests** (`internal/mb/mb_test.go`, `httptest` server like `TestRetryOn503`): `"Ella Red - Muse"` sends the fielded query and no `dismax`; `"Ella Red Muse"` sends `query=Ella Red Muse&dismax=true`; the existing `testdata/recording_search.json` decodes into 2 recordings with releases.

### 2. Placeholder from a recording (`internal/db`)
- New file `internal/db/addsong.go`:
  ```go
  // NewSong is a MusicBrainz recording to add as a placeholder track.
  type NewSong struct {
      RecordingMBID string
      Title         string
      ISRC          string
      LengthMs      int
      SpotifyURL    string
      Artists       []MatchArtist
      Release       *NewSongRelease // nil: the track becomes its own single
  }

  type NewSongRelease struct {
      MBID, ReleaseGroupMBID string
      Title, Type, Date      string // Type is ours: album, ep, single, compilation
      SpotifyURL             string
      Artists                []MatchArtist
      Disc, Position         int
  }

  // AddSong creates the placeholder with its artists (resolved by MBID, then
  // name, else created) on its release (reused or created), and a queued
  // downloads row. It returns the track's and the release's IDs, and whether
  // the release is new (to fetch its cover).
  func AddSong(s NewSong) (trackID, releaseID string, newRelease bool, err error)
  ```
- Inside one transaction:
  1. Resolve track and release artists with `resolveArtistTx`.
  2. Find the release: `SELECT uuid FROM releases WHERE mbid = ?`, then `WHERE release_group_mbid = ?`, then the `LibraryReleaseFor` title rule. Make a `libraryReleaseForTx(q querier, artistID, rgMBID, title string)` from the body of `LibraryReleaseFor` and have the old function call it, so both share the logic.
  3. Missing: `insertRelease` (type from `NewSongRelease.Type`, invalid → `audio.ReleaseSingle`), then the same `UPDATE releases SET mbid, release_group_mbid, spotify_url` with the "MBID already taken" check as `ConfirmInboxRelease`. With `Release == nil`, a single titled after the track, as `insertTrack` does.
  4. Insert the placeholder. Move the `INSERT INTO tracks(... has_file 0 ...)`, `track_artist_map` and `INSERT INTO downloads` lines of `ConfirmInboxRelease` into `insertPlaceholderTx(tx *sql.Tx, t InboxTrack, artistIDs []string) (string, error)` and use it from both functions.
  5. `insertReleaseTrack`; on `ErrSlotTaken` retry once at `MAX(position)+1` on that disc (read in the same tx).
- **Tests** (`internal/db/addsong_test.go`, `dbtest`): new release and artists created with MBIDs and a `queued` download; a second song with the same release group MBID lands on the same release; an artist matched by name gets the MBID; a taken slot moves to the next position; `Release == nil` creates a single.

### 3. MusicBrainz step (handlers, components)
- **Routes** (`internal/api/routes.go`, all `handleNormal`):
  - `GET /add` → `getAddSongPage` (the page; `?q=` pre-fills and searches at once with `hx-trigger="load"`)
  - `GET /ui/add/musicbrainz?q=` → `searchSongsMusicBrainz`
  - `POST /ui/add/musicbrainz` → `addSongFromMusicBrainz` (form: `recording_id`, optional `release_id`)
- New file `internal/api/addsong.go`.
- `searchSongsMusicBrainz`: `mb.Default().SearchSongs(ctx, q, 5)`. For each recording build a `components.SongCandidate`:
  ```go
  type SongCandidate struct {
      RecordingID, Title, Artists string // Artists: Credits.String()
      Length                     time.Duration
      ReleaseID, ReleaseTitle, Year, CoverURL string // from tagging.PickRelease; "" if none
      OwnedTrackID, OwnedReleaseID string // set when the library has it
      OwnedHasFile                 bool
  }
  ```
  "Already there": read `db.ListTracks(db.Normal)` once and call `db.OwnedTrack(library, db.InboxTrack{RecordingMBID, ISRC, Title, LengthMs, Artists})` (the inbox's rule: MBID, ISRC, then title + artist + length within 3 s). MusicBrainz errors give 502 with a short message, as the YouTube search does.
- `addSongFromMusicBrainz`: validate IDs with `validMBID`.
  - With `release_id`: `GetRelease`, find the medium track whose `Recording.ID == recording_id` (none → 400 "that recording isn't on this release"). Fill `NewSong` from it the way `tracking.Check` fills an `InboxTrack` (length: `t.Length`, else `t.Recording.Length`; first ISRC; `mb.SpotifyURL(t.Recording.Relations, "track")`), and `NewSongRelease` from the release (`tagging.ReleaseType(rel.ReleaseGroup)`, `mb.SpotifyURL(rel.Relations, "album")`, disc `max(medium.Position, 1)`).
  - Without: `GetRecording`, `Release: nil`.
  - Run the owned check again (another tab may have added it); if owned, 409 with the link text.
  - `db.AddSong`, then `downloads.Enqueue(trackID)`, and `tagging.EnqueueCover(releaseID)` when `newRelease`. Answer `hxLocation(w, "/downloads")`.
- **Components** (new `internal/components/addsong.templ`):
  - `AddSongPage(q string)`: page header "Add a song", one text input `name="q"` with `hx-get="/ui/add/musicbrainz"` on submit, results div.
  - `SongCandidates(q string, found []SongCandidate)`: up to 5 rows: cover (`size-14`, `onerror` fallback to `/assets/img/default_track.png`), title, artists, `formatDuration(Length)`, release title and year. Owned rows show "In your library" linking to `/releases/<OwnedReleaseID>` (or "Already queued" → `/downloads` when `!OwnedHasFile`) instead of "Add". Others: "Add" (`hx-post`, `hx-vals` with both IDs). Below the list: "None of these" → `hx-get="/ui/add/youtube?q=<q>"` into the same results div. No results: say so and load the YouTube step directly (`hx-trigger="load"`).

### 4. YouTube step
- **Routes** (`handleNormal`): `GET /ui/add/youtube?q=` → `searchSongsYouTube`; `POST /ui/add/youtube` → `addSongFromYouTube`.
- **Share the candidate row:** split `YouTubeCandidates` in `downloads.templ` into `templ youTubeCandidate(c ytdlp.Entry, expected time.Duration)` (thumbnail, title, channel, length, `lengthDiff`, `channelBadges`, Preview link) with the action buttons as `{ children... }`. `YouTubeCandidates` keeps its output; the add page passes its own buttons. Expected length is 0 here, so no diff is shown.
- `internal/downloads/youtube.go`: add
  ```go
  // GuessArtist is the artist a video is probably by: the "Artist - " prefix
  // of its title, else its channel without " - Topic".
  func GuessArtist(e ytdlp.Entry) string
  ```
  Test it next to `CleanTitle`.
- `searchSongsYouTube`: `downloads.Default().YT.Search(ctx, q, 5)` (503 with `YTErr` when missing). For each entry: `artist := GuessArtist(e)`, `title := CleanTitle(e.Title, []string{artist})`, and `db.TrackBySourceURL(ytdlp.VideoURL(e.ID))` for "already there". Render `components.SongYouTubeCandidates(found, titles, artists, ownedIDs)`.
- Each row's "Use this" opens (plain `<details>`, no JS) a small form: title and artist inputs pre-filled, hidden `video_id`, "Add" → `POST /ui/add/youtube`. Owned rows show "In your library" instead.
- `addSongFromYouTube`: `ytdlp.Parse(ytdlp.VideoURL(video_id))` (same URL check as everywhere), title and artist required and trimmed, `db.ResolveArtists(db.Normal, []string{artist})`, `TrackBySourceURL` check again (409), then `downloads.EnqueueImport(downloads.YouTubeImport{VideoID, Title, ArtistIDs})`. Answer `hxLocation(w, "/upload/youtube")`, where import jobs show.

### 5. Entry points and docs
- `TracksPage` header: an "Add song" link to `/add` next to "Upload track", in normal mode only (`if !secretMode(ctx)`).
- `SearchPage`: in normal mode with `q != ""`, a line under the header "Not in your library? Add “<q>” as a song" → `/add?q=<url-escaped q>`.
- `FEATURES.md`: add the feature under Implemented. `README.md`: nothing new to configure. `docs/plans/README.md`: add this plan's row.

## Tests (`internal/api/addsong_test.go`)
Use `fakeMusicBrainz` (routes `/recording` → `testdata` search JSON copied from `internal/mb/testdata/recording_search.json`, `/release/` → `release.json`) and `downloadsHandler` (fake yt-dlp).
- MusicBrainz search lists the fixture's recordings with release title and year; a recording the library already has (same MBID) shows "In your library" and no "Add".
- POST with a release ID creates the placeholder on a new release with a queued `download` job; posting the same recording again gives 409; a recording not on the posted release gives 400.
- YouTube search renders the 5 fixture entries with pre-filled titles (`Massive Attack - Teardrop (Official Video)` → title `Teardrop`, artist `Massive Attack`); POST queues a `youtube_import` job with the resolved artist ID; a bad `video_id` gives 400.
- Secret mode: all five routes answer 404 (covered by the `NormalOnly` mode test once they use `handleNormal`; run it).

## Verification
- `make build`, `go vet ./...`, `make test` pass after each step.
- **Manual check:** search "Ella Red Muse", pick the album version: the release appears with one greyed placeholder that fills from the first source that has it. Search a song that's already in the library: it says so and links to it. Search nonsense: YouTube results appear; pick one, edit the title, and it imports as a single that then shows up on the Tagging page.

## Out of scope
Adding the rest of a release from here (follow the artist), recording search inside the secret mode, an inbox entry for the same release group staying "new" after one of its songs is added this way.
