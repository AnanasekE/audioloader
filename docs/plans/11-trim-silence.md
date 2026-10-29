# Plan 11: Trim silence at the end

**Goal:** "Trim silence" in the track menu shows how much silence the end of the file has ("4:12 → 3:31"), and on confirm cuts it off, keeping 1 s of tail, without re-encoding where possible. The track's stored length follows the file.

**Covers (REQUESTS.md):** Trim silence at the end.

**Depends on:** plan 0 (track files, `audio.ProbeDuration`), plan 4 (saved positions).

## Decisions (the "To decide" items)
- **Manual only.** Downloads and uploads are never trimmed automatically.
- **Matching unchanged.** Length matching against MusicBrainz (±3–5 s) keeps using the stored length, which is the trimmed one only after a manual trim. No "original length" column.
- **Fingerprint unchanged.** `file_size` / `head_hash` keep describing the original file, so uploading or downloading the untrimmed original again is still caught as a duplicate.
- **Copy first, re-encode only as a fallback.** Every format the app stores (MP3, FLAC, Ogg/Opus, M4A/MP4, WAV) has independently decodable audio packets, so `-c copy` with `-t` cuts at a packet boundary (≤ ~50 ms off). The result is checked with `audio.ProbeDuration`; only if it's more than 0.5 s off the cut point (or ffmpeg fails) is the audio re-encoded.
- **Synchronous request**, not a job: detection decodes the file once (a few seconds for a song). A 5-minute timeout covers long episodes. One trim at a time (a package mutex).
- **Both modes.** It works on any track the viewer can see (`handle` + `visibleTrack`), secret ones included.
- **Rules:** silence is `noise=-50dB`, `d=2` (2 s or more). Only the last silence counts, and only if it runs to the end of the file (no `silence_end`, or one within 0.5 s of the file's length). Cut at `start + 1 s`. Nothing to do if that saves less than 1 s, or if the silence starts in the first second (an all-silent file).
- **No schema change.**

## Steps

### 1. `internal/trim` package: detection
- New `internal/trim/trim.go`:
  ```go
  // ErrUnavailable is returned when ffmpeg isn't installed.
  var ErrUnavailable = errors.New("ffmpeg needs to be installed")

  // Client runs ffmpeg. FFmpeg comes from FFMPEG, defaulting to ffmpeg on PATH.
  type Client struct{ FFmpeg string }

  func New(ffmpeg string) (*Client, error)   // exec.LookPath, like ytdlp.New
  func Default() (*Client, error)            // sync.Once over New(os.Getenv("FFMPEG"))
  func SetDefault(c *Client, err error)      // for tests

  const (
      Noise  = "-50dB"
      MinGap = 2 * time.Second
      Tail   = time.Second  // kept after the silence starts
      MinCut = time.Second  // smaller savings aren't offered
  )

  // Silence is one quiet stretch; End is 0 when it runs to the end.
  type Silence struct{ Start, End time.Duration }

  // Plan is what a trim would do: Total is the file's length now, CutAt
  // the new length; CutAt 0 means there's nothing to trim.
  type Plan struct{ Total, CutAt time.Duration }
  func (p Plan) None() bool { return p.CutAt == 0 }

  // ParseSilences reads silencedetect's log lines
  // ("silence_start: 251.2", "silence_end: 260 | silence_duration: 8.8").
  func ParseSilences(log string) []Silence

  // TrailingCut applies the rules above to the silences of a file of length total.
  func TrailingCut(silences []Silence, total time.Duration) Plan

  // Detect plans the trim of the file at path.
  func (c *Client) Detect(ctx context.Context, path string) (Plan, error)
  ```
- `Detect`: `total` from `audio.ProbeDuration` on the opened file (the same reader the upload uses; no ffprobe). Then
  `exec.CommandContext(ctx, c.FFmpeg, "-hide_banner", "-nostdin", "-nostats", "-i", abs, "-map", "0:a:0", "-af", "silencedetect=noise=-50dB:d=2", "-f", "null", "-")`, with `abs` from `filepath.Abs` (never starts with `-`). Parse stderr; on a non-zero exit return the error with the last 500 bytes of stderr (copy `tail` from `ytdlp`).
- **Tests** (`internal/trim/trim_test.go`, no ffmpeg needed): `ParseSilences` on `testdata/silencedetect.log` (a real log captured once, with a middle and a trailing silence, `[silencedetect @ 0x…]` prefixes intact). `TrailingCut` table: trailing silence with no end → `CutAt = start+1s`; trailing silence with end at total → same; only a middle silence → `None`; saving < 1 s → `None`; silence from 0.3 s → `None`; no silences → `None`.

### 2. Cutting
- In `trim.go`:
  ```go
  // Cut writes src's first at of audio to dst (same extension), stream-
  // copied, re-encoding only if the copy fails or misses at by > 0.5 s.
  // It returns dst's length.
  func (c *Client) Cut(ctx context.Context, src, dst string, at time.Duration) (time.Duration, error)
  ```
- Copy: `-hide_banner -nostdin -y -i <src> -map 0 -map_metadata 0 -t <at in seconds, %.3f> -c copy <dst>`. `-map 0` keeps an embedded cover picture.
- Re-encode fallback: same, with `-c:v copy -c:a <codec>` from the extension: `.mp3` `libmp3lame -q:a 2`; `.m4a`/`.mp4` `aac -b:a 256k`; `.ogg`/`.opus` `libopus -b:a 160k`; `.flac` `flac`; `.wav` `pcm_s16le`. Unknown extension → error.
- Check the result with `audio.ProbeDuration(dst)`.
- **Test** (`TestCutWithFFmpeg`): `t.Skip` when `exec.LookPath("ffmpeg")` fails. Generate an 8 s FLAC (3 s tone, 5 s silence) with `ffmpeg -f lavfi -i "sine=frequency=440:duration=3,apad=pad_dur=5" -c:a flac`, in `t.TempDir()`. `Detect` → `CutAt` within 0.1 s of 4 s; `Cut` → length within 0.1 s of 4 s. No network either way.

### 3. Trimming a track
- `internal/db/tracks.go`:
  ```go
  // SetTrimmedLength stores a track's new length after its file was cut,
  // and moves saved positions past the end back to it.
  func SetTrimmedLength(id string, d time.Duration) error
  ```
  One transaction: `UPDATE tracks SET duration_seconds = ? WHERE uuid = ? AND has_file = 1` (0 rows → `ErrNotFound`), `UPDATE playback_positions SET seconds = MIN(seconds, ?) WHERE track_id = ?`. Seconds rounded with `d.Round(time.Second)`, as `AttachFile` does.
- `internal/trim/track.go`:
  ```go
  // Preview plans the trim of a track's stored file.
  func (c *Client) Preview(ctx context.Context, id uuid.UUID) (Plan, error)
  // Apply detects again (never trusting a cut point from the browser),
  // cuts into a temp file next to the original, renames it over the
  // original and stores the new length. A Plan with None() did nothing.
  func (c *Client) Apply(ctx context.Context, id uuid.UUID) (Plan, error)
  ```
  - Path from `db.TrackFilePath(id)`.
  - Temp file: `os.CreateTemp(db.TracksDir, ".trim-*"+ext)`. The name must not start with the track's UUID, or `TrackFilePath`'s `<uuid>.*` glob could find it. Remove it on any error.
  - Order: `Cut` → `os.Rename(tmp, path)` (same directory, atomic) → `db.SetTrimmedLength(id, newLength)`. A stream already open keeps reading the old inode; `http.ServeContent` sends the new mod time afterwards.
  - A package-level `sync.Mutex` held for the whole `Apply`.
- **Test** (`internal/trim/track_test.go`, skips without ffmpeg): `dbtest` DB with `db.TracksDir` set to a temp dir, add the generated FLAC with `db.AddTrack`, save a position of 7 s. `Apply` → file shorter, `GetTrack` length 4 s, position 4 s, fingerprint columns unchanged, no `.trim-*` left in the directory. A second `Apply` returns `None()`.

### 4. Track menu and handlers
- **Routes** (`internal/api/routes.go`, `handle`, both modes):
  - `GET /ui/tracks/{track_id}/trim` → `previewTrim`
  - `POST /ui/tracks/{track_id}/trim` → `applyTrim`
- New file `internal/api/trim.go`. Both handlers: `trackID`, `visibleTrack`, then `trim.Default()`; when it's unavailable, render the fragment with the error (status 200, so it shows in the menu). Context `context.WithTimeout(r.Context(), 5*time.Minute)`.
  - `previewTrim` renders `components.TrimPreview(t, plan, "")`.
  - `applyTrim` runs `Apply`, then `hxLocation(w, backTo(r, "/tracks"))`, as `deleteTrack` does. An ffmpeg failure gives 502 with the short reason; the file is untouched.
- **Components** (`internal/components/tracks.templ`):
  - In `trackMenu`, after "Edit": a button "Trim silence…" with `hx-get="/ui/tracks/<id>/trim"`, `hx-swap="outerHTML"` (the pattern of "Add to playlist…"), shown for every track with a file.
  - `templ TrimPreview(t *audio.Track, p trim.Plan, errMsg string)`: the error, or "No silence at the end", or `formatDuration(p.Total) + " → " + formatDuration(p.CutAt)` with a "Trim" button: `hx-post` to the same URL, `hx-confirm` "Cut <saved> of silence from the end of “<title>”? This can't be undone." and an `htmx-indicator` "Trimming…".

### 5. Docs
- `FEATURES.md`: add "Trim silence at the end" under Implemented, with the rules (-50 dB for 2 s, 1 s tail, manual, stream copy).
- `README.md`: `FFMPEG` is now also used for trimming. `docs/plans/README.md`: add this plan's row.

## Tests (API, `internal/api/trim_test.go`)
- `trim.SetDefault(nil, trim.ErrUnavailable)`: GET shows "ffmpeg needs to be installed", POST changes nothing.
- With real ffmpeg (skip otherwise): upload the generated file through `POST /ui/tracks`, GET shows "0:08 → 0:04", POST answers with `HX-Location`, and the track row now says 0:04.
- An unknown track ID gives 404; in normal mode a secret track gives 404 (`visibleTrack`).

## Verification
- `make build`, `go vet ./...`, `make test` pass after each step (with and without ffmpeg on PATH).
- **Manual check:** a song with a hidden track after a long gap: the menu shows e.g. "6:40 → 3:52"; confirm; playback ends ~1 s after the music, the length shows 3:52 on the track row and release page, and a saved position past the end is gone. A song without trailing silence says "No silence at the end".

## Out of scope
Trimming the start or the middle, automatic trimming of downloads and uploads, undo (the original isn't kept), batch trimming.
