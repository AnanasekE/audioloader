# Plan 4: Podcasts and long audio

**Goal:** episodes grouped into shows, a saved position per episode, played/unplayed state, "continue listening", playback speed, skip buttons and chapters.

**Depends on:** plan 3 (the queue and player API). Plan 1's `skipTrack` hook gets the podcast check.

## Schema (one migration)

```sql
ALTER TABLE tracks ADD COLUMN kind TEXT NOT NULL DEFAULT 'music' CHECK (kind IN ('music','episode'));
ALTER TABLE tracks ADD COLUMN show_id TEXT REFERENCES shows(uuid) ON DELETE SET NULL;
ALTER TABLE tracks ADD COLUMN published TEXT;   -- episode date

CREATE TABLE shows (
  uuid TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  author TEXT,
  img_url TEXT DEFAULT "/assets/img/default_track.png"
);

CREATE TABLE playback_positions (
  track_id TEXT PRIMARY KEY REFERENCES tracks(uuid) ON DELETE CASCADE,
  seconds REAL NOT NULL,
  finished INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE chapters (
  track_id TEXT NOT NULL REFERENCES tracks(uuid) ON DELETE CASCADE,
  start_ms INTEGER NOT NULL,
  title TEXT NOT NULL,
  PRIMARY KEY (track_id, start_ms)
);
```

SQLite's `ALTER TABLE ADD COLUMN` with `REFERENCES` only works if the default is NULL, which it is here. Create `shows` before altering `tracks`.

## Steps

### 1. Kind and shows
- **Upload forms:** a "Music / Episode" switch. Episode mode shows a show picker (with a "new show" option) and a date instead of release fields. Default to episode for files over 30 min, and let the user change it.
- **Import:** tags with genre "Podcast", ID3 `PCST` or `TGID`, or a duration over 30 min default to episode, grouped into a show by album tag.
- Episodes are not placed on releases. The music library views (`ListTracks`, releases, the artist track list) filter `kind = 'music'`; add this to the central track filter from plan 0.
- **Pages:** `/shows` (grid) and `/shows/{id}` (episodes, newest first, with played state).
- Add `kind = 'episode'` to `skipTrack` in the tagging job.

### 2. Per-episode position (server-side)
- `PUT /api/tracks/{id}/position` with `{seconds, finished}`. `player.js` sends it for episodes every 10 s, on pause, and on `pagehide` (use `navigator.sendBeacon` there).
- Mark `finished = 1` when within 30 s of the end, or on `ended`.
- Playing an episode resumes from the server position (embed it in the play button's data attributes). Music keeps the current localStorage-only behaviour.
- Add "Mark played / unplayed" buttons.

### 3. Continue listening
- On the index page: episodes with `0 < seconds` and `finished = 0`, ordered by `updated_at DESC`, limit 10, each with a progress bar.

### 4. Speed and skip
- **Player bar:** −15 s and +30 s buttons, plus a speed menu (0.8–2.0×). Show them only while an episode is playing.
- Store the speed per show in localStorage. Set `audio.preservesPitch = true` (the default in modern browsers, but be explicit).
- Media Session `seekbackward`/`seekforward` already exist; set their default offsets to 15 and 30.

### 5. Chapters
- Parse chapters server-side in `internal/audio`, next to `duration.go`, which already walks MP3 and MP4:
  - **ID3v2 `CHAP` frames:** start time and the `TIT2` sub-frame.
  - **MP4:** Nero `chpl` atom in `moov/udta`. QuickTime text-track chapters are optional and can wait.
- Parse on upload and on a one-time backfill job. Store the results in `chapters`.
- **Player:** a chapter list in the queue popover with click-to-seek, and the current chapter title under the track title.
- Test against small fixture files in `internal/audio/testdata/`. Generate them with `ffmpeg` and commit them.

## Out of scope
RSS feed subscription and auto-download of episodes. That could later be another source adapter from plan 2.
