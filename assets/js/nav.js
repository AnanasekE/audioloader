// Phone "More" sheet: a <dialog> opened by any [data-sheet-open] button.
// Esc closes it natively; a click on the backdrop (the dialog itself) does too.
// Listeners are delegated on document because hx-boost swaps the body.
document.addEventListener("click", (e) => {
	const opener = e.target.closest("[data-sheet-open]");
	if (opener) {
		document.getElementById(opener.dataset.sheetOpen)?.showModal();
		return;
	}
	if (e.target instanceof HTMLDialogElement && e.target.id.endsWith("-sheet")) {
		e.target.close();
	}
});

// Search: Cmd/Ctrl+K, or / outside a field, focuses the top-strip search.
// Arrow keys move through the suggestions, Esc clears and blurs.
const searchInput = () => document.querySelector('[role="search"] input[name="q"]');

function setSearchHint() {
	const hint = document.getElementById("search-hint");
	if (hint) hint.textContent = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘K" : "Ctrl K";
}
setSearchHint();
document.addEventListener("htmx:afterSettle", setSearchHint);

document.addEventListener("keydown", (e) => {
	const input = searchInput();
	if (!input) return;
	const t = e.target;
	const inField = t instanceof Element && t.closest("input, textarea, select, [contenteditable]");

	if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key.toLowerCase() === "k") {
		e.preventDefault();
		input.focus();
		input.select();
		return;
	}
	if (e.key === "/" && !e.metaKey && !e.ctrlKey && !e.altKey && !inField) {
		e.preventDefault();
		input.focus();
		input.select();
		return;
	}

	const form = input.form;
	if (!form || !(t instanceof Element) || !form.contains(t)) return;
	const suggest = document.getElementById("search-suggest");
	if (e.key === "Escape") {
		suggest?.replaceChildren();
		input.blur();
		return;
	}
	if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
	const links = [...(suggest?.querySelectorAll("a") ?? [])];
	if (!links.length) return;
	e.preventDefault();
	const i = links.indexOf(t);
	if (e.key === "ArrowDown") links[Math.min(i + 1, links.length - 1)].focus();
	else if (i <= 0) input.focus();
	else links[i - 1].focus();
});
