# Share a page from iOS

An iOS Shortcut in the share sheet sends the page you are looking at to audioloader, which queues it as a secret track. Nothing opens in the browser unless the address names no creator.

The same Shortcut also takes Apple Music song links, see [Apple Music songs](#apple-music-songs).

## Server

1. Make a token: `openssl rand -hex 32`.
2. Set it as `AUDIOLOADER_SHARE_TOKEN`. At least 32 characters; shorter ones leave sharing off (the log says so).
3. Sharing a secret source page also needs the vault (`AUDIOLOADER_SECRET`) and the secret source (`AUDIOLOADER_SECRET_SOURCE`). Without them, `/api/share` answers 404 for those pages. Without the token, it answers 404 for everything.
4. Optionally set `AUDIOLOADER_PUBLIC_URL` to the address your phone reaches the server at, so the link in the "creator needed" answer is right behind a proxy.

The token only opens `POST /api/share`. A secret source page is always shared as secret mode, an Apple Music link as normal mode. Anyone with it can queue pages (at most 30 a minute), but can't read the library.

## Shortcut

1. Open Shortcuts, make a new shortcut, and in its settings turn on **Show in Share Sheet**; accept **URLs** and **Safari web pages**.
2. Add **Get Contents of URL**:
   - URL: `https://<server>/api/share`
   - Method: `POST`
   - Headers: `Authorization` = `Bearer <your token>`
   - Request Body: `Form`, with a Text field `url` set to the **Shortcut Input**.
3. Add **If**: the result **contains** `Finish it in the app` or `Open the Add page`.
   - Then: **Split Text** (by New Lines), **Get Item from List** (Last Item), then **Open URLs**. That opens the form with the address filled in, so you can pick the creator.
   - Otherwise: **Show Notification** with the **Contents of URL**.

Answers are one line of plain text: `Queued: <title> by <creator>`, `Already in your library: <title>`, or the reason it failed. The track shows up in secret mode when the download finishes.

## Apple Music songs

Share a song from Apple Music (Share, Copy Link, or the share sheet) through the same Shortcut. Audioloader reads the public song page, looks the song up on MusicBrainz and, if one recording fits (same title, an artist credited, length within 3 seconds), adds it right away as a placeholder on its release with a queued download: `Queued: <title> by <artists>`. If it is already there: `Already in your library: <title>`.

When no recording fits for sure, nothing is added. The answer is `Open the Add page to pick.` and the link to the Add page with the song filled in; the Shortcut opens it from the second line, as above. A playlist link answers `Open the Add page to import a playlist.` with the same kind of link. Album links without a song are refused.

You can test it from a terminal:

    curl -X POST https://<server>/api/share -H "Authorization: Bearer $TOKEN" -d url=<page address>
