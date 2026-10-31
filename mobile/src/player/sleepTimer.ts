// Sleep timer logic, pure: no audio, no timers. Same rules as the web player
// (assets/js/sleep-timer.js): the countdown is wall-clock time, so pausing does
// not stop it; the volume fades over the last FADE_MS; "track" ends playback
// when the current track ends. Times are ms since the epoch, passed as `now`.

export type SleepTimer = { kind: "off" } | { kind: "at"; endsAt: number } | { kind: "track" };

export const FADE_MS = 10_000;
export const DEFAULT_MINUTES = 30;
export const MINUTES = [15, 30, 45, 60, 75, 90, 105, 120] as const;

export const off: SleepTimer = { kind: "off" };

export const isActive = (t: SleepTimer): boolean => t.kind !== "off";
export const start = (minutes: number, now: number): SleepTimer => ({ kind: "at", endsAt: now + minutes * 60_000 });
export const endOfTrack = (): SleepTimer => ({ kind: "track" });

// extend adds minutes to a running countdown, or starts one
export function extend(t: SleepTimer, minutes: number, now: number): SleepTimer {
  return t.kind === "at" ? { kind: "at", endsAt: t.endsAt + minutes * 60_000 } : start(minutes, now);
}

// remaining is the ms left, or null when off or waiting for the track to end
export const remaining = (t: SleepTimer, now: number): number | null => (t.kind === "at" ? Math.max(0, t.endsAt - now) : null);

// minutesLeft is what the badge shows: rounded up, so "1m" lasts until the end
export function minutesLeft(t: SleepTimer, now: number): number | null {
  const r = remaining(t, now);
  return r === null ? null : Math.ceil(r / 60_000);
}

export const shouldFade = (t: SleepTimer, now: number): boolean => t.kind === "at" && t.endsAt - now <= FADE_MS && t.endsAt - now > 0;
export const shouldStop = (t: SleepTimer, now: number): boolean => t.kind === "at" && t.endsAt <= now;

// fadeFactor goes from 1 to 0 over the last FADE_MS
export function fadeFactor(t: SleepTimer, now: number): number {
  return t.kind === "at" ? Math.min(1, Math.max(0, (t.endsAt - now) / FADE_MS)) : 1;
}

// badge is the short text for the mini player and the button: "23m", "End" or ""
export function badge(t: SleepTimer, now: number): string {
  if (t.kind === "track") return "End";
  const m = minutesLeft(t, now);
  return m === null ? "" : `${m}m`;
}
