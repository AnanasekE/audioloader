# Plan 31: Repeat (loop)

**Goal:** repeat the queue or the current track, in the web player and the mobile app.

**Depends on:** plans 24 and 29 (player bar, saved state), plan 22 (mobile player).

## Status

- [ ] 1. Repeat logic and saved state (web)
- [ ] 2. Repeat button (web)
- [ ] 3. Mobile app

## Rules

As in plan 29; for `mobile/`, `npx tsc --noEmit` and `npx jest` pass. One commit per step, no attribution lines.

## Decisions

- **Three modes, one button:** off, all, one; each click moves to the next. It sits next to shuffle, with the same pressed styling. Icons `repeat` and `repeat-1` in `icon()`, same stroke style. `aria-label` "Repeat: off" / "Repeat: all" / "Repeat: one".
- **Repeat one:** on `ended`, the track starts again from 0. Next and Prev still move as usual (as in most players), and the mode stays.
- **Repeat all:** `next()` at the end of the order goes back to position 0. With shuffle on, the order is reshuffled for the new pass, keeping the track that just played out of the first slot.
- **Sleep timer wins:** "End of this track" stops even with repeat one.
- **Episodes:** repeat applies to them too; position saving stays as it is (an episode that ends is marked finished, then restarts).
- **Saved:** `repeat` ("off" | "all" | "one") in the saved state; `player-state.js` gets `repeatFrom(state)` returning a valid mode or "off", with node tests.
- **Logic** stays small and pure: a `nextPos(pos, length, repeat)` helper next to it, tested, that returns the next position or -1.

## Steps

### 1. Repeat logic and saved state (web)
- `repeatFrom` and `nextPos` with tests; `player.js` uses them in `next()` and on `ended`.

### 2. Repeat button (web)
- Button and icons in `layout.templ` (desktop and phone spots, kept in step as the sleep button is).
- Headless Firefox screenshots at 1440×900 and 390×844 in the scratchpad.
- `FEATURES.md`, Playback: repeat. `REQUESTS.md`: remove the loop entry.

### 3. Mobile app
- Same modes in `mobile/src/player/queue.ts` (with jest tests), a button in the full-screen player; with `react-native-track-player` use its `RepeatMode` if that is what the player uses, otherwise the same logic. `FEATURES.md` mobile section.
