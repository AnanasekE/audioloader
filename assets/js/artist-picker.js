// Artist picker dropdowns (artistPicker in internal/components/artists.templ):
// keeps the summary showing the checked names, fuzzy-filters the list from
// the search box, and closes on outside clicks.
(() => {
	// boosted navigation re-runs page scripts; the document-level listeners below must only be added once
	if (window.artistPickerReady) return;
	window.artistPickerReady = true;

	function refresh(picker) {
		const names = [...picker.querySelectorAll("input[name=artist_id]:checked")].map((c) => c.dataset.name);
		picker.querySelector("[data-artist-summary]").textContent = names.length ? names.join(", ") : "Select artists";
	}

	// fuzzyScore matches query as a subsequence of text, case-insensitively
	// ("hmk" matches "Hatsune Miku"). Returns null for no match; lower is better:
	// consecutive letters and matches at word starts score best.
	function fuzzyScore(query, text) {
		const q = query.toLowerCase();
		const t = text.toLowerCase();
		let score = 0;
		let ti = 0;
		let prev = -1;
		for (const ch of q) {
			const found = t.indexOf(ch, ti);
			if (found === -1) return null;
			const wordStart = found === 0 || /[\s\-_.]/.test(t[found - 1]);
			score += found - prev - 1;
			if (!wordStart && found !== prev + 1) score += 1;
			prev = found;
			ti = found + 1;
		}
		return score;
	}

	function filter(picker) {
		const query = picker.querySelector("[data-artist-search]").value.trim();
		const list = picker.querySelector("[data-artist-options]");
		const labels = [...list.querySelectorAll("label")];

		// remember the server's order so clearing the search restores it
		labels.forEach((label, i) => {
			if (label.dataset.order === undefined) label.dataset.order = i;
		});

		const scored = labels.map((label) => ({
			label,
			score: query ? fuzzyScore(query, label.querySelector("input").dataset.name) : 0,
		}));
		scored.sort((a, b) => (a.score ?? Infinity) - (b.score ?? Infinity) || a.label.dataset.order - b.label.dataset.order);

		for (const { label, score } of scored) {
			label.hidden = score === null;
			list.append(label);
		}
		picker.querySelector("[data-artist-empty]").hidden = scored.some((s) => s.score !== null);
	}

	document.addEventListener("change", (e) => {
		const picker = e.target.closest("[data-artist-picker]");
		if (picker && e.target.name === "artist_id") refresh(picker);
	});

	document.addEventListener("input", (e) => {
		if (e.target.matches("[data-artist-search]")) filter(e.target.closest("[data-artist-picker]"));
	});

	// Enter in the search box would submit the form; toggle the top match instead
	document.addEventListener("keydown", (e) => {
		if (e.key !== "Enter" || !e.target.matches("[data-artist-search]")) return;
		e.preventDefault();
		const picker = e.target.closest("[data-artist-picker]");
		const first = picker.querySelector("[data-artist-options] label:not([hidden]) input");
		if (!first || first.disabled) return;
		first.checked = !first.checked;
		first.dispatchEvent(new Event("change", { bubbles: true }));
	});

	// toggle doesn't bubble, so listen in the capture phase
	document.addEventListener(
		"toggle",
		(e) => {
			const picker = e.target;
			if (!picker.matches?.("[data-artist-picker]")) return;
			const search = picker.querySelector("[data-artist-search]");
			if (picker.open) {
				search.focus();
			} else if (search.value) {
				search.value = "";
				filter(picker);
			}
		},
		true,
	);

	// form.reset() restores the checkboxes after the reset event fires, so refresh on the next tick
	document.addEventListener(
		"reset",
		(e) => setTimeout(() => e.target.querySelectorAll("[data-artist-picker]").forEach(refresh)),
		true,
	);

	document.addEventListener("click", (e) => {
		document.querySelectorAll("[data-artist-picker][open]").forEach((picker) => {
			if (!picker.contains(e.target)) picker.open = false;
		});
	});

	document.querySelectorAll("[data-artist-picker]").forEach(refresh);
	// pickers that arrive later, through boosted navigation or swaps
	document.addEventListener("htmx:load", (e) => e.detail.elt.querySelectorAll?.("[data-artist-picker]").forEach(refresh));
})();
