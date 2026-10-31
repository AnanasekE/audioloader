# Audioloader mobile app

Expo (React Native, TypeScript, expo-router) app that browses and plays the library from the server's JSON API (`/api/v1`, plan 21). It runs in **Expo Go**, so no Xcode is needed.

## Run

```sh
cd mobile
npm install
npx expo start
```

Open the QR code in Expo Go on the phone. The phone needs to reach the server, so turn Tailscale on and use the server's tailnet address (for example `https://audioloader.<tailnet>.ts.net`, or `http://<tailscale-ip>:<port>`).

Checks: `npx tsc --noEmit`, `npx jest`, and `npx expo export --platform web` as a build smoke test.

## Server address

On first launch the app asks for the server address and checks it with `GET /api/v1/me`. It is kept in secure storage. To change it: Settings (top right), Change server. Changing the server drops the stored vault cookie, the cache and the queue.

## Auth and secret mode

The server has no login; it relies on Tailscale. Secret mode uses the vault cookie:

- Settings, "Unlock vault" sends the passphrase to `POST /api/v1/vault`; "Lock" calls `POST /api/v1/vault/lock`. Both are hidden when the server has no vault (`vault: false` in `/api/v1/me`).
- Every request uses `credentials: "include"`, so React Native's native cookie jar carries the cookie when it keeps it.
- As well, the `Set-Cookie` value from the unlock response is kept in secure storage and sent as a `Cookie` header on every request and on audio streams. This works whether or not the native jar survives an app restart, so it is always on. Locking deletes both.
- Not verified against a live server yet: whether the native cookie jar alone survives a restart. The header fallback makes that question moot.
- On every mode change (unlock, lock, or `/me` reporting a different mode) the app clears its query cache, stops playback and empties the queue, so secret state never shows in normal mode. The queue saved on disk is only restored in the mode it was saved in.

## Player

One `expo-audio` `AudioPlaylist` plays the queue, because it is the only expo-audio object whose next/previous buttons reach the lock screen. The JS queue (`src/player/queue.ts`, pure and tested) is the source of truth and is mirrored to the playlist. Background audio and silent-mode playback are set with `setAudioModeAsync`, and the lock screen / Control Center shows title, artist, artwork and play, pause, next, previous and seek through `setActiveForLockScreen`. `app.json` sets `UIBackgroundModes: ["audio"]` for standalone builds.

Long-press a track for "Play next" and "Add to end of queue". The full-screen player (tap the mini player) shows the queue with up, down and remove.

Episodes resume at their saved position and save it every 15 s, on pause, on switching track and at the end (`POST /api/tracks/{id}/position`), with the web player's rule: within 30 s of the end counts as finished.

## Sleep timer

The full-screen player has a "Sleep timer" button. With no timer running, the first tap starts 30 minutes at once and opens the menu; the menu offers 15 to 120 minutes in steps of 15, "End of this track/episode", "+15 min" and "Off". The countdown is wall-clock time (pausing does not stop it). A once-a-second JS check fades `AudioPlaylist.volume` over the last 10 s, pauses, and puts the volume back. "End of this track" pauses when the queue moves on by itself, or when the last track ends. A running timer shows as a badge ("23m") on the mini player. The timer is not saved: it is gone after the app restarts. Logic and tests are in `src/player/sleepTimer.ts`.

**Unverified:** that the timer still fires with the screen locked or the app in the background. The plan relies on JS timers (`setInterval`) continuing to run while audio plays in the background on iOS, with `UIBackgroundModes: ["audio"]`. This needs a real device; it was not tried. If it does not hold, the timer will fire late (when the app is next active) and no native workaround exists yet.

## Icons

Tab and header icons are Ionicons from `@expo/vector-icons`. The app, splash, favicon and Android adaptive icons in `assets/images` are generated from `assets/img/favicon.png` by `tools/make-icons.sh` (needs ImageMagick); rerun it after changing the favicon.

## Known limits and not done

- Lock-screen artwork cannot send the vault cookie, so it shows in normal mode only.
- Not tried on a device or with a live server from this checkout; the player was written against the installed `expo-audio` types and native sources.
- No offline downloads, no uploading from the phone, no share extension (use the Shortcut from plan 20), no standalone (EAS) build. See `REQUESTS.md`.
- API field names are set in `src/api/types.ts`; adjust them there if the server's shapes differ.
