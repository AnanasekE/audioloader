// Filter box for the card grids (Releases, Artists). An input marked
// [data-grid-filter] hides the [data-filter-text] cards of the page whose
// text (lower-cased title and artist names) doesn't contain every word typed,
// and the shelves left with no card.
window.gridFilter = {
	// matches reports whether text contains every word of query (any case).
	matches(text, query) {
		const t = text.toLowerCase();
		return query
			.toLowerCase()
			.split(/\s+/)
			.filter(Boolean)
			.every((w) => t.includes(w));
	},
};

document.addEventListener("input", (e) => {
	const input = e.target.closest?.("[data-grid-filter]");
	if (!input) return;
	const cards = document.querySelectorAll("[data-filter-text]");
	cards.forEach((c) => {
		c.hidden = !window.gridFilter.matches(c.dataset.filterText, input.value);
	});
	document.querySelectorAll("[data-filter-shelf]").forEach((s) => {
		s.hidden = !s.querySelector("[data-filter-text]:not([hidden])");
	});
});
