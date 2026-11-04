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

- **Large**
  - A start page (continue listening, recently added, the inbox count) instead of opening on Tracks.
  - Edit releases, playlists and tracks in place or in a side panel instead of on separate pages.
  - Drag and drop to reorder playlists and tracklists.
  - Plan 35 (the React web app) stays on hold; revisit it only if these need more than templ and htmx can do.

## Sync Apple Music playlists

Follow an Apple Music playlist, for example Favorite Songs, and download what gets added to it without pasting the link again.

- **Follow:** from a finished Apple Music import (`/add/apple/<id>`), "Keep in sync" saves the playlist link. A list of followed playlists, with the last check and "Stop syncing".
- **Check:** a background job reads each followed playlist on a timer (setting, default every hour) and finds songs not seen before.
- **New songs:** a sure match (the import's "Best match" rule) is added as a placeholder with a queued download, and to the audioloader playlist if the import made one. Unsure and not-found songs go to the Inbox to pick, as on the import page.
- **Removals:** a song removed from the Apple playlist is not deleted from the library.
- **Open:** Favorite Songs is a library playlist with no public page. Find out whether it can be shared as a public link; if not, it needs the Apple Music API with a MusicKit user token (an Apple Developer account), which the import avoids today.
- **Open:** a public page lists only the first N songs, so new songs past N are not seen. Check where Apple puts new songs (top or bottom) and what N is.
