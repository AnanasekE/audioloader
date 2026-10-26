# Plan 7: Track editing and artist pages

**Goal:** the two "Not implemented yet" features that later plans build on: editing and deleting tracks, and artist pages with profile pictures.

**Covers (FEATURES.md, Not implemented yet):** Edit and delete tracks, Artist pages and profile pictures.

**Needed by:** plan 8 (artist page, pictures), plan 9 (track edit form).

## Schema (one migration)
```sql
ALTER TABLE artists ADD COLUMN image TEXT;  -- file name in data/, NULL = default picture
```

## Steps

### 1. Edit a track
- `GET /tracks/{track_id}/edit`: title, artists (the existing picker), kind (track/episode), secret toggle (only when unlocked). Reached from the track menu.
- `POST /ui/tracks/{track_id}` saves it in one transaction: title, then `track_artist_map` replaced. Rebuild the track's FTS row.
- Never touches the file, MBIDs or releases (the tracklist editor already handles releases).
- Test: artists replaced, search finds the new title, a locked viewer gets 404 on a secret track's edit page (add the route to the leak test seed).

### 2. Delete a track
- "Delete" in the track menu, with a confirm dialog that names the track and its releases.
- `db.DeleteTrack`: one transaction removes `release_tracks`, playlist entries, positions, downloads and tagging rows (check which already cascade), then the track. Remove its file from `data/` after the commit.
- A release left with no tracks stays (as when its tracks are removed in the tracklist editor today).
- Test: rows gone, file gone, release renumbering unaffected for the others.

### 3. Artist page
- `GET /artists/{artist_id}`: picture, name, MusicBrainz link if set, Follow status, then their releases by type (reuse the release shelves) and tracks not on any of their releases.
- Artist cards and artist names in track rows link here.
- Uses `artistCond` like the artist list; a secret-only artist returns 404 when locked. Add it to the leak test.

### 4. Profile pictures
- Generalize `internal/db/covers.go`: the cover storage (`SaveCoverImage`, type sniffing, `removeCover`) takes a kind (release or artist) so both share the code. Keep `data/` as the store.
- `POST /ui/artists/{artist_id}/image` uploads one (JPEG, PNG, WebP, checked by content, not extension); "Remove picture" returns to the default.
- `GET /artist-images/{artist_id}` serves it; `audio.Artist.ImgURL` points there when `image` is set, else the default image.
- Test: upload, replace (old file removed), remove.

### 5. Rename and merge from the artist page
- Rename (unique, case-insensitive, as now) and "Merge into…" (reuse `mergeDuplicateArtists` logic for one pair).

## Verification
- Tests above; the leak test passes with the new routes.
- **Manual check:** edit a track's title and artists, find it in search under the new title, delete another one, open an artist page and give it a picture.

## Out of scope
Editing a track's file or audio, bulk edits.
