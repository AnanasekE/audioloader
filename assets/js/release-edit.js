// Tracklist editor (ReleaseEditPage in internal/components/releases.templ):
// reorder, remove and add rows. The form posts track_id and disc per row in
// row order; the server derives positions from that order within each disc.
(() => {
	const form = document.getElementById("release-edit");
	if (!form) return;

	const rows = document.getElementById("release-edit-rows");
	const select = document.getElementById("release-add-track");
	const template = document.getElementById("release-edit-row");
	const empty = form.querySelector("[data-edit-empty]");

	function renumber() {
		const count = {};
		rows.querySelectorAll(".release-edit-row").forEach((row) => {
			const disc = row.querySelector("input[name=disc]").value || "1";
			count[disc] = (count[disc] || 0) + 1;
			row.querySelector("[data-number]").textContent = count[disc];
		});
		empty.hidden = rows.children.length > 0;
	}

	function addSelected() {
		const opt = select.selectedOptions[0];
		if (!opt || !opt.value) return;

		const row = template.content.firstElementChild.cloneNode(true);
		row.querySelector("input[name=track_id]").value = opt.value;
		row.querySelector("[data-title]").textContent = opt.dataset.title;
		row.querySelector("[data-artists]").textContent = opt.dataset.artists;
		// new rows join the last row's disc, so filling disc 3 doesn't need retyping
		const last = rows.lastElementChild;
		row.querySelector("input[name=disc]").value = last ? last.querySelector("input[name=disc]").value : "1";
		rows.append(row);
		opt.remove();
		select.value = "";
	}

	// a removed track goes back into the "add from library" list
	function restoreOption(row) {
		const title = row.querySelector("[data-title]").textContent;
		const artists = row.querySelector("[data-artists]").textContent;
		const opt = new Option(artists ? `${title} — ${artists}` : title, row.querySelector("input[name=track_id]").value);
		opt.dataset.title = title;
		opt.dataset.artists = artists;
		select.append(opt);
	}

	form.addEventListener("click", (e) => {
		const btn = e.target.closest("[data-action]");
		if (!btn) return;
		const row = btn.closest(".release-edit-row");
		switch (btn.dataset.action) {
			case "up":
				row.previousElementSibling?.before(row);
				break;
			case "down":
				row.nextElementSibling?.after(row);
				break;
			case "remove":
				restoreOption(row);
				row.remove();
				break;
			case "add":
				addSelected();
				break;
		}
		renumber();
	});

	form.addEventListener("input", (e) => {
		if (e.target.name === "disc") renumber();
	});

	renumber();
})();
