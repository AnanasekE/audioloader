// "Download from URL" page (SecretSourcePage in internal/components/secretsource.templ):
// a creator added with the "New creator" form joins the picker, already ticked.
(() => {
	const root = document.getElementById("secret-source");
	if (!root) return;

	// the server's HX-Trigger carries {id, name} of the new creator
	document.body.addEventListener("artistsChanged", function onArtistsChanged(e) {
		// body survives boosted navigation, this page's elements don't
		if (!root.isConnected) {
			document.body.removeEventListener("artistsChanged", onArtistsChanged);
			return;
		}
		const { id, name } = e.detail;
		if (!id) return;
		for (const options of root.querySelectorAll("[data-artist-options]")) {
			const label = document.createElement("label");
			label.className = "flex cursor-pointer items-center gap-2 rounded px-1 py-1 text-sm hover:bg-zinc-100 dark:hover:bg-zinc-700";
			const input = document.createElement("input");
			input.type = "checkbox";
			input.name = "artist_id";
			input.value = id;
			input.dataset.name = name;
			input.checked = true;
			label.append(input, name);
			options.append(label);
			input.dispatchEvent(new Event("change", { bubbles: true }));
		}
	});
})();
