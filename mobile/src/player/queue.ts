import type { Track } from "@/api/types";

// The play queue as plain data and pure functions, so it can be tested
// without audio. index is the position of the current track, or -1 when
// the queue is empty.
export type QueueState = { items: Track[]; index: number };

export const emptyQueue: QueueState = { items: [], index: -1 };

export function current(q: QueueState): Track | null {
  return q.items[q.index] ?? null;
}

// setQueue replaces the queue. Tracks are kept once each, in order, and index
// points at the same track in the de-duplicated list.
export function setQueue(tracks: Track[], index: number): QueueState {
  const wanted = tracks[index];
  const seen = new Set<string>();
  const items = tracks.filter((t) => !seen.has(t.id) && seen.add(t.id));
  if (!items.length || !wanted) return emptyQueue;
  return { items, index: items.findIndex((t) => t.id === wanted.id) };
}

// playNext puts a track right after the current one.
export function playNext(q: QueueState, track: Track): QueueState {
  if (!q.items.length) return { items: [track], index: 0 };
  const items = q.items.slice();
  items.splice(q.index + 1, 0, track);
  return { items, index: q.index };
}

export function addToEnd(q: QueueState, track: Track): QueueState {
  if (!q.items.length) return { items: [track], index: 0 };
  return { items: [...q.items, track], index: q.index };
}

// removeAt drops the track at i. Removing the current track leaves index on
// the track that followed it (or the last one).
export function removeAt(q: QueueState, i: number): QueueState {
  if (i < 0 || i >= q.items.length) return q;
  const items = q.items.filter((_, j) => j !== i);
  if (!items.length) return emptyQueue;
  let index = q.index;
  if (i < index) index--;
  else if (i === index) index = Math.min(index, items.length - 1);
  return { items, index };
}

// move takes the track at from and puts it at to, keeping the current track
// current.
export function move(q: QueueState, from: number, to: number): QueueState {
  const n = q.items.length;
  if (from < 0 || from >= n || to < 0 || to >= n || from === to) return q;
  const items = q.items.slice();
  const [t] = items.splice(from, 1);
  items.splice(to, 0, t);
  let index = q.index;
  if (index === from) index = to;
  else if (from < index && to >= index) index--;
  else if (from > index && to <= index) index++;
  return { items, index };
}

export function hasNext(q: QueueState): boolean {
  return q.index >= 0 && q.index + 1 < q.items.length;
}

export function hasPrevious(q: QueueState): boolean {
  return q.index > 0;
}

export function jumpTo(q: QueueState, i: number): QueueState {
  return i < 0 || i >= q.items.length ? q : { items: q.items, index: i };
}

// next and previous return the same state at the ends.
export function next(q: QueueState): QueueState {
  return hasNext(q) ? jumpTo(q, q.index + 1) : q;
}

export function previous(q: QueueState): QueueState {
  return hasPrevious(q) ? jumpTo(q, q.index - 1) : q;
}
