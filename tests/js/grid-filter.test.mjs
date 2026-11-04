// Runs assets/js/grid-filter.js: node --test tests/js
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
globalThis.window = globalThis;
globalThis.document = { addEventListener() {} };
new Function(readFileSync(root + "assets/js/grid-filter.js", "utf8"))();
const { matches } = window.gridFilter;

test("an empty query matches everything", () => {
	assert.ok(matches("ok computer radiohead", ""));
	assert.ok(matches("ok computer radiohead", "   "));
});

test("every word must be contained, in any order and case", () => {
	assert.ok(matches("ok computer radiohead", "RADIO ok"));
	assert.ok(!matches("ok computer radiohead", "ok kid"));
});

test("words match inside other words", () => {
	assert.ok(matches("amnesiac", "nesia"));
});
