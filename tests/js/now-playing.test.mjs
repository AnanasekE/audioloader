// Runs assets/js/now-playing.js: node --test tests/js
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
globalThis.window = globalThis;
new Function(readFileSync(root + "assets/js/now-playing.js", "utf8"))();

const { shouldClose } = window.nowPlaying;
const H = 800;

test("a drag closes the sheet past a quarter of its height", () => {
	assert.equal(shouldClose(199, 0, H), false);
	assert.equal(shouldClose(201, 0, H), true);
});

test("a fast flick closes it even after a short drag", () => {
	assert.equal(shouldClose(40, 0.6, H), true);
	assert.equal(shouldClose(40, 0.3, H), false);
});

test("dragging up or not at all never closes it", () => {
	assert.equal(shouldClose(0, 0, H), false);
	assert.equal(shouldClose(-300, 2, H), false);
});
