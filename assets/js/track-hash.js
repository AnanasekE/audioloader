// Track fingerprints, matching db.Fingerprint on the server: file size plus
// a SHA-256 of the first HEAD_SIZE bytes. Used to spot duplicates before upload;
// the server checks again on upload, so this is only an early warning.
// (Not named fingerprint.js: privacy blockers drop scripts with that name.)
window.trackFingerprint = (() => {
	// must match db.HeadSize
	const HEAD_SIZE = 64 * 1024;

	// crypto.subtle only exists on secure origins (https or localhost)
	const supported = () => Boolean(window.crypto?.subtle);

	async function of(file) {
		const head = await file.slice(0, HEAD_SIZE).arrayBuffer();
		const digest = await crypto.subtle.digest("SHA-256", head);
		const hash = [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
		return { size: file.size, hash };
	}

	// exist resolves to one boolean per fingerprint: true if already in the library
	async function exist(fps) {
		const res = await fetch("/api/tracks/exists", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(fps),
		});
		if (!res.ok) throw new Error(`exists check failed: ${res.status}`);
		return res.json();
	}

	return { HEAD_SIZE, supported, of, exist };
})();
