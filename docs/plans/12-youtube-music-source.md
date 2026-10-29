# Plan 12: YouTube Music source

**Goal:** before a download falls back to "needs a pick", search YouTube Music's official songs ourselves with yt-dlp and download the result automatically when it clearly is the track (title, artist, and a length within 2 s of MusicBrainz's).

**Covers (REQUESTS.md):** YouTube Music instead of spotDL (the new source only; removing spotDL is the optional last step, not done now).

**Depends on:** plan 6 (source order, yt-dlp search, `needs_pick`).

## Decisions
- **New order:** `folder`, `spotdl`, `ytmusic`, `yt-dlp`. spotDL stays ahead while both exist, since a Spotify link from MusicBrainz is an exact match; `ytmusic` comes right before plain YouTube, as REQUESTS.md asks.
- **Plain YouTube stays pick-only.** `ytdlp.Source.Find` keeps returning nothing; when `ytmusic` isn't confident, the download still ends as `needs_pick`.
- **Confident means all of:** the expected length is known, the length is within 2 s, the cleaned title matches (`TitleScore ≥ 0.9`), and one of the track's artists is named by the result. Among results that pass, the smallest length difference wins. No length known → no automatic pick.
- **Same scoring as the existing matching:** `CleanTitle` plus `match.Similarity`, with and without bracketed parts. That's `MapTracks`'s title rule, moved into `internal/match` so `internal/sources/ytdlp` can use it (it can't import `internal/downloads`, which imports it). The top-5 picker itself has no score, only the length difference and badges, which stay as they are.
- **Same binary, same client:** the source wraps the existing `*ytdlp.Client` and its `Download`; there's no new env var. It's on whenever yt-dlp is.
- **Dedupe:** a `ytmusic` download stores `ytdlp.VideoURL(id)` as `source_url`, like a pick, so the same video is never stored twice.
- **No schema change.**

## Steps

### 1. Check yt-dlp's output and pin it
- Run once with the installed yt-dlp:
  `yt-dlp -J --flat-playlist --no-warnings -- "https://music.youtube.com/search?q=massive+attack+teardrop#songs"`
  and note which fields the flat entries have (`id` and `title` at least; `duration`, `channel`, `artists` may be missing in flat mode).
- Save a trimmed copy (5 entries, real field names) as `internal/sources/ytdlp/testdata/music_search.json`, using the video IDs that `search.json` already uses where you can. Write the yt-dlp version in a comment in the test.
- Add a case to `testdata/fake-yt-dlp`, before `*playlist*`: `*music.youtube.com/search*) cat "$here/music_search.json" ;;`.

### 2. Title scoring in `internal/match`
- New `internal/match/title.go`: move `CleanTitle` and its three regexes (`noise`, `dash`, `parens`) from `internal/downloads/youtube.go`. Keep `downloads.CleanTitle` as a one-line wrapper so callers don't change.
- Add:
  ```go
  // TitleScore is how well a video or song title fits a track title,
  // 0..1: the cleaned title against the track's, with and without
  // bracketed parts ("Song (Remastered)" is still the song).
  func TitleScore(candidate, title string, artists []string) float64
  ```
  It's the expression inside `MapTracks` today. Change `MapTracks` to call it, so behaviour stays the same.
- **Tests:** move the `CleanTitle` cases to `internal/match`; add `TitleScore` cases (exact 1; "(Official Video)" and "Artist - " prefix 1; "(Remastered 2019)" ≥ 0.9; a different song < 0.6). The `MapTracks` tests in `internal/downloads` must pass unchanged.

### 3. Music search on the client
- `internal/sources/ytdlp/ytdlp.go`:
  - Add `Artists []string `json:"artists"`` to `Entry`.
  - Add:
    ```go
    // MusicSearch returns up to n of YouTube Music's songs for query (the
    // "#songs" filter: official audio only), best first. Entries the flat
    // list gives without a length are looked up one by one (at most 3).
    func (c *Client) MusicSearch(ctx context.Context, query string, n int) ([]Entry, error)
    ```
  - Build the URL with `"https://music.youtube.com/search?" + url.Values{"q": {query}}.Encode() + "#songs"` and pass it to the existing `c.info(ctx, u)`, which puts it after `--`. Same query checks as `Search` (collapse spaces, non-empty, `1 ≤ n ≤ 20`). Cut to `n`, set `Thumbnail` as `Search` does.
  - If step 1 showed missing `duration` or `channel`/`artists`, fill the top 3 entries that lack them from `c.Info(ctx, VideoURL(e.ID))`. Skip this if the flat list has them; the plan doesn't need it then.
  - Add `// ArtistName is the channel name without " - Topic", e.g. for a song's artist.` `func ArtistName(e Entry) string` (uses `cmp.Or(e.Channel, e.Uploader)`).
- **Test** (`ytdlp_test.go`, `fake(t)`): `MusicSearch` returns the fixture's entries with thumbnails; a query of spaces is rejected without running yt-dlp.

### 4. The `ytmusic` source
- New `internal/sources/ytdlp/music.go`:
  ```go
  // MusicName is the source name stored on downloads.
  const MusicName = "ytmusic"

  // LengthTolerance is how far a song's length may be from the track's.
  var LengthTolerance = 2 * time.Second

  // MusicSource searches YouTube Music and offers its top song only when it
  // clearly is the track.
  type MusicSource struct{ C *Client }

  func (MusicSource) Name() string { return MusicName }
  func (s MusicSource) Find(ctx context.Context, want sources.Want) ([]sources.Candidate, error)
  func (s MusicSource) Fetch(ctx context.Context, c sources.Candidate, progress func(float64)) (sources.File, error) // same as Source.Fetch

  // BestSong picks the confident match from search results, if any.
  func BestSong(found []Entry, want sources.Want) (Entry, bool)
  ```
- `Find`: no title or `want.Duration <= 0` → `nil, nil`. Query is `<first artist> <title>` (as REQUESTS.md has it, no dash). `MusicSearch(ctx, q, 5)`, then `BestSong`. A match gives one candidate `{Source: MusicName, Ref: id, Label: VideoURL(id)}`.
- `BestSong`, for each entry:
  - `|Duration − want.Duration| ≤ LengthTolerance`
  - `match.TitleScore(e.Title, want.Title, want.Artists) ≥ 0.9`
  - Some `a` in `want.Artists` is `match.Same` as, or normalized-contained in, one of `e.Artists`, or `ArtistName(e)`, or the `Artist - ` prefix of `e.Title`.
  - Then the smallest length difference wins, ties going to the higher title score.
- Factor the body of `Source.Fetch` into an unexported `fetch(ctx, c *Client, id string, progress)` shared by both sources.
- **Tests** (`music_test.go`): `BestSong` table: exact song accepted; 3 s off rejected; right length but another artist rejected; "(Official Video)" title accepted; unknown length rejected; of two passing entries the closer one wins. `Find` against the fake picks the fixture's Topic song for "Massive Attack"/"Teardrop" at 5:31.

### 5. Order and downloader
- `internal/sources/sources.go`: `Order = []string{"folder", "spotdl", "ytmusic", "yt-dlp"}`, comment updated. Extend the `Rank`/`Position` table test.
- `internal/downloads/downloads.go`, `Default()`: when `d.YT != nil`, append `ytdlp.MusicSource{C: d.YT}` before `ytdlp.Source{C: d.YT}`.
- `Run`: after the source loop picks a candidate, if `cand.Source == ytdlp.MusicName`: `sourceURL = ytdlp.VideoURL(cand.Ref)`; if `db.TrackBySourceURL(sourceURL)` finds a track, `MergePlaceholder` and return (the same as the pasted-video branch). The existing `SetSourceURL` after `Attach` then stores it.
- **Tests** (`internal/downloads/downloads_test.go`, fake yt-dlp): a placeholder "Teardrop" (331 s) by Massive Attack downloads with source `ytmusic` and gets `source_url`; one whose length is 20 s off ends `needs_pick`; a second placeholder for the same video merges instead of storing a second file; with a folder source that has the file, `ytmusic` isn't asked (the fake records calls, or the folder test's order check).

### 6. Docs
- `README.md` and `FEATURES.md`: the new order, and that YouTube Music results are taken automatically only on a confident match.
- `docs/plans/README.md`: add this plan's row.

### 7. Optional, later: remove spotDL (do not do this now)
Only after `ytmusic` has filled a good number of real releases without wrong picks:
- Drop `spotdl` from `sources.Order`, `Downloader` (`SP`, `SPErr`, `mapAlbum`, `albumSongs`) and `getDownloadsPage`; delete `internal/sources/spotdl` and its tests.
- `Dockerfile`: remove Python and spotDL (about 400 MB).
- Decide whether `releases.spotify_url` / `tracks.spotify_url` still serve anything; if not, stop filling them, and leave the columns (never edit an applied migration).
- `README.md`: remove `SPOTDL`.

## Verification
- `make build`, `go vet ./...`, `make test` pass after each step. No test runs the real yt-dlp or touches the network.
- **Manual check:** confirm an inbox release without a Spotify link. Most tracks fill from `ytmusic` (the Downloads page shows the source); spot-check a few by listening. Tracks with an odd length (a live version, a long intro) show "Needs a pick" instead.

## Out of scope
Using YouTube Music results in the "Pick from YouTube" list, album mapping from YouTube Music playlists, removing spotDL (step 7, later).
