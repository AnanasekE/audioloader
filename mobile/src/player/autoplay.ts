import type { Track } from "@/api/types";
import type { QueueTrack } from "./queue";

// Autoplay, as pure functions (same rules as the web player): when the queue
// runs out, GET /api/v1/autoplay suggests tracks like the last few played,
// never one of the recently played ones.

export const HISTORY_MAX = 50;
export const SEEDS = 5;
export const BATCH = 10;

// pushHistory puts id last (most recent); an earlier play of it moves up.
export const pushHistory = (history: string[], id: string): string[] => [...history.filter((h) => h !== id), id].slice(-HISTORY_MAX);

// historyFrom reads a saved history: a list of strings, else empty.
export function historyFrom(v: unknown): string[] {
  return Array.isArray(v) && v.every((x) => typeof x === "string") ? (v as string[]).slice(-HISTORY_MAX) : [];
}

// autoplayPath is the request: the last SEEDS played tracks as seeds, newest
// first, and the whole history excluded.
export function autoplayPath(history: string[], n = BATCH): string {
  const q = [...history.slice(-SEEDS).reverse().map((id) => `seed=${encodeURIComponent(id)}`), ...history.map((id) => `exclude=${encodeURIComponent(id)}`), `n=${n}`];
  return `/api/v1/autoplay?${q.join("&")}`;
}

// asAutoplay keeps the playable suggestions that aren't queued already, marked as autoplay.
export function asAutoplay(suggested: Track[], queued: Track[]): QueueTrack[] {
  const have = new Set(queued.map((t) => t.id));
  return suggested.filter((t) => t.streamUrl && !have.has(t.id)).map((t) => ({ ...t, auto: true }));
}
