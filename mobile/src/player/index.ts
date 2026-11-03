import AsyncStorage from "@react-native-async-storage/async-storage";
import { createAudioPlaylist, setAudioModeAsync, type AudioPlaylist, type AudioPlaylistStatus, type AudioSource } from "expo-audio";
import { useSyncExternalStore } from "react";
import { absoluteUrl, authHeaders, post } from "@/api/client";
import type { Track } from "@/api/types";
import { onModeReset } from "@/mode";
import { bodyToSave, dueForSave, isEpisode, startPosition } from "./positions";
import * as Q from "./queue";
import * as S from "./sleepTimer";

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
  sleep: S.SleepTimer;
  repeat: Q.Repeat;
};

let state: PlayerState = { queue: Q.emptyQueue, playing: false, position: 0, duration: 0, buffering: false, sleep: S.off, repeat: "off" };
const listeners = new Set<() => void>();

function set(patch: Partial<PlayerState>) {
  const prev = state;
  state = { ...state, ...patch };
  listeners.forEach((l) => l());
  if (patch.repeat !== undefined || patch.sleep) applyLoop();
  if (patch.queue || patch.repeat !== undefined) schedulePersist();
  if (patch.queue) {
    const before = Q.current(prev.queue);
    const after = Q.current(state.queue);
    if (before?.id !== after?.id) {
      nextAt = 0;
      trackSwitched(before, prev, after);
    }
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
      // "end of this track": the queue moved on by itself, so stop here
      if (state.sleep.kind === "track") {
        pl.pause();
        endSleep();
      }
      set({ queue: Q.jumpTo(q, currentIndex), position: 0 });
      updateLockScreen();
    }
  });
  playlist = pl;
  applyLoop();
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
  // the last track of the queue ended
  if (s.didJustFinish && state.sleep.kind === "track") endSleep();
  if (mine) set({ playing: s.playing, position: s.currentTime, duration: s.duration, buffering: s.isBuffering });
  else set({ playing: s.playing });
  if (!mine || !t || !isEpisode(t)) return;
  if (s.didJustFinish) saveEpisode(t, s.currentTime, s.duration, true);
  else if ((wasPlaying && !s.playing) || (s.playing && dueForSave(lastSave, Date.now()))) saveEpisode(t, s.currentTime, s.duration);
}

// applyLoop gives the playlist the loop mode of the repeat mode (see queue.ts)
function applyLoop() {
  if (playlist) playlist.loop = Q.loopMode(state.repeat, state.sleep.kind === "track");
}

export const toggleRepeat = () => set({ repeat: Q.nextRepeat(state.repeat) });

// ---- sleep timer ----
// The logic is in sleepTimer.ts. This part runs a once-a-second check while a
// timer is on: it fades the volume over the last 10 s, pauses, and puts the
// volume back. The countdown is wall-clock time (pausing does not stop it).

let sleepTicker: ReturnType<typeof setInterval> | undefined;
let fadeBase: number | null = null; // the volume to return to while fading
let lastBadge = "";

function endFade() {
  if (fadeBase === null) return;
  if (playlist) playlist.volume = fadeBase;
  fadeBase = null;
}

function endSleep() {
  clearInterval(sleepTicker);
  sleepTicker = undefined;
  endFade();
  lastBadge = "";
  set({ sleep: S.off });
}

function sleepTick() {
  const now = Date.now();
  const t = state.sleep;
  if (S.shouldStop(t, now)) {
    playlist?.pause();
    return endSleep();
  }
  if (S.shouldFade(t, now) && state.playing && playlist) {
    fadeBase ??= playlist.volume;
    playlist.volume = fadeBase * S.fadeFactor(t, now);
  }
  // re-render the badge when its text changes
  const b = S.badge(t, now);
  if (b !== lastBadge) {
    lastBadge = b;
    set({});
  }
}

function setSleep(t: S.SleepTimer) {
  if (!S.isActive(t)) return endSleep();
  if (!S.shouldFade(t, Date.now())) endFade();
  lastBadge = S.badge(t, Date.now());
  set({ sleep: t });
  sleepTicker ??= setInterval(sleepTick, 1000);
}

export const startSleep = (minutes: number) => setSleep(S.start(minutes, Date.now()));
export const sleepAtEndOfTrack = () => setSleep(S.endOfTrack());
export const extendSleep = (minutes: number) => setSleep(S.extend(state.sleep, minutes, Date.now()));
export const cancelSleep = () => endSleep();

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

// how many tracks "Play next" has put after the current one since it started
let nextAt = 0;
const playable = (t: Track | Track[]): Track[] => (Array.isArray(t) ? t : [t]).filter((x) => x.streamUrl);

// playFrom starts tracks[index] with the rest of the list queued after it,
// as clicking a row does on the web.
export function playFrom(tracks: Track[], index: number): void {
  // a placeholder track has no file (streamUrl "") and cannot play
  const wanted = tracks[index];
  if (!wanted?.streamUrl) return;
  const playable = tracks.filter((t) => t.streamUrl);
  load(Q.setQueue(playable, playable.indexOf(wanted)), true);
}

export function playNow(track: Track): void {
  if (!track.streamUrl) return;
  load(Q.setQueue([track], 0), true);
}

// playNext and addToEnd take a track or a batch (a release, a playlist). The
// batch goes in as given; placeholders (no file) are skipped.
export function playNext(tracks: Track | Track[]): void {
  const add = playable(tracks);
  if (!add.length) return;
  if (!state.queue.items.length) return load(Q.setQueue(add, 0), true);
  const at = Q.playNextSlot(state.queue, nextAt);
  const pl = engine();
  add.forEach((t, k) => pl.insert(sourceOf(t), at + k));
  const q = Q.playNext(state.queue, add, nextAt);
  nextAt += add.length;
  set({ queue: q });
}

export function addToEnd(tracks: Track | Track[]): void {
  const add = playable(tracks);
  if (!add.length) return;
  if (!state.queue.items.length) return load(Q.setQueue(add, 0), true);
  add.forEach((t) => engine().add(sourceOf(t)));
  set({ queue: Q.addToEnd(state.queue, add) });
}

export function toggle(): void {
  if (!state.queue.items.length) return;
  const pl = engine();
  if (pl.playing) pl.pause();
  else pl.play();
}

export function next(): void {
  const i = Q.nextIndex(state.queue, state.repeat);
  if (i < 0) return;
  // a wrap goes back to the start, so it is a jump, not the playlist's next()
  const wrapped = i <= state.queue.index;
  set({ queue: Q.jumpTo(state.queue, i), position: 0 });
  if (wrapped) engine().skipTo(i);
  else engine().next();
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
  if (i > q.index && i <= q.index + nextAt) nextAt--;
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
  endSleep();
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
  state = { queue: Q.emptyQueue, playing: false, position: 0, duration: 0, buffering: false, sleep: S.off, repeat: state.repeat };
  pendingSeek = null;
  nextAt = 0;
  applyLoop();
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
    const op = q.items.length ? AsyncStorage.setItem(QUEUE_KEY, JSON.stringify({ secret: modeSecret, queue: q, repeat: state.repeat })) : AsyncStorage.removeItem(QUEUE_KEY);
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
    const snap = JSON.parse(raw) as { secret: boolean; queue: Q.QueueState; repeat?: unknown };
    if (snap.secret !== secret) {
      await AsyncStorage.removeItem(QUEUE_KEY);
      return;
    }
    if (snap.queue?.items?.length) {
      load(snap.queue, false);
      set({ repeat: Q.repeatFrom(snap.repeat) });
    }
  } catch {}
}
