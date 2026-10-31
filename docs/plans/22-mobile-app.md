# Plan 22: Mobile app (Expo)

**Goal:** an iOS (and Android) app that plays the library from the server over Tailscale, with background audio and lock-screen controls, in normal and secret mode.

**Depends on:** plan 21 (JSON API).

## Status

- [ ] 1. Project skeleton and server setup screen
- [ ] 2. API client and mode
- [ ] 3. Library screens
- [ ] 4. Player, queue and background audio
- [ ] 5. Episode positions
- [ ] 6. Secret mode in the app
- [ ] 7. Docs

## Rules

- Everything lives in `mobile/`. The Go build, `Makefile` targets and `Dockerfile` don't change, and `make test` doesn't need Node. Add `mobile/node_modules` to `.gitignore`.
- One commit per step. After each: `cd mobile && npx tsc --noEmit && npx jest` pass, and `npx expo export --platform web` builds (a smoke check, since iOS can't be built here).
- TypeScript, Expo SDK latest stable, `expo-router` for screens. Keep dependencies few; pin exact versions in `package.json`.
- No attribution lines in commits. Never name the secret source site.

## Decisions

- **Runs in Expo Go.** The user develops on Linux, so no Xcode builds: pick only libraries that work in Expo Go. Audio is **`expo-audio`** (background playback and lock-screen / Control Center metadata and controls through its lock-screen API; check the installed SDK's docs for the exact call, e.g. `setActiveForLockScreen`). Set `UIBackgroundModes: ["audio"]` in `app.json` for later standalone builds. No `react-native-track-player`.
- **Server URL** is entered once on first launch, checked with `GET /api/v1/me`, and kept in `expo-secure-store`. A settings screen can change it.
- **Auth is the vault cookie** (plan 21). React Native's `fetch` keeps cookies natively; make every request with `credentials: "include"`. If the cookie doesn't survive an app restart, store the `Set-Cookie` value from `POST /api/v1/vault` in secure-store and send it as a `Cookie` header — decide in step 2 with a test against the dev server, and write down which in `mobile/README.md`. Locking clears both.
- **Secret state never shows in normal mode:** all screens render from API responses only; on any mode change (unlock, lock, a `me` that says the mode changed) the app clears its query cache, stops playback and empties the queue, as the web does on a full reload.
- **Data fetching:** `@tanstack/react-query`, one hook per endpoint, in `mobile/src/api/`. The API types in `mobile/src/api/types.ts` match the field names fixed by plan 21's tests.
- **Queue** is local (in memory, plus the current queue saved to AsyncStorage so it survives a restart in the same mode; drop it on mode change).
- **Streaming** uses the `streamUrl` from the API with the cookie header; cover images use `imageUrl` with the same header.
- **Out of scope** (stay in REQUESTS.md): offline downloads, uploading from the phone, a share extension (the Shortcut from plan 20 covers sharing).
- **Look:** the "Console" design (dark, green accent) from plan 17; reuse its colour tokens from `styles/` as constants in `mobile/src/theme.ts`.

## Steps

### 1. Project skeleton and server setup screen
- `npx create-expo-app mobile --template` (TypeScript, expo-router), remove the template's sample screens.
- Tabs: Tracks, Artists, Playlists, Podcasts, Search; Releases in normal mode only (inside Artists or as a tab, hidden in secret mode).
- First-launch screen: server URL field, "Connect" calls `/api/v1/me`, shows a clear error if it fails.

### 2. API client and mode
- `mobile/src/api/client.ts`: base URL, `get<T>`, `post`, error type from `{"error"}`, cookie handling as decided above. Jest tests with a mocked `fetch`: URL joining, error parsing, cookie header.
- `useMe()` and a `ModeProvider` that exposes `secret`, `vault`, `secretSource` and resets caches and player on change.

### 3. Library screens
- Lists with paging (`FlatList`, `onEndReached`), detail screens for artist, release, playlist, show, and a search screen. Tapping a track plays it with the rest of the list queued after it, like the web.

### 4. Player, queue and background audio
- `mobile/src/player/`: one `expo-audio` player, a queue store (next, previous, play now, play next, add to end, remove, reorder), a mini player above the tabs and a full-screen player with seek.
- Lock-screen / Control Center: title, artist, artwork, play/pause, next, previous, seek. Audio mode set to play in background and in silent mode.
- Jest tests for the queue store (pure logic, no audio).

### 5. Episode positions
- Start episodes at `position`; save the position every 15 s and on pause through `POST /api/tracks/{id}/position`, and mark finished at the end, as `player.js` does. Reuse its thresholds.

### 6. Secret mode in the app
- Settings: "Unlock vault" (passphrase field → `POST /api/v1/vault`) and "Lock". Hide them when `vault` is false. Wrong passphrase shows an error.
- On unlock/lock, reset as decided above. A test for the reset logic.

### 7. Docs
- `mobile/README.md`: run with `npx expo start`, open in Expo Go on the phone over Tailscale, how to change the server, how auth works, what isn't done.
- `REQUESTS.md`: replace the mobile app section with what's left: offline downloads, uploading from the phone, share extension, a standalone (EAS) build.
- `FEATURES.md`: a "Mobile app" section.
- `docs/plans/README.md`: row 22, status "plans 0–22".
