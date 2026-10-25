// Playlist editor (PlaylistEditPage in internal/components/playlists.templ):
// reorder and remove rows. The form posts track_id per row, in order.
(() => {
	const form = document.getElementById("playlist-edit");
	if (!form) return;
	const rows = document.getElementById("playlist-edit-rows");
	const empty = form.querySelector("[data-edit-empty]");

	function renumber() {
		rows.querySelectorAll("[data-number]").forEach((n, i) => (n.textContent = i + 1));
		empty.hidden = rows.children.length > 0;
	}

	form.addEventListener("click", (e) => {
		const btn = e.target.closest("[data-action]");
		if (!btn) return;
		const row = btn.closest(".playlist-edit-row");
		if (btn.dataset.action === "up") row.previousElementSibling?.before(row);
		if (btn.dataset.action === "down") row.nextElementSibling?.after(row);
		if (btn.dataset.action === "remove") row.remove();
		renumber();
	});

	renumber();
})();
