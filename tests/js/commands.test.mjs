// Runs the pure part of assets/js/commands.js: node --test tests/js
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
globalThis.window = globalThis;
new Function(readFileSync(root + "assets/js/commands.js", "utf8"))();

const { parse, match, list } = window.commands;
const cmds = [
	{ id: "shuffle-on", label: "Shuffle on", keywords: "random" },
	{ id: "shuffle-off", label: "Shuffle off", keywords: "random", when: () => false },
	{ id: "sleep", label: "Sleep in 30 minutes", keywords: "timer stop" },
	{ id: "dark", label: "Dark theme", keywords: "appearance" },
];
const ids = (found) => found.map((c) => c.id);

test("parse: a leading > means commands only", () => {
	assert.deepEqual(parse("sh"), { only: false, text: "sh" });
	assert.deepEqual(parse(">sh"), { only: true, text: "sh" });
	assert.deepEqual(parse("  > sh "), { only: true, text: "sh" });
	assert.deepEqual(parse(">"), { only: true, text: "" });
	assert.deepEqual(parse("a > b"), { only: false, text: "a > b" });
});

test("match: every typed word must begin a word of the label or keywords", () => {
	assert.deepEqual(ids(match(cmds, "sh")), ["shuffle-on"]);
	assert.deepEqual(ids(match(cmds, "SLEEP 30")), ["sleep"]);
	assert.deepEqual(ids(match(cmds, "timer")), ["sleep"]);
	assert.deepEqual(ids(match(cmds, "rand")), ["shuffle-on"]);
	assert.deepEqual(ids(match(cmds, "huffle")), [], "not in the middle of a word");
	assert.deepEqual(ids(match(cmds, "sleep dark")), []);
});

test("match: commands that don't apply are left out", () => {
	assert.deepEqual(ids(match(cmds, "shuffle off")), []);
});

test("match: no text matches nothing, except for the > list of all", () => {
	assert.deepEqual(match(cmds, ""), []);
	assert.deepEqual(match(cmds, "  "), []);
	assert.deepEqual(ids(match(cmds, "", true)), ["shuffle-on", "sleep", "dark"]);
});

test("the command list has unique ids and the fields nav.js reads", () => {
	assert.equal(new Set(list.map((c) => c.id)).size, list.length);
	for (const c of list) {
		assert.equal(typeof c.label, "string", c.id);
		assert.equal(typeof c.run, "function", c.id);
	}
});
