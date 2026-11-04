// Runs assets/js/cover-tint.js: node --test tests/js
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
globalThis.window = globalThis;
new Function(readFileSync(root + "assets/js/cover-tint.js", "utf8"))();

const { tintFrom, gradient, contrastWithWhite } = window.coverTint;

test("white text is readable on the tint of any colour", () => {
	const steps = [0, 32, 64, 96, 128, 160, 192, 224, 255];
	for (const r of steps) {
		for (const g of steps) {
			for (const b of steps) {
				const t = tintFrom(r, g, b);
				assert.ok(contrastWithWhite(t) >= 4.5, `rgb(${r} ${g} ${b}) -> ${JSON.stringify(t)} has contrast ${contrastWithWhite(t)}`);
			}
		}
	}
});

test("bright colours are darkened and toned down", () => {
	const t = tintFrom(255, 255, 0);
	assert.ok(Math.max(t.r, t.g, t.b) < 140, JSON.stringify(t));
	const hot = tintFrom(255, 0, 0);
	assert.ok(hot.r > hot.g && hot.r > hot.b, "keeps its hue");
	assert.ok(hot.g > 0, "saturation is clamped, so it is not pure red");
});

test("grey stays grey and black does not vanish", () => {
	const grey = tintFrom(128, 128, 128);
	assert.equal(grey.r, grey.g);
	assert.equal(grey.g, grey.b);
	const black = tintFrom(0, 0, 0);
	assert.ok(black.r > 0, "a faint tint of black remains");
});

test("the gradient goes from the tint to something darker", () => {
	assert.equal(gradient({ r: 100, g: 50, b: 20 }), "linear-gradient(to bottom, rgb(100 50 20), rgb(35 18 7))");
});
