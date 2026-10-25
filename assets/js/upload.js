// Single upload form (AddTrackForm in internal/components/tracks.templ):
// checks the picked file against the library before it can be submitted.
(() => {
	const form = document.getElementById("upload-form");
	if (!form) return;

	// track-hash.js; if it failed to load, skip the early check (the server still rejects duplicates)
	const { trackFingerprint } = window;
	const input = form.querySelector("input[name=track]");
	const submit = form.querySelector("button[type=submit]");
	const status = document.getElementById("track-file-status");

	const EXISTS_MSG = "This track already exists in the library.";
	let checkID = 0; // a newer pick or reset makes older checks stale

	function setStatus(state, msg) {
		status.dataset.state = state;
		status.textContent = msg;
	}

	// fileDuration reads a file's length in seconds through an <audio>
	// element, or resolves 0 if the browser can't tell
	function fileDuration(file) {
		return new Promise((resolve) => {
			const el = document.createElement("audio");
			const url = URL.createObjectURL(file);
			const done = (d) => (URL.revokeObjectURL(url), resolve(d));
			el.preload = "metadata";
			el.onloadedmetadata = () => done(Number.isFinite(el.duration) ? el.duration : 0);
			el.onerror = () => done(0);
			el.src = url;
		});
	}

	// Files over 30 minutes are usually podcast episodes: move the file to
	// the episode form. Picking "Single" again moves back by hand.
	const LONG = 30 * 60;
	async function suggestEpisode(file) {
		const episodeInput = document.getElementById("episode-file");
		const episodeKind = document.getElementById("kind-episode");
		if (!episodeInput || !episodeKind || (await fileDuration(file)) < LONG) return false;
		const dt = new DataTransfer();
		dt.items.add(file);
		episodeInput.files = dt.files;
		const title = form.querySelector("input[name=title]").value;
		if (title) document.getElementById("episode-title").value = title;
		episodeKind.checked = true;
		document.getElementById("result-episode").textContent = "This file is over 30 minutes, so it's set up as a podcast episode. Pick Single above if it's music.";
		input.value = "";
		return true;
	}

	if (location.hash === "#episode") document.getElementById("kind-episode")?.click();

	input.addEventListener("change", async () => {
		if (input.files[0] && (await suggestEpisode(input.files[0]))) return;
		const id = ++checkID;
		// a custom validity message blocks submission until cleared
		input.setCustomValidity("");
		setStatus("", "");

		const file = input.files[0];
		if (!file || !trackFingerprint?.supported()) return;

		// no submitting until we know whether it's a duplicate
		submit.disabled = true;
		setStatus("pending", "Checking library…");
		try {
			const [exists] = await trackFingerprint.exist([await trackFingerprint.of(file)]);
			if (id !== checkID) return;
			if (exists) {
				input.setCustomValidity(EXISTS_MSG);
				setStatus("exists", EXISTS_MSG);
			} else {
				setStatus("", "");
			}
		} catch {
			// best effort: the upload itself still rejects duplicates
			if (id === checkID) setStatus("", "");
		} finally {
			if (id === checkID) submit.disabled = false;
		}
	});

	form.addEventListener("reset", () => {
		checkID++;
		input.setCustomValidity("");
		setStatus("", "");
		submit.disabled = false;
	});
})();
