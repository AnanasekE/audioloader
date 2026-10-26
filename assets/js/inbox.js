// Remembers which artist groups of the inbox are collapsed, in localStorage.
// Groups are <details data-artist="id">; they start open.
(() => {
	const KEY = "inbox-collapsed";

	function collapsed() {
		try {
			const ids = JSON.parse(localStorage.getItem(KEY));
			return Array.isArray(ids) ? ids : [];
		} catch {
			return [];
		}
	}

	function restore() {
		const ids = collapsed();
		for (const d of document.querySelectorAll("details[data-artist]")) {
			d.open = !ids.includes(d.dataset.artist);
		}
	}

	restore();
	if (window.inboxGroupsBound) return;
	window.inboxGroupsBound = true;

	// the list is swapped when filters change
	document.addEventListener("htmx:afterSettle", restore);
	// toggle doesn't bubble, so listen on the capture phase
	document.addEventListener(
		"toggle",
		(e) => {
			const d = e.target;
			if (!(d instanceof HTMLDetailsElement) || !d.dataset.artist) return;
			const ids = collapsed().filter((id) => id !== d.dataset.artist);
			if (!d.open) ids.push(d.dataset.artist);
			try {
				localStorage.setItem(KEY, JSON.stringify(ids));
			} catch {
				// private mode or storage full: the groups just reopen next time
			}
		},
		true,
	);
})();
