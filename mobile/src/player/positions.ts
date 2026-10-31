import type { PositionBody, Track } from "@/api/types";

// Episode positions, with the rules of assets/js/player.js: within 30 s of
// the end an episode counts as finished and its saved position resets to 0.
export const FINISH_WITHIN = 30; // seconds from the end
export const SAVE_EVERY = 15_000; // ms between saves while playing

export function isEpisode(t: Pick<Track, "kind">): boolean {
  return t.kind === "episode";
}

// bodyToSave is what POST /api/tracks/{id}/position gets.
export function bodyToSave(position: number, duration: number, ended = false): PositionBody {
  const finished = ended || (duration > 0 && duration - position < FINISH_WITHIN);
  return { seconds: finished ? 0 : Math.max(0, position), finished };
}

// startPosition is where an episode begins: at the saved position, unless it
// was finished or never started. override is a position saved in this session
// that the cached track may not have yet.
export function startPosition(t: Pick<Track, "position" | "finished">, override?: number): number {
  if (override !== undefined) return override;
  if (!t.position || t.finished) return 0;
  return t.position;
}

// dueForSave says whether 15 s have passed since the last save.
export function dueForSave(lastSave: number, now: number): boolean {
  return now - lastSave >= SAVE_EVERY;
}
