# Plan 29: Sleep timer

**Goal:** a sleep timer that pauses playback after a chosen time, in the web player and the mobile app. It's built for podcasts but works for any track.

**Depends on:** plan 24 (player bar), plan 27 (volume, for the fade), plan 22 (mobile player).

## Status

- [x] 1. Timer logic (web)
- [x] 2. Timer UI (web)
- [x] 3. Mobile app

## Rules

As in plan 19 for the web; for `mobile/`, `npx tsc --noEmit` and `npx jest` pass. One commit per step, no attribution lines.

## Decisions

- **Options:** 15, 30, 45, 60, 75, 90, 105 and 120 minutes, plus "End of this episode" ("End of this track" for music) and "Off".
- **Where:**
  - In the web player bar, a moon button. On episodes it sits next to −15/+30/speed, always visible. On music it's in the same place but at `min-[720px]` only, so the phone bar doesn't get more crowded.
  - **Default 30 minutes:** with no timer running, one click starts a 30-minute timer right away and opens a small menu (like the queue popover) showing it running. From there you can pick another option or turn it off. With a timer running, the click only opens the menu.
  - While a timer runs, the button shows the minutes left ("23m") in the accent colour, and the menu has "Off" and "+15 min".
- **What happens at the end:** the volume fades to 0 over the last 10 seconds, then playback pauses, then the volume goes back to where it was. Pausing saves the episode position as any pause does. "End of this episode" pauses when the current track ends, instead of moving to the next one.
- **The countdown counts wall-clock time, not playback time.** A timer started for 30 minutes ends 30 minutes later even if you paused in between. That's the common podcast-app behaviour and the simplest to reason about. Pausing by hand doesn't cancel the timer; it stays until it fires or is turned off.
- **Saved:** the end time (an absolute timestamp, or "end of track") goes into the player's saved state, so a reload or an hx-boost page change keeps it. A timer that already ended while the page was closed is dropped on load.
- **Logic in a small pure module** `assets/js/sleep-timer.js` (`start(minutes, now)`, `remaining(now)`, `shouldFade(now)`, `shouldStop(now)`, `extend`, `cancel`). Node tests next to the existing `player-state` tests. `player.js` checks it once a second and on `ended`.
- **Mobile app:** same options, the same 30-minute default on the first tap, logic and fade, in `mobile/src/player/sleepTimer.ts` with jest tests, shown in the full-screen player and as a small "23m" badge on the mini player. On iOS, JS timers keep running in the background while audio plays. Check this on a device, and write the result in `mobile/README.md`. If it doesn't hold, note it there; don't build a native workaround in this plan.

## Steps

### 1. Timer logic (web)
- `sleep-timer.js` with tests, saved-state fields in `player-state.js` (with tests for restore and the drop of an expired timer).

### 2. Timer UI (web)
- Button, menu, countdown, fade and pause in `player.js`, a `moon` case in `icon()`.
- Headless Firefox screenshots of the player with an episode and a running timer at 1440×900 and 390×844, in the scratchpad.
- `FEATURES.md`, Playback/Podcasts: the sleep timer. `docs/plans/README.md`: row 29, update the status line.

### 3. Mobile app
- As decided. `FEATURES.md` mobile section and `mobile/README.md`.
