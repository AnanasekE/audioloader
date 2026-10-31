# Requests

Features asked for and not implemented yet. Implemented requests are removed from this file; `FEATURES.md` lists what exists.

## Mobile app (Expo)

A native app for iOS (and Android, since Expo builds both) that talks to the same server, instead of using the web app in a browser.

- **Expo / React Native** in its own folder (e.g. `mobile/`), with no build step added to the Go app.
- **Connects over Tailscale** to the server URL entered once on first launch (e.g. `https://audioloader.<tailnet>.ts.net`).
- **Playback first:** library browsing (tracks, releases, artists, playlists, podcasts), a queue, background audio with lock-screen and Control Center controls, and resume of episode positions, like the web player.
- **Same modes:** normal and secret mode, with the vault unlocked in the app. Secret state never shows in normal mode, as on the web.
- **Needs a JSON API:** today the server mostly returns HTML for htmx. The app needs JSON endpoints for lists, track details, the queue and progress. These must follow `db.Viewer` exactly like the pages, and the leak test must cover them.
- **Auth:** the server has no login and relies on Tailscale. The app needs the vault cookie, or a token, for secret mode. Decide which.
- **Share extension** in the app, calling `POST /api/share` (plan 22 may do it).
- **To decide:** offline downloads of tracks for listening without a connection, and whether uploading from the phone belongs here.
