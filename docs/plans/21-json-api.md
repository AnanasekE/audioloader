# Plan 21: JSON API for the mobile app

**Goal:** read-only JSON endpoints (plus the few writes playback needs) so a native app can browse the library and play it, in both modes, with exactly the same visibility rules as the HTML pages.

**Depends on:** plans 0–20.

## Status

- [x] 1. JSON helpers and mode endpoints
- [ ] 2. Tracks and search
- [ ] 3. Artists, releases, playlists, shows
- [ ] 4. Leak test and docs

## Rules

As in plan 19. Plus: every handler reads through `db.Viewer` from the request (`viewer(r)`), never `db.Normal` or `db.Secret` directly. Reuse the `db` functions the HTML pages call; add a `db` function only when no page has one.

## Decisions

- **Prefix `/api/v1/`.** Registered with the existing `handle` / `handleNormal` / `handleSecretMode` helpers, so normal-only things (releases, covers) are 404 in secret mode exactly like their pages.
- **Auth is the vault cookie** (user's decision). The app unlocks with `POST /api/v1/vault` (`{"passphrase": "..."}` or form) → 204 with the same `Set-Cookie` as the web, 401 after the same 1 s sleep on a wrong passphrase, 404 when the vault is off. `POST /api/v1/vault/lock` → 204 with the clearing cookie. No new token.
- **`GET /api/v1/me`** → `{"secret": bool, "vault": bool, "secretSource": bool}`, so the app knows which mode it's in and what to show.
- **Shapes** live in `internal/api/apijson.go` as small structs with `json` tags (camelCase), built from `audio.*` types. Never encode `audio.*` structs directly: a new field there must not leak by accident. IDs are strings. Durations and positions are seconds (float). Each track carries `streamUrl` (`/api/tracks/{id}/stream`, which already exists and serves ranges), `imageUrl` (the track image, else the release cover in normal mode only, else ""), `artists: [{id, name}]`, and for episodes `position`, `finished`.
- **Lists** take `?limit=` (default 100, max 500) and `?offset=`, and answer `{"items": [...], "total": n}`. Sorts reuse the page's `sort` values where the page has them.
- **Errors** are JSON `{"error": "message"}` with the right status, via one `jsonError` helper; 404s stay plain `http.NotFound` from the mode wrappers (the app treats any 404 as "not there").
- **Queue is not on the server.** The web queue lives in the browser; the app keeps its own. Position saving reuses `POST /api/tracks/{id}/position`; check it accepts a JSON body or form, and accept both if it doesn't.
- **No writes** besides vault, lock and position in this plan.

## Endpoints

| Route | Wrapper | Answers |
|---|---|---|
| `GET /api/v1/me` | handle | mode flags |
| `POST /api/v1/vault`, `POST /api/v1/vault/lock` | handle | 204 / 401 / 404 |
| `GET /api/v1/tracks?sort=&q=&limit=&offset=` | handle | tracks with files |
| `GET /api/v1/tracks/{track_id}` | handle | one track, plus `releases`, `tags`, `description`, `chapters` |
| `GET /api/v1/search?q=` | handle | `{tracks, artists, releases, playlists, shows}`, each capped at 20 |
| `GET /api/v1/artists`, `GET /api/v1/artists/{artist_id}` | handle | artist list; one artist with its tracks (and releases in normal mode) |
| `GET /api/v1/releases`, `GET /api/v1/releases/{release_id}` | handleNormal | releases; one release with tracks in order |
| `GET /api/v1/playlists`, `GET /api/v1/playlists/{playlist_id}` | handle | playlists; one with tracks in order |
| `GET /api/v1/shows`, `GET /api/v1/shows/{show_id}` | handle | shows; one show with episodes, newest first, with positions |

## Steps

### 1. JSON helpers and mode endpoints
- `writeJSON(w, status, v)`, `jsonError(w, status, msg)`, `pageParams(r) (limit, offset)`.
- `/api/v1/me`, `/api/v1/vault`, `/api/v1/vault/lock`. Share the passphrase check and sleep with `unlockVault`.
- Tests: `me` in both modes and with the vault off; unlock right/wrong/off; lock clears the cookie.

### 2. Tracks and search
- Track list and detail, search. Tests: shape of one track (field names fixed by the test, so the app can rely on them), paging, a secret track is 404 in normal mode and vice versa, `imageUrl` never points at `/covers/` in secret mode.

### 3. Artists, releases, playlists, shows
- The rest of the table. Tests per resource: list and detail in both modes; releases 404 in secret mode; a playlist holding tracks of both modes lists only the viewer's.

### 4. Leak test and docs
- `leak_test.go` already walks every GET route in `Routes`; add `/api/v1/...` entries to its `shown` map for both modes (e.g. `/api/v1/tracks` must list "ZZSECRET Song" in secret mode), so the JSON checks aren't vacuous. Add `/api/v1/search?q=zz` and friends to `extra`.
- `docs/api.md`: the endpoints, shapes and auth, short.
- `FEATURES.md`: "JSON API for the mobile app (read-only, both modes)".
- `docs/plans/README.md`: row 21, status "plans 0–21".
