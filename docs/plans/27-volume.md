# Plan 27: Volume control

**Goal:** the web player gets a volume slider and a mute button. `player.js` already saves and restores `audio.volume`, but nothing on the page changes it.

**Depends on:** plan 24 (player bar and queue panel).

## Status

- [x] 1. Volume slider and mute

## Rules

As in plan 19: `make build`, `go vet ./...` and `make test` pass; no attribution lines; plain JS, Tailwind classes as in `layout.templ`.

## Decisions

- **Where:** in the player bar, right side, after the seek row and before ☰: a speaker button (mute toggle) and a range input (0–1, step 0.01, about 96px wide). Shown from `min-[720px]` up only. On phones the hardware buttons set the volume, and iOS Safari ignores `audio.volume` anyway.
- **Mute** uses `audio.muted`, so unmuting goes back to the previous level. Dragging the slider above 0 unmutes. The icon has three states: muted or 0, low (< 0.5), high, as inline SVG in `icon()` (`volume-x`, `volume-1`, `volume-2`, same stroke style). `aria-label` "Mute" / "Unmute", and the slider `aria-label` "Volume".
- **Saved:** add `muted` to the saved state next to `volume`, and restore both. The slider follows `volumechange`, so it stays right when the volume changes elsewhere.
- **Keyboard:** with focus not in a text field, only `m` (toggle mute) is global. Arrow keys are left to the page so it scrolls; the volume slider keeps its native arrow handling when focused.
- **Exponential feel:** the slider sets `audio.volume = value²`, so the low end isn't all crammed together, and shows `sqrt(volume)`. Keep this in one small pair of functions.

## Steps

### 1. Volume slider and mute
- Markup, `icon()` cases, `player.js` wiring, saved state.
- If `player-state.js` has node tests, extend them for `muted` in the saved state.
- Headless Firefox screenshot of the player bar at 1440×900 and 390×844 (slider hidden there), in the scratchpad.
- `FEATURES.md`, Playback: "Volume slider and mute (desktop), remembered across reloads". `docs/plans/README.md`: row 27, update the status line.
