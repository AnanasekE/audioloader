# Requests

Features asked for and not implemented yet. Implemented requests are removed from this file; `FEATURES.md` lists what exists.

## Mobile app (Expo): what's left

The Expo app in `mobile/` browses and plays the library (see `FEATURES.md`). Still open:

- **Offline downloads** of tracks for listening without a connection.
- **Uploading from the phone.**
- **A share extension** in the app, calling `POST /api/share`; until then the Shortcut from plan 20 covers sharing.
- **A standalone (EAS) build** instead of Expo Go.
- **Lock-screen artwork in secret mode:** the lock screen cannot send the vault cookie, so cover images show in normal mode only.

## UI

What is left of the UX audit (`docs/ux-audit.md`) after plan 37: the quick fixes, the context menus and the start page are done.

- **Large**
  - Edit releases, playlists and tracks in place or in a side panel instead of on separate pages.
  - Drag and drop to reorder playlists and tracklists.
  - Plan 35 (the React web app) stays on hold; revisit it only if these need more than templ and htmx can do.
