// Runs the pure part of assets/js/context-menu.js: node --test tests/js
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
globalThis.window = globalThis;
new Function(readFileSync(root + "assets/js/context-menu.js", "utf8"))();

const { placeMenu } = window.contextMenu;

test("room on both sides: opens at the point", () => {
	assert.deepEqual(placeMenu(100, 200, 192, 180, 1440, 900), { left: 100, top: 200 });
});

test("flips left and up near the right and bottom edges", () => {
	assert.deepEqual(placeMenu(1400, 800, 192, 180, 1440, 900), { left: 1208, top: 620 });
});

test("stays inside when it fits on neither side", () => {
	assert.deepEqual(placeMenu(100, 150, 192, 400, 250, 300), { left: 8, top: 8 });
	assert.deepEqual(placeMenu(0, 0, 192, 180, 1440, 900), { left: 8, top: 8 });
});
