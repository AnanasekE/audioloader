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

test("nothing left, nothing resumes", () => {
	assert.equal(window.playerState.forMode(saved("normal", [t("s", true)]), "normal"), null);
	assert.equal(window.playerState.forMode(null, "normal"), null);
});
