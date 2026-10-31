# Requests

Features asked for and not implemented yet. Implemented requests are removed from this file; `FEATURES.md` lists what exists.

## Mobile app (Expo): what's left

The Expo app in `mobile/` browses and plays the library (see `FEATURES.md`). Still open:

- **Offline downloads** of tracks for listening without a connection.
- **Uploading from the phone.**
- **A share extension** in the app, calling `POST /api/share`; until then the Shortcut from plan 20 covers sharing.
- **A standalone (EAS) build** instead of Expo Go.
- **Lock-screen artwork in secret mode:** the lock screen cannot send the vault cookie, so cover images show in normal mode only.

## Share to the app from iOS (secret auto-download)

Sharing a page from the secret source in Safari (or another app) through the iOS share sheet sends it to audioloader, which downloads it as a secret track without opening the web form.

- **Entry point:** an iOS Shortcut that shows in the share sheet, which needs no app, or a share extension in the Expo app above. The Shortcut can come first.
- **Endpoint:** a POST that takes the page URL and queues the same job as the secret-mode "Download from URL" form. The same rules apply: only the configured host, https only, only the checked URL is fetched, and dedupe by `source_url` and fingerprint.
- **Never name the site** in the repo, as for the existing secret source.
- **Auth:** it must work without a browser session. A long random token in an env var (e.g. `AUDIOLOADER_SHARE_TOKEN`), sent as a header and compared in constant time. It is only valid for this one endpoint and always creates secret tracks.
- **Creator question decided:** from the URL, as the Download from URL form does since plan 19. A "needs creators" state, where shared tracks wait in secret mode until creators are picked (as Downloads holds "needs a pick"), stays only as the fallback for URLs without an uploader.
- **Feedback:** the share responds quickly with "queued" or a clear error (wrong host, already in library). The track appears in secret mode when the job finishes.
