// Runs assets/js/player-state.js: node --test tests/js
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
globalThis.window = globalThis;
new Function(readFileSync(root + "assets/js/player-state.js", "utf8"))();

const t = (id, secret = false) => ({ id, title: id, secret });
const saved = (mode, tracks, pos = 0) => ({
	mode,
	track: tracks[pos],
	time: 42,
	paused: false,
	queue: tracks,
	order: tracks.map((_, i) => i),
	pos,
});

test("state saved in the other mode is dropped", () => {
	assert.equal(window.playerState.forMode(saved("secret", [t("a", true)]), "normal"), null);
	assert.equal(window.playerState.forMode(saved("normal", [t("a")]), "secret"), null);
});

test("the same mode resumes untouched", () => {
	const s = window.playerState.forMode(saved("secret", [t("a", true), t("b", true)], 1), "secret");
	assert.equal(s.track.id, "b");
	assert.equal(s.time, 42);
	assert.deepEqual(s.queue.map((x) => x.id), ["a", "b"]);
});

test("tracks of the other side are dropped from the queue", () => {
	// no mode: saved before modes existed, with secret tracks in it
	const s = window.playerState.forMode({ ...saved(undefined, [t("a"), t("s", true), t("b")], 2), mode: undefined }, "normal");
	assert.deepEqual(s.queue.map((x) => x.id), ["a", "b"]);
	assert.equal(s.track.id, "b");
	assert.equal(s.pos, 1);
	assert.equal(s.mode, "normal");
});

test("when the playing track goes, the next kept one takes over, paused", () => {
	const s = window.playerState.forMode(saved("normal", [t("s", true), t("b")], 0), "normal");
	assert.equal(s.track.id, "b");
	assert.equal(s.paused, true);
	assert.equal(s.time, 0);
});

test("volume slider is squared and maps back", () => {
	const { volumeFromSlider, sliderFromVolume } = window.playerState;
	assert.equal(volumeFromSlider(0.5), 0.25);
	assert.equal(volumeFromSlider(2), 1);
	assert.equal(volumeFromSlider(-1), 0);
	assert.ok(Math.abs(sliderFromVolume(volumeFromSlider(0.3)) - 0.3) < 1e-9);
});

test("saved volume and muted are restored, bad values are ignored", () => {
	const a = window.playerState.audioSettings;
	assert.deepEqual(a({ volume: 0.4, muted: true }), { volume: 0.4, muted: true });
	assert.deepEqual(a({ volume: 3, muted: "yes" }), { volume: null, muted: null });
	assert.deepEqual(a({}), { volume: null, muted: null });
	assert.deepEqual(a(null), { volume: null, muted: null });
});

test("a saved sleep timer is restored, an expired or bad one dropped", () => {
	const f = window.playerState.sleepFrom;
	assert.deepEqual(f({ sleep: { endsAt: 2000 } }, 1000), { endsAt: 2000 });
	assert.equal(f({ sleep: { endsAt: 1000 } }, 1000), null);
	assert.equal(f({ sleep: { endsAt: 500 } }, 1000), null);
	assert.deepEqual(f({ sleep: { endOfTrack: true } }, 1000), { endOfTrack: true });
	assert.equal(f({ sleep: { endsAt: "soon" } }, 1000), null);
	assert.equal(f({ sleep: "x" }, 1000), null);
	assert.equal(f({}, 1000), null);
	assert.equal(f(null, 1000), null);
});

test("the repeat mode is restored, anything else is off", () => {
	const f = window.playerState.repeatFrom;
	assert.equal(f({ repeat: "all" }), "all");
	assert.equal(f({ repeat: "one" }), "one");
	assert.equal(f({ repeat: "loop" }), "off");
	assert.equal(f({}), "off");
	assert.equal(f(null), "off");
});

test("nextPos moves on, and only repeat all wraps", () => {
	const n = window.playerState.nextPos;
	assert.equal(n(0, 3, "off"), 1);
	assert.equal(n(2, 3, "off"), -1);
	assert.equal(n(2, 3, "one"), -1);
	assert.equal(n(2, 3, "all"), 0);
	assert.equal(n(0, 1, "all"), 0);
	assert.equal(n(0, 0, "all"), -1);
	assert.equal(n(0, 3, "one"), 1);
});

test("keepOutOfFirst moves the track that just played off the first slot", () => {
	const k = window.playerState.keepOutOfFirst;
	assert.deepEqual(k([2, 0, 1], 2), [0, 2, 1]);
	assert.deepEqual(k([0, 2, 1], 2), [0, 2, 1]);
	assert.deepEqual(k([0], 0), [0]);
});

test("playNextSlot keeps Play next in the order added", () => {
	const s = window.playerState.playNextSlot;
	assert.equal(s(2, 0, 10), 3);
	assert.equal(s(2, 1, 10), 4);
	assert.equal(s(2, 3, 10), 6);
	// the queue got shorter since: never past the end
	assert.equal(s(2, 5, 5), 5);
	assert.equal(s(0, 0, 1), 1);
	assert.equal(s(0, -2, 4), 1);
});

test("autoplay is on unless saved as off; the history is a list of IDs", () => {
	const p = window.playerState;
	assert.equal(p.autoplayFrom({}), true);
	assert.equal(p.autoplayFrom(null), true);
	assert.equal(p.autoplayFrom({ autoplay: false }), false);
	assert.equal(p.autoplayFrom({ autoplay: true }), true);
	assert.deepEqual(p.historyFrom({ history: ["a", "b"] }), ["a", "b"]);
	assert.deepEqual(p.historyFrom({ history: ["a", 3] }), []);
	assert.deepEqual(p.historyFrom({ history: "a" }), []);
	assert.deepEqual(p.historyFrom({}), []);
	const long = Array.from({ length: 80 }, (_, i) => "t" + i);
	assert.equal(p.historyFrom({ history: long }).length, 50);
	assert.equal(p.historyFrom({ history: long })[49], "t79");
});

test("pushHistory keeps the last 50, newest at the end, no repeats", () => {
	const p = window.playerState;
	assert.deepEqual(p.pushHistory(["a", "b"], "c"), ["a", "b", "c"]);
	assert.deepEqual(p.pushHistory(["a", "b", "c"], "a"), ["b", "c", "a"]);
	let h = [];
	for (let i = 0; i < 60; i++) h = p.pushHistory(h, "t" + i);
	assert.equal(h.length, 50);
	assert.equal(h[0], "t10");
	assert.equal(h[49], "t59");
});

test("autoplayURL seeds with the last 5 newest first and excludes the history", () => {
	const url = new URL(window.playerState.autoplayURL(["a", "b", "c", "d", "e", "f", "g"], 10), "http://x");
	assert.equal(url.pathname, "/api/v1/autoplay");
	assert.deepEqual(url.searchParams.getAll("seed"), ["g", "f", "e", "d", "c"]);
	assert.deepEqual(url.searchParams.getAll("exclude"), ["a", "b", "c", "d", "e", "f", "g"]);
	assert.equal(url.searchParams.get("n"), "10");
	assert.deepEqual(new URL(window.playerState.autoplayURL([], 10), "http://x").searchParams.getAll("seed"), []);
});

test("trackFromAPI builds a queue entry marked as autoplay", () => {
	const j = { id: "x", title: "T", artists: [{ id: "1", name: "A" }, { id: "2", name: "B" }], imageUrl: "/covers/x", streamUrl: "/api/tracks/x/stream" };
	assert.deepEqual(window.playerState.trackFromAPI(j, false), { id: "x", title: "T", artists: "A, B", img: "/covers/x", src: "/api/tracks/x/stream", auto: true });
	const s = window.playerState.trackFromAPI({ ...j, imageUrl: "", artists: [] }, true);
	assert.equal(s.secret, true);
	assert.equal(s.img, "/assets/img/default_secret.png");
	assert.equal(s.artists, "");
	assert.equal(window.playerState.trackFromAPI({ ...j, imageUrl: "" }, false).img, "/assets/img/default_track.png");
});

test("autoStart and autoLast keep what the user adds before the autoplay tracks", () => {
	const p = window.playerState;
	const queue = [{ id: "a" }, { id: "b" }, { id: "x", auto: true }, { id: "y", auto: true }, { id: "c" }];
	// playing a (position 0): b, then autoplay x y, then c added later
	const order = [0, 1, 2, 3, 4];
	assert.equal(p.autoStart(order, queue, 0), 2);
	assert.equal(p.autoStart([0, 1, 4], queue, 0), 3);
	assert.equal(p.autoStart(order, queue, 2), 3);
	assert.deepEqual(p.autoLast(order, queue, 0), [0, 1, 4, 2, 3]);
	// what has played already stays where it is, even an autoplay track
	assert.deepEqual(p.autoLast([2, 0, 3, 1], queue, 1), [2, 0, 1, 3]);
});

test("nothing left, nothing resumes", () => {
	assert.equal(window.playerState.forMode(saved("normal", [t("s", true)]), "normal"), null);
	assert.equal(window.playerState.forMode(null, "normal"), null);
});
