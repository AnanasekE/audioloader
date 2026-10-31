import AsyncStorage from "@react-native-async-storage/async-storage";
import { createAudioPlaylist, setAudioModeAsync, type AudioPlaylist, type AudioPlaylistStatus, type AudioSource } from "expo-audio";
import { useSyncExternalStore } from "react";
import { absoluteUrl, authHeaders, post } from "@/api/client";
import type { Track } from "@/api/types";
import { onModeReset } from "@/mode";
import { bodyToSave, dueForSave, isEpisode, startPosition } from "./positions";
import * as Q from "./queue";

// The player. One expo-audio AudioPlaylist plays the queue: it is the only
// expo-audio object with next/previous, which the lock screen and Control
// Center need. QueueState (queue.ts) is the source of truth; every change to
// it is mirrored to the playlist with add/insert/remove/skipTo.

export type PlayerState = {
  queue: Q.QueueState;
  playing: boolean;
  position: number; // seconds into the current track
  duration: number;
  buffering: boolean;
};

let state: PlayerState = { queue: Q.emptyQueue, playing: false, position: 0, duration: 0, buffering: false };
const listeners = new Set<() => void>();

function set(patch: Partial<PlayerState>) {
  const prev = state;
  state = { ...state, ...patch };
  listeners.forEach((l) => l());
  if (patch.queue) {
    schedulePersist();
    const before = Q.current(prev.queue);
    const after = Q.current(state.queue);
    if (before?.id !== after?.id) trackSwitched(before, prev, after);
  }
}

const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => listeners.delete(l);
};

export const getPlayerState = () => state;
export const usePlayer = (): PlayerState => useSyncExternalStore(subscribe, getPlayerState);

// ---- engine ----

let playlist: AudioPlaylist | null = null;
// while we rewrite the playlist, track-changed events are noise
let mutating = 0;

function engine(): AudioPlaylist {
  if (playlist) return playlist;
  // play in the background and with the silent switch on; lock-screen
  // controls need doNotMix
  setAudioModeAsync({ playsInSilentMode: true, shouldPlayInBackground: true, interruptionMode: "doNotMix" }).catch(() => {});
  const pl = createAudioPlaylist({ updateInterval: 500 });
  pl.addListener("playlistStatusUpdate", onStatus);
  pl.addListener("trackChanged", ({ currentIndex }) => {
    if (mutating) return;
    const q = state.queue;
    if (currentIndex >= 0 && currentIndex < q.items.length && currentIndex !== q.index) {
      set({ queue: Q.jumpTo(q, currentIndex), position: 0 });
      updateLockScreen();
    }
  });
  playlist = pl;
  return pl;
}

function onStatus(s: AudioPlaylistStatus) {
  const wasPlaying = state.playing;
  const t = Q.current(state.queue);
  // ignore updates that still describe another track than the current one
  const mine = s.currentIndex === state.queue.index;
  if (mine && pendingSeek && t?.id === pendingSeek.id && s.isLoaded) {
    const to = pendingSeek.seconds;
    pendingSeek = null;
    void playlist?.seekTo(to);
  }
  if (mine) set({ playing: s.playing, position: s.currentTime, duration: s.duration, buffering: s.isBuffering });
  else set({ playing: s.playing });
  if (!mine || !t || !isEpisode(t)) return;
  if (s.didJustFinish) saveEpisode(t, s.currentTime, s.duration, true);
  else if ((wasPlaying && !s.playing) || (s.playing && dueForSave(lastSave, Date.now()))) saveEpisode(t, s.currentTime, s.duration);
}

// ---- episode positions ----

let pendingSeek: { id: string; seconds: number } | null = null;
let lastSave = 0;
// positions saved in this session, newer than the cached track lists
const saved = new Map<string, number>();

function saveEpisode(t: Track, position: number, duration: number, ended = false) {
  lastSave = Date.now();
  const body = bodyToSave(position, duration, ended);
  saved.set(t.id, body.seconds);
  post(`/api/tracks/${t.id}/position`, body).catch(() => {});
}

// trackSwitched runs when the current track changes: it saves where the
// previous episode stopped and queues a seek to where the new one resumes.
function trackSwitched(before: Track | null, prev: PlayerState, after: Track | null) {
  if (before && isEpisode(before) && prev.duration > 0) saveEpisode(before, prev.position, prev.duration);
  pendingSeek = null;
  if (after && isEpisode(after)) {
    const at = startPosition(after, saved.get(after.id));
    if (at > 0) pendingSeek = { id: after.id, seconds: at };
  }
}

function sourceOf(t: Track): AudioSource {
  return { uri: absoluteUrl(t.streamUrl), headers: authHeaders(), name: t.title };
}

function lockScreenMetadata(t: Track) {
  return {
    title: t.title,
    artist: t.artists.map((a) => a.name).join(", "),
    // the lock screen cannot send a cookie, so this works in normal mode only
    artworkUrl: t.imageUrl ? absoluteUrl(t.imageUrl) : undefined,
  };
}

function updateLockScreen() {
  const t = Q.current(state.queue);
  if (!t || !playlist) return;
  playlist.setActiveForLockScreen(true, lockScreenMetadata(t), {
    showNextTrack: true,
    showPreviousTrack: true,
    showSeekForward: true,
    showSeekBackward: true,
    seekForwardIntervalSeconds: 30,
    seekBackwardIntervalSeconds: 15,
  });
}

// load rewrites the playlist to match q and optionally starts playing.
function load(q: Q.QueueState, autoplay: boolean) {
  const pl = engine();
  mutating++;
  try {
    pl.pause();
    pl.clear();
    q.items.forEach((t) => pl.add(sourceOf(t)));
    if (q.items.length) pl.skipTo(q.index);
  } finally {
    mutating--;
  }
  set({ queue: q, position: 0, duration: q.items[q.index]?.duration ?? 0 });
  updateLockScreen();
  if (autoplay && q.items.length) pl.play();
}

// ---- actions ----

// playFrom starts tracks[index] with the rest of the list queued after it,
// as clicking a row does on the web.
export function playFrom(tracks: Track[], index: number): void {
  load(Q.setQueue(tracks, index), true);
}

export function playNow(track: Track): void {
  load(Q.setQueue([track], 0), true);
}

export function playNext(track: Track): void {
  if (!state.queue.items.length) return playNow(track);
  const q = Q.playNext(state.queue, track);
  engine().insert(sourceOf(track), state.queue.index + 1);
  set({ queue: q });
}

export function addToEnd(track: Track): void {
  if (!state.queue.items.length) return playNow(track);
  engine().add(sourceOf(track));
  set({ queue: Q.addToEnd(state.queue, track) });
}

export function toggle(): void {
  if (!state.queue.items.length) return;
  const pl = engine();
  if (pl.playing) pl.pause();
  else pl.play();
}

export function next(): void {
  if (!Q.hasNext(state.queue)) return;
  set({ queue: Q.next(state.queue), position: 0 });
  engine().next();
  updateLockScreen();
}

// previous restarts the track when it has played for more than 3 s
export function previous(): void {
  if (state.position > 3 || !Q.hasPrevious(state.queue)) {
    void seekTo(0);
    return;
  }
  set({ queue: Q.previous(state.queue), position: 0 });
  engine().previous();
  updateLockScreen();
}

export function jumpTo(i: number): void {
  if (i === state.queue.index || i < 0 || i >= state.queue.items.length) return;
  set({ queue: Q.jumpTo(state.queue, i), position: 0 });
  engine().skipTo(i);
  engine().play();
  updateLockScreen();
}

export async function seekTo(seconds: number): Promise<void> {
  if (!state.queue.items.length) return;
  const to = Math.max(0, state.duration ? Math.min(seconds, state.duration) : seconds);
  set({ position: to });
  await engine().seekTo(to);
}

export const skipBy = (delta: number) => seekTo(state.position + delta);

// removeAt drops a queued track. Removing the playing one skips to the next
// first, or stops when it was the last.
export function removeAt(i: number): void {
  const q = state.queue;
  if (i < 0 || i >= q.items.length) return;
  if (i === q.index) {
    if (!Q.hasNext(q)) return reset();
    next();
  }
  engine().remove(i);
  set({ queue: Q.removeAt(state.queue, i) });
}

// move reorders queued tracks. The playing track stays where it is.
export function move(from: number, to: number): void {
  const q = state.queue;
  if (from === q.index || to === q.index) return;
  const moved = Q.move(q, from, to);
  if (moved === q) return;
  const pl = engine();
  mutating++;
  try {
    pl.remove(from);
    pl.insert(sourceOf(q.items[from]), to);
  } finally {
    mutating--;
  }
  set({ queue: moved });
}

// reset stops playback and empties the queue, also on disk. It runs on every
// mode change so nothing of the other mode keeps playing or shows up.
export function reset(): void {
  if (playlist) {
    mutating++;
    try {
      playlist.pause();
      playlist.clear();
      playlist.clearLockScreenControls();
    } catch {
    } finally {
      mutating--;
    }
  }
  state = { queue: Q.emptyQueue, playing: false, position: 0, duration: 0, buffering: false };
  pendingSeek = null;
  saved.clear();
  listeners.forEach((l) => l());
  void AsyncStorage.removeItem(QUEUE_KEY).catch(() => {});
}

onModeReset(reset);

// ---- saved queue ----

const QUEUE_KEY = "queue";
let modeSecret: boolean | null = null;
let persistTimer: ReturnType<typeof setTimeout> | undefined;

function schedulePersist() {
  if (modeSecret === null) return;
  clearTimeout(persistTimer);
  persistTimer = setTimeout(() => {
    const q = state.queue;
    const op = q.items.length ? AsyncStorage.setItem(QUEUE_KEY, JSON.stringify({ secret: modeSecret, queue: q })) : AsyncStorage.removeItem(QUEUE_KEY);
    op.catch(() => {});
  }, 500);
}

// bindMode tells the player which mode the app is in. The first call also
// restores the saved queue, paused, when it was saved in the same mode.
export async function bindMode(secret: boolean): Promise<void> {
  const first = modeSecret === null;
  modeSecret = secret;
  if (!first) return;
  try {
    const raw = await AsyncStorage.getItem(QUEUE_KEY);
    if (!raw || state.queue.items.length) return;
    const snap = JSON.parse(raw) as { secret: boolean; queue: Q.QueueState };
    if (snap.secret !== secret) {
      await AsyncStorage.removeItem(QUEUE_KEY);
      return;
    }
    if (snap.queue?.items?.length) load(snap.queue, false);
  } catch {}
}
