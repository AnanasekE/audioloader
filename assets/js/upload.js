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

	input.addEventListener("change", async () => {
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
