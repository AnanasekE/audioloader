# Plan 11: Secret mode

**Goal:** unlocking the vault switches the app into a separate secret mode. Normal mode shows only normal tracks; secret mode shows only secret tracks. Secret tracks become a simpler kind of track, and can be downloaded from the secret source.

**Covers (REQUESTS.md):** Secret mode, Secret tracks are a different kind of track, Downloading secret tracks.

**Depends on:** plans 5–10. It comes last for the same reason as plan 5: it touches every read path that exists by then.

## Rule for this plan
**Never name the secret source's site** in code, comments, tests, plans, fixtures or commit messages. It's configured privately and always called "the secret source".

## Decisions
- **Background pages are normal-mode only.** Inbox, Downloads, Tagging, Follow, the YouTube imports and the release cover and MusicBrainz link tools don't exist in secret mode (their nav links are hidden, the routes return 404). Secret tracks are never tagged or matched, and secret downloads are their own job that no page lists, so nothing would be on those pages.
- **No releases for secret tracks.** Making a track secret removes it from its releases (with a confirm dialog that lists them). Making it public again leaves it without a release; "Turn tracks without a release into singles" can follow later.
- **Existing data:** one migration removes `release_tracks` rows of secret tracks and deletes releases left with only secret tracks (the tracks stay). It can't be undone: the migration first copies `database.db` to `database.db.bak-<version>`, and logs each removed release.

## Schema (one migration, after the cleanup above)
Built as migration 18 (the cleanup) and 19 (this schema). Besides what is shown, 19 adds `artists.secret` and `shows.secret` (created in secret mode; such an artist or show stays hidden from normal mode until it has a track, so a creator added before its first upload doesn't show up there), and recreates the search index with a `modes` bit mask (1 normal, 2 secret) in place of `secret`.
```sql
ALTER TABLE tracks ADD COLUMN description TEXT;
ALTER TABLE tracks ADD COLUMN image TEXT;          -- per-track picture in data/, secret tracks only
CREATE TABLE track_tags (
  track_id TEXT NOT NULL REFERENCES tracks(uuid) ON DELETE CASCADE,
  tag TEXT NOT NULL COLLATE NOCASE,
  PRIMARY KEY (track_id, tag)
);
```

## Steps

### 1. `db.Viewer` becomes a mode
- `Viewer{Mode}` with `Normal`, `Secret` and `All` (background jobs only). `Public` is renamed `Normal`.
- `trackCond`: `secret = 0` in normal mode, `secret = 1` in secret mode, `1` for All.
- Derived rules flip the same way: a release, artist or show is visible in a mode if it has at least one track of that mode (or no tracks at all, in normal mode only). Rewrite `releaseCond`, `artistCond`, `showCond` as "EXISTS a track of this mode" plus the empty case.
- The middleware sets `Secret` mode from the vault cookie, else `Normal`. `api.viewer(r)` stays the one accessor.
- Test the conditions directly with a seeded DB: each mode sees only its side.

### 2. Leak test, both ways
- Split `leak_test.go` into a table over two modes: normal mode must show no trace of the `ZZSECRET` seed; secret mode must show no trace of a `ZZPUBLIC` seed (title, UUIDs, artist only on public tracks).
- Routes that don't exist in secret mode must return 404 there. The route list still comes from `api.Routes`.
- Expect this to fail in many places first; fix them one page at a time.

### 3. Pages follow the mode
- Tracks, artists, playlists, podcasts, search, continue listening and counts all filter through the viewer, so most of this is step 1. Check each page by hand.
- **Releases page:** 404 in secret mode; the nav link is hidden.
- **Lists in secret mode:** tracks by date added (default), by creator, and by tag.
- **Playlists:** a playlist belongs to the mode it was created in (`playlists.secret`, same migration). A track of the other mode can't be added. Existing playlists with mixed tracks: the migration puts them in normal mode, and their secret entries stay hidden there, as now.

### 4. Distinct look
- `<body data-mode="secret">` switches an accent color token (Tailwind CSS variable) and shows a "Secret" badge in the nav next to Lock.
- A secret placeholder image in `assets/`, used for secret tracks without an uploaded image. Per-track image upload on the track edit form (plan 7), reusing the picture storage.

### 5. Uploads in secret mode
- Single, batch, import and episode uploads in secret mode create secret tracks. Remove the "secret" checkboxes; the server sets the flag from the mode, never from the form.
- Secret uploads skip release creation: the single upload form shows title, creators, description and tags only. Batch and import hide the release controls.
- "Make secret / public" stays in the track menu; it moves the track to the other mode (and off its releases, see Decisions).

### 6. Player state per mode
- The stored player state and queue carry the mode they were saved in. On page load, if `data-mode` differs, clear the queue and state. Locking and unlocking also clear them at once (extend `player.purgeSecret()` to a `player.reset()`).
- Track JSON already has `secret`; drop any track in the queue whose flag doesn't match the mode.

### 7. Secret source adapter (`internal/sources/secret`)
- **Config:** `AUDIOLOADER_SECRET_SOURCE` names the page host. If audio files live on a different host, `AUDIOLOADER_SECRET_SOURCE_MEDIA` lists the allowed media hosts (comma-separated). Without the first, the feature is off and its routes 404. Document both in `README.md` without an example value.
- **URL check:** `net/url`, `https` only, host must equal the configured host. Only the checked URL is fetched. The HTTP client refuses redirects to hosts outside the allowed list.
- **Page parsing:** fetch the page (size cap, timeout), find the audio URL in `og:audio` / `og:audio:secure_url`, then `<audio src>` / `<source src>`, else the first absolute URL in the page text (e.g. a player's inline script config) whose host is an allowed media host and whose path ends in an audio extension. The audio URL must be on an allowed host. Take `og:title`, then jPlayer's `.jp-title`, then `<title>`, and `og:description` or `.jp-description`, as editable defaults. If the media host needs it, send the checked page URL as `Referer`. Site-specific notes are kept locally, outside git.
- **Fetch:** stream to a temp file with a size cap, then the usual duration check.
- **Tests:** an `httptest` server as the fake host, fixture HTML pages written for the tests. Cases: wrong host, redirect off-host, audio on a disallowed host, missing audio, page defaults.

### 8. "Download from URL" in secret mode
- `GET /upload/secret-source`: paste a URL, "Check" shows the page's title and description as editable fields plus the creator picker (with "add new").
- Creators are always picked by hand; the form can't be submitted without at least one (checked on the server too).
- **Dedupe:** the page URL is the track's `source_url`; an existing one is reported before downloading. The fingerprint check runs as usual.
- Runs as a job on the downloads lane, so a slow file doesn't block the request; the page polls its status. The job never shows on `/downloads` (normal-mode only).

### 9. Docs
- `FEATURES.md`: rewrite "Secret tracks" for modes. `README.md`: the secret source variables, described generically.

## Verification
- The two-way leak test, plus the tests above.
- **Manual check:** with the vault locked, nothing secret anywhere. Unlock: the accent changes, only secret tracks show, Releases and Inbox are gone, the queue is empty. Download one track from the secret source with two creators. Lock: the queue clears, and the track is gone from every page.

## Out of scope
More than one secret source, downloading whole pages or creators' profiles, encryption at rest.
