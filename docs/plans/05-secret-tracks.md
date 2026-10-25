# Plan 5: Secret tracks

**Goal:** tracks marked secret are invisible, in every way, unless the browser has unlocked the secret UI.

**Depends on:** plans 0–4. It comes last because it has to cover every read path that exists by then.

## Threat model
Someone using the app in a locked browser must not be able to learn that a secret track exists. That covers its title, artist, file, count, duration, position gaps, search hits, and the duplicate check. It does not protect against someone with server or filesystem access.

## Design
- **Flags:** `tracks.secret INTEGER NOT NULL DEFAULT 0`. A release, artist or show that contains *only* secret tracks is itself hidden. That's derived, not a separate flag.
- **Unlock:** `GET /vault` (unlinked) shows a passphrase form. The passphrase hash comes from the `AUDIOLOADER_SECRET` env var (bcrypt via `golang.org/x/crypto/bcrypt`). With no env var, the feature is off and the routes return 404.
- **Session:** on success, set the cookie `al_vault` to `exp|HMAC-SHA256(key, exp)`, with `HttpOnly`, `SameSite=Strict`, `Secure` when served over TLS, and a 12 h expiry. The key comes from `data/secret.key`, generated on first run with mode 0600.
- **Lock:** `POST /vault/lock` clears the cookie. Also add a lock button in the nav, visible only when unlocked.
- **Middleware:** puts `Viewer{Secret bool}` in the request context. Use one accessor, `api.viewer(r)`.
- **Central filter:** every db read function that touches tracks takes `v db.Viewer` and adds `(t.secret = 0 OR ?)` with `v.Secret`. No function reads tracks without a `Viewer` argument, so the compiler finds every call site.
- Add `t.secret = 1` to `skipTrack` in plan 1, and leave secret tracks out of plan 2's owned-track matching when building inbox data.

## Steps

### 1. Schema, flag and auth
- Migration to add the column, the vault routes, the cookie signing and verification, and the middleware.
- Test: tampered cookies, expired cookies, and the feature being off without the env var.

### 2. Thread `Viewer` through `internal/db`
- Change signatures one file at a time: tracks, releases, artists, shows, playlists, search, and continue listening.
- **Counts and durations:** compute them from filtered rows, never from stored totals.
- **Release positions:** a locked viewer sees secret tracks removed and positions shown as stored. A gap is a leak, so when locked, number the visible tracks sequentially in the view layer instead of using `position`.
- **Derived hiding:** releases, artists and shows with zero visible tracks are left out of lists, and their pages return 404 for a locked viewer.

### 3. Close the remaining endpoints
Work through each leak and add a test for each:
- [ ] `GET /api/tracks/{id}/stream` returns 404 when locked.
- [ ] `GET /ui/tracks/{id}`, the release, artist, show and playlist pages, and their `/edit` pages.
- [ ] Search (filter at query time, or keep secret rows out of the FTS table and query them separately when unlocked).
- [ ] `POST /api/tracks/exists` and the upload 409. When locked, a match against a secret track returns the same answer as a match against a non-existent one (`false`), and `AddTrack` then rejects with a generic "upload failed" error. That leaks less than a 409. The secret-only duplicate index still holds.
- [ ] `NextPositions` (the batch upload page) uses the unfiltered max position, but returns only what the release's visible data implies. Simplest fix: when locked, don't offer existing releases that contain secret tracks as upload targets.
- [ ] Playlists: secret entries are hidden when locked, and so are playlist counts.
- [ ] Inbox, downloads and tagging review pages: hide rows for secret tracks.
- [ ] Positions and continue listening.

### 4. Client side
- **Player:** on page load, the server renders `data-vault="0|1"` on `<body>`. If it's locked and the stored player state or queue has tracks marked `secret: true`, drop them. Include a `secret` field in the track JSON for this.
- Locking also clears that state immediately (`player.purgeSecret()`).
- Media Session metadata is cleared along with it.
- **Mark as secret:** a toggle on the track edit form and in the batch/import rows, visible only when unlocked.

### 5. Leak test
- One table-driven test: seed a library with one secret track (unique title `ZZSECRET`), on a release with one public track, by a secret-only artist, in a playlist. Then request every registered GET route as a locked viewer, using IDs from the seed for routes with path params. Assert that neither `ZZSECRET` nor the secret UUIDs appear in any body, and that the secret-only artist's page returns 404.
- Build the route list from `routes.go`, so new routes are covered automatically: export the list, or use a test that walks `mux`.

## Out of scope
Encrypting files at rest, multiple users.
