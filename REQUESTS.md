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
- **To decide:** offline downloads of tracks for listening without a connection, and whether uploading from the phone belongs here.

## Share to the app from iOS (secret auto-download)

Sharing a page from the secret source in Safari (or another app) through the iOS share sheet sends it to audioloader, which downloads it as a secret track without opening the web form.

- **Entry point:** an iOS Shortcut that shows in the share sheet, which needs no app, or a share extension in the Expo app above. The Shortcut can come first.
- **Endpoint:** a POST that takes the page URL and queues the same job as the secret-mode "Download from URL" form. The same rules apply: only the configured host, https only, only the checked URL is fetched, and dedupe by `source_url` and fingerprint.
- **Never name the site** in the repo, as for the existing secret source.
- **Auth:** it must work without a browser session. A long random token in an env var (e.g. `AUDIOLOADER_SHARE_TOKEN`), sent as a header and compared in constant time. It is only valid for this one endpoint and always creates secret tracks.
- **Conflicts with today's rule:** creators are always picked by hand and the form can't be submitted without one. Decide between:
  - a "needs creators" state, where shared tracks wait in secret mode until creators are picked, the same way Downloads holds "needs a pick";
  - taking the creator from the page, which today's rule forbids.
- **Feedback:** the share responds quickly with "queued" or a clear error (wrong host, already in library). The track appears in secret mode when the job finishes.
