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

test("nothing left, nothing resumes", () => {
	assert.equal(window.playerState.forMode(saved("normal", [t("s", true)]), "normal"), null);
	assert.equal(window.playerState.forMode(null, "normal"), null);
});
