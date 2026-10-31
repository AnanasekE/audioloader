// Filter inputs ([data-filter]) hide the rows ([data-filter-row]) of the
// list marked [data-filter-rows] on the same page whose text doesn't contain
// what was typed.
document.addEventListener("input", (e) => {
	const input = e.target.closest?.("[data-filter]");
	if (!input) return;
	const q = input.value.trim().toLowerCase();
	document.querySelectorAll("[data-filter-rows] [data-filter-row]").forEach((row) => {
		row.hidden = q !== "" && !row.textContent.toLowerCase().includes(q);
	});
});
