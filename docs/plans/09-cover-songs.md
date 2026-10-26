# Plan 9: Cover songs

**Goal:** a cover can point to its original song, as a reference only. Credits stay with the cover artist.

**Covers (REQUESTS.md):** Covers: link to the original song.

**Depends on:** plan 7 (track edit form).

## Design
- The original is either a track in the library, or a text reference (title, artists, optional MusicBrainz recording ID). Never both.
- Reference artists are text, not rows in `artists`. They don't create artists, don't count as credits and don't appear in artist listings.
- **MusicBrainz models covers on works, not recordings:** a cover recording has a `performance` relation to a work with the `cover` attribute. That's what the release page shows as "cover of". To name the original, look up the work (`inc=artist-rels+recording-rels`) and take the earliest non-cover recording's title and artist credit. Verify this on a real example before building step 4.

## Schema (one migration)
```sql
ALTER TABLE tracks ADD COLUMN original_track_id TEXT REFERENCES tracks(uuid) ON DELETE SET NULL;
ALTER TABLE tracks ADD COLUMN original_title TEXT;
ALTER TABLE tracks ADD COLUMN original_artists TEXT;  -- display text, e.g. "A & B"
ALTER TABLE tracks ADD COLUMN original_mbid TEXT;     -- MusicBrainz recording ID
```
SQLite allows a column `REFERENCES` in `ALTER TABLE ADD COLUMN` only with a NULL default, which this is.

## Steps

### 1. Data and display
- `audio.Track` gets `CoverOf *CoverRef` (title, artists, link). Load it in the track queries with a LEFT JOIN on `original_track_id`.
- **Secret rule:** a linked original that the viewer can't see is shown as plain text from its title, or not at all. Add the case to the leak test.
- Show "Cover of *Original* by *Artist*" under the title on track rows, release pages and in the player (track JSON gets a `coverOf` field). Link to the library track, or to `musicbrainz.org/recording/<mbid>`.

### 2. Setting it
- On the track edit form, a "Cover of" field with three modes:
  - pick a track in the library (reuse the search suggestions, tracks only, excluding itself)
  - title and artist as text
  - paste a MusicBrainz recording URL: fetch the recording and fill title and artist as text with the MBID
- "Remove" clears all four columns. Also reachable from the track menu ("Set cover of…").
- Test: the three modes, deleting the original sets the link to NULL, and the cover's own artists never change.

### 3. Search and artist pages (optional)
- Index `original_title` and `original_artists` in FTS as a separate, lower-ranked column, so searching the original's name finds covers.
- On an artist page, "Covered by": tracks whose original is one of this artist's tracks.

### 4. Suggest from tagging
- Add `inc=work-rels` to the recording lookup in tagging. If a matched recording has a cover performance relation and the track's "Cover of" is empty, look up the work as described above and store a suggestion.
- Show it on the Tagging page ("Cover of X by Y? Apply / Reject"), or apply it with the match when the match itself is applied. Never overwrites a set field.
- Test with fixture recording and work JSON.

## Verification
- Tests above.
- **Manual check:** set one cover by picking a library track, one by text, one by MusicBrainz URL. Delete the linked original and see the cover keep only its own credits.

## Out of scope
Remixes, samples and other derived relations.
