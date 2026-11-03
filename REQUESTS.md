# Requests

Features asked for and not implemented yet. Implemented requests are removed from this file; `FEATURES.md` lists what exists.

## Mobile app (Expo): what's left

The Expo app in `mobile/` browses and plays the library (see `FEATURES.md`). Still open:

- **Offline downloads** of tracks for listening without a connection.
- **Uploading from the phone.**
- **A share extension** in the app, calling `POST /api/share`; until then the Shortcut from plan 20 covers sharing.
- **A standalone (EAS) build** instead of Expo Go.
- **Lock-screen artwork in secret mode:** the lock screen cannot send the vault cookie, so cover images show in normal mode only.

## Queue on phones

The ☰ popover is small; decide whether it becomes a full-screen sheet or part of an expanded player.

## Move YouTube imports onto their MusicBrainz release

A YouTube import (video or playlist) is auto-tagged, but it stays on the release chosen at import (often a single). When the applied match puts the recording on a different MusicBrainz release, for example an album, the track should be able to move there.

- **Offer, don't overwrite:** the "never overwrites what you set" rule stays the default. The Tagging page offers "Move to <release>" on matches whose release differs from the track's, and the track moves only when the user accepts.
- **The target release** is created from MusicBrainz as the Add page does (plan 23), with its artists, IDs and cover, or reused if the library has it. The other tracks of that release are not added; the release shows them as missing, like any partly owned release.
- **The old release** is deleted if the move leaves it empty and it was created by the import (a single named after the video).
- **Open:** whether matches scoring 90%+ should move without asking, as a setting.

## UI

What is left of the UX audit (`docs/ux-audit.md`) after plan 37: the quick fixes and the context menus are done.

- **Medium**
  - Sort control on the Tracks page (title, artist, added), like the secret Tracks page.
  - Filter box on the Releases and Artists card grids.
  - A type filter on the Search page.
  - On phones, make Playlists, Podcasts, Tagging and Downloads easier to find than "More".
  - Link the rows on the Downloads page to their release and artist.
- **Large**
  - A start page (continue listening, recently added, the inbox count) instead of opening on Tracks.
  - Edit releases, playlists and tracks in place or in a side panel instead of on separate pages.
  - Drag and drop to reorder playlists and tracklists.
  - Plan 35 (the React web app) stays on hold; revisit it only if these need more than templ and htmx can do.

## Search for new music to download

Find music that isn't in the library yet and queue it, by more than a single text box.

- **Search by field:** song title, artist, release (album, EP, single) name, with type and year filters, against MusicBrainz.
- **Browse results:** an artist shows its releases, a release shows its tracklist, with what's already owned marked.
- **Download from the results:** add one song, a whole release, or pick tracks, as placeholders with queued downloads in the usual source order.
- **Today:** the Add box searches MusicBrainz recordings by free text ("Artist - Title") and takes MusicBrainz links; this extends it.
