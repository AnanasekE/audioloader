// Runs assets/js/tags.js against the tagged fixtures: node --test tests/js
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
globalThis.window = globalThis;
// tags.js is a plain browser script; evaluate it like a <script> tag would
new Function(readFileSync(root + "assets/js/tags.js", "utf8"))();

const fixture = (name) => new File([readFileSync(root + "internal/tags/testdata/" + name)], name);

const REC = "f3bba4cd-8018-468b-902e-bc8f029593e5";
const REL = "0d33ef7a-1f5d-4365-b807-b412271b99c3";
const RG = "6f9f6899-c0d3-311d-ae87-a10ae6bc53a9";
const ART = "10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8";

for (const name of ["tagged.mp3", "tagged.flac", "tagged.m4a"]) {
	test(`MusicBrainz IDs from ${name}`, async () => {
		const t = await window.readTags(fixture(name));
		assert.equal(t.title, "Teardrop");
		assert.deepEqual(t.mb, { recording: REC, release: REL, releaseGroup: RG, artists: [ART] });
		assert.equal(t.isrc, "GBAAA9800322");
	});
}
