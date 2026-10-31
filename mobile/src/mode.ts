import type { Me } from "./api/types";

// Mode handling without React, so it can be tested. The app is in normal or
// secret mode (me.secret). When the mode changes, everything that may hold
// the other mode's data (query cache, player, queue) must be dropped, as the
// web does with a full page reload.

type Handler = () => void;

const handlers = new Set<Handler>();

// onModeReset registers a handler run on every reset (player stop, queue
// clear, cache clear). It returns an unregister function.
export function onModeReset(fn: Handler): () => void {
  handlers.add(fn);
  return () => handlers.delete(fn);
}

// resetForModeChange runs all handlers. A throwing handler must not stop the
// others: leaking secret state is worse than a failed cleanup.
export function resetForModeChange() {
  for (const fn of [...handlers]) {
    try {
      fn();
    } catch {}
  }
}

// ModeTracker remembers the last seen mode and reports changes.
export class ModeTracker {
  private last: boolean | null = null;

  // update takes a fresh /me answer and returns true when the mode changed
  // since the previous one (the very first answer is no change).
  update(me: Pick<Me, "secret">): boolean {
    const changed = this.last !== null && this.last !== me.secret;
    this.last = me.secret;
    return changed;
  }

  forget() {
    this.last = null;
  }
}
