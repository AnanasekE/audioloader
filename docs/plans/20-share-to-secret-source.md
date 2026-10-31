# Plan 20: Share a secret source page from iOS

**Goal:** sharing a page of the secret source from Safari (or any app) through the iOS share sheet queues it as a secret track, without opening the web form. The entry point is an iOS Shortcut; a share extension in the Expo app (plan 22) can come later and call the same endpoint.

**Depends on:** plan 11 (secret source), plan 19 (creator from the URL).

## Status

- [x] 1. Share token config
- [x] 2. The share endpoint
- [ ] 3. Shortcut setup and docs

## Rules

As in plan 19: one commit per step; `make build`, `go vet ./...` and `make test` pass; no attribution lines in commits; never name the site in code, tests, comments, commits or docs (tests use `secrettest`'s fake host).

## Decisions

- **Endpoint:** `POST /api/share`, body form field `url` (also accept a JSON body `{"url": "..."}`, since Shortcuts sends either). It is not a cookie route: the browser session and vault cookie are ignored.
- **Auth:** `Authorization: Bearer <token>`, token from `AUDIOLOADER_SHARE_TOKEN`, compared with `crypto/subtle.ConstantTimeCompare` (compare SHA-256 hashes of both, so length doesn't leak). The feature is off (the route answers 404) when the token is unset, shorter than 32 characters (log once at startup why), the vault is off, or no secret source is configured. A wrong or missing token is 401 after a 1 s sleep, like a wrong passphrase.
- **What it does:** exactly what "Download from URL" does, as the secret viewer (`db.Secret`):
  1. `src.Normalize(url)`: wrong host, not https, etc. → the same messages as `secretSourceError`.
  2. Already in the library (`existingSecretTrack`) → 409 "Already in your library: <title>".
  3. `src.Look`. A listing (an uploader's page) → 422 "That's a list of recordings; share a single recording." No audio → the usual message.
  4. Creator: `src.Uploader` + `db.CreatorForUploader`, as plan 19; when there is no match, create (or find by name) the creator as the download POST does. Move that create-or-find block out of `downloadFromSecretSource` into a helper both use.
  5. **Fallback, no uploader in the URL:** don't queue. Answer 422 "No creator in that address. Finish it in the app:" followed by the absolute link to `/upload/secret-source?url=<escaped url>` (build it from `AUDIOLOADER_PUBLIC_URL` if set, else from the request's Host). The Shortcut opens that link. This replaces the "needs creators" waiting state from REQUESTS.md; it isn't built.
  6. Title, description and tags are the page's defaults (`page.Title` or, if empty, the last path segment unescaped; `page.Description`; `page.Tags`), with the same length limits as the form (cut, don't refuse).
  7. `downloads.EnqueueSecret`, then 202 "Queued: <title> by <creator>".
- **Responses are `text/plain`**, one line (two for the fallback), so the Shortcut can show them in a notification as-is. Never include anything from the library except the matched track's or creator's name, and only after the token passed.
- **Route registration:** a new `handleToken` helper in `routes.go` that adds the pattern to `Routes` and a new `TokenOnly` list. The leak test skips `TokenOnly` routes for its cookie checks; a dedicated test covers them instead.
- **Rate:** at most 30 shares a minute in total (a simple in-memory token bucket); over that, 429.

## Steps

### 1. Share token config (`internal/api`, `main.go`)
- `api.Register` takes the share token too (or read it in a small `share.go` via an exported `SetShareToken`, whichever fits `Register`'s current shape better). `main.go` reads `AUDIOLOADER_SHARE_TOKEN`.
- `shareToken(r) bool` does the constant-time check. Unit tests: right token, wrong token, missing header, `Bearer` with no token, token of a different length, feature off.

### 2. The share endpoint (`internal/api/share.go`, `routes.go`)
- `handleToken(mux, "POST /api/share", shareSecretPage)`, following the decisions above.
- Factor the create-or-find-creator block out of `downloadFromSecretSource` into `creatorForName(v, name)`; the form handler uses it unchanged in behaviour.
- API tests with the fake host and a fake page server (as the existing secret source tests do):
  - no token / wrong token → 401; token unset → 404; vault off → 404; source off → 404;
  - a normal page with a known uploader → 202, a job queued for `db.Secret` with that creator, and no new creator;
  - an unknown uploader → 202 and the creator is created once (sharing a second page by them doesn't duplicate it);
  - the same page twice while queued → still 202, one job (EnqueueSecret already dedupes);
  - a page already in the library → 409;
  - a listing → 422; wrong host → 400; a URL with no uploader → 422 with the `/upload/secret-source?url=` link;
  - a valid vault cookie without the token → 401 (the cookie never authorizes this route);
  - nothing in any response names a normal-mode track or artist.
- `leak_test.go`: skip `TokenOnly` routes in the cookie loops, and add a check that every `TokenOnly` route answers 401 with no token in both modes.

### 3. Shortcut setup and docs
- `docs/ios-share.md`: how to set the env var (`openssl rand -hex 32`), and a step-by-step for the Shortcut: "Show in Share Sheet" for URLs and Safari web pages → "Get Contents of URL" `POST https://<server>/api/share` with header `Authorization: Bearer …` and form field `url` → "If" the result contains "Finish it in the app" → "Open URLs" with the second line → otherwise "Show Notification" with the result. Don't name the site.
- `README.md`: add `AUDIOLOADER_SHARE_TOKEN` (and `AUDIOLOADER_PUBLIC_URL` if added) to the env var list, link `docs/ios-share.md`.
- `FEATURES.md`, secret mode: add "Share a page from iOS (Shortcut) to queue it as a secret track".
- `REQUESTS.md`: remove the "Share to the app from iOS" section, except a one-line note under the mobile app: "Share extension in the app, calling `POST /api/share`" (plan 22 may do it).
- `docs/plans/README.md`: add row 20, status "plans 0–20".
