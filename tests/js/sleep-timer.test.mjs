// Runs assets/js/sleep-timer.js: node --test tests/js
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
globalThis.window = globalThis;
new Function(readFileSync(root + "assets/js/sleep-timer.js", "utf8"))();

const MIN = 60000;
const T0 = 1_000_000;

test("off by default", () => {
	const t = window.sleepTimer.create(null);
	assert.equal(t.active(), false);
	assert.equal(t.remaining(T0), null);
	assert.equal(t.shouldStop(T0), false);
	assert.equal(t.save(), null);
});

test("start counts down in wall-clock time", () => {
	const t = window.sleepTimer.create(null);
	t.start(30, T0);
	assert.equal(t.active(), true);
	assert.equal(t.remaining(T0), 30 * MIN);
	assert.equal(t.remaining(T0 + 29 * MIN), MIN);
	assert.equal(t.minutesLeft(T0 + 29 * MIN + 1), 1);
	assert.equal(t.minutesLeft(T0), 30);
	assert.equal(t.shouldStop(T0 + 30 * MIN - 1), false);
	assert.equal(t.shouldStop(T0 + 30 * MIN), true);
	assert.equal(t.remaining(T0 + 31 * MIN), 0);
});

test("fade covers the last ten seconds", () => {
	const t = window.sleepTimer.create(null);
	t.start(1, T0);
	assert.equal(t.shouldFade(T0 + 49_000), false);
	assert.equal(t.fadeFactor(T0 + 49_000), 1);
	assert.equal(t.shouldFade(T0 + 50_000), true);
	assert.equal(t.fadeFactor(T0 + 55_000), 0.5);
	assert.equal(t.shouldFade(T0 + 60_000), false); // over: stop, not fade
	assert.equal(t.fadeFactor(T0 + 60_000), 0);
	assert.equal(t.shouldStop(T0 + 60_000), true);
});

test("extend adds time, or starts a timer", () => {
	const t = window.sleepTimer.create(null);
	t.start(10, T0);
	t.extend(15, T0 + MIN);
	assert.equal(t.remaining(T0 + MIN), 24 * MIN);
	const u = window.sleepTimer.create(null);
	u.extend(15, T0);
	assert.equal(u.remaining(T0), 15 * MIN);
});

test("cancel turns it off", () => {
	const t = window.sleepTimer.create(null);
	t.start(10, T0);
	t.cancel();
	assert.equal(t.active(), false);
	assert.equal(t.shouldStop(T0 + 99 * MIN), false);
	assert.equal(t.save(), null);
});

test("end of track has no countdown and survives save and restore", () => {
	const t = window.sleepTimer.create(null);
	t.startEndOfTrack();
	assert.equal(t.active(), true);
	assert.equal(t.endOfTrack(), true);
	assert.equal(t.remaining(T0), null);
	assert.equal(t.shouldStop(T0 + 99 * MIN), false);
	assert.deepEqual(t.save(), { endOfTrack: true });
	assert.equal(window.sleepTimer.create(t.save()).endOfTrack(), true);
	// picking minutes replaces it
	t.start(15, T0);
	assert.equal(t.endOfTrack(), false);
});

test("a saved countdown restores", () => {
	const t = window.sleepTimer.create({ endsAt: T0 + 5 * MIN });
	assert.equal(t.remaining(T0), 5 * MIN);
	assert.deepEqual(t.save(), { endsAt: T0 + 5 * MIN });
});
