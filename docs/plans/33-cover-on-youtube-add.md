# Plan 33: Mark a YouTube import as a cover

**Goal:** when adding a song from YouTube, say it is a cover and of what, instead of fixing it afterwards.

**Depends on:** plan 9 (cover songs), plan 26 (one Add page).

## Status

- [x] 1. "This is a cover" on YouTube imports

## Rules

As in plan 19.

## Decisions

- **Where:** every form that creates a track from a YouTube video in normal mode: the "Use this" form in `SongYouTubeCandidates`, and the form shown for a pasted YouTube / YouTube Music video link. Not playlists (a per-row cover field would bloat the page; the track menu's "Set cover of…" stays for those).
- **What:** a checkbox "This is a cover". Checked, it shows the same choices as the track edit form's `coverOfFields` (pick from the library, text, MusicBrainz recording URL), reused, not copied. Titles containing "cover" (case-insensitive, e.g. "Song (Cover)") tick it by default and prefill the text field with the title minus the cover part.
- **Saved** through the same db function the edit form uses, in the same transaction as the new track, so a failed cover save doesn't leave a half-made import.
- **Secret mode:** not shown (secret tracks have no "Cover of").

## Steps

### 1. "This is a cover" on YouTube imports
- Templ, handler changes in `internal/api/addsong.go` (and the pasted-link handler), tests for both paths with and without a cover.
- Headless Firefox screenshot of the form with the checkbox ticked, in the scratchpad.
- `FEATURES.md`, Covers. `REQUESTS.md`: remove the cover management entry.
