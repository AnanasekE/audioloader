// Phone "More" sheet: a <dialog> opened by any [data-sheet-open] button.
// Esc closes it natively; a click on the backdrop (the dialog itself) does too.
// Listeners are delegated on document because hx-boost swaps the body.
document.addEventListener("click", (e) => {
	const opener = e.target.closest("[data-sheet-open]");
	if (opener) {
		document.getElementById(opener.dataset.sheetOpen)?.showModal();
		return;
	}
	if (e.target instanceof HTMLDialogElement && (e.target.id.endsWith("-sheet") || e.target.id === "shortcuts-dialog")) {
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

// Actions: the box also lists commands (commands.js) under an "Actions"
// heading, above the server's results; ">" first lists only commands, and
// Enter then runs the first. Arrow keys and Enter work on them as on links.
const suggestBox = () => document.getElementById("search-suggest");
const POP = "pop absolute top-full left-0 z-30 mt-1 max-h-[70vh] w-full min-w-80 overflow-y-auto";

function renderActions() {
	const input = searchInput();
	const suggest = suggestBox();
	if (!input || !suggest) return;
	const { only, text } = window.commands.parse(input.value);
	suggest.querySelector("[data-actions]")?.remove();
	if (only) suggest.replaceChildren();
	const found = window.commands.match(window.commands.list, text, only);
	if (!found.length && !only) return;
	const group = document.createElement("div");
	group.dataset.actions = "";
	const heading = document.createElement("h2");
	heading.className = "section-label";
	heading.textContent = "Actions";
	group.append(heading);
	for (const c of found) {
		const b = document.createElement("button");
		b.type = "button";
		b.className = "menu-item min-h-11 py-1 focus:bg-panel-2";
		b.dataset.command = c.id;
		b.textContent = c.label;
		group.append(b);
	}
	if (!found.length) {
		const p = document.createElement("p");
		p.className = "px-2 py-1 text-fg-muted";
		p.textContent = "No action matches.";
		group.append(p);
	}
	let pop = suggest.querySelector(".pop");
	if (!pop) {
		pop = document.createElement("div");
		pop.className = POP;
		suggest.append(pop);
	}
	// "Nothing matches." from the server is no news when an action does
	if (found.length) pop.querySelector(":scope > p")?.remove();
	pop.prepend(group);
}

function runCommand(id) {
	const cmd = window.commands.list.find((c) => c.id === id);
	const input = searchInput();
	suggestBox()?.replaceChildren();
	if (input) {
		input.value = "";
		input.blur();
	}
	cmd?.run();
}

document.addEventListener("input", (e) => e.target === searchInput() && renderActions());
document.addEventListener("focusin", (e) => e.target === searchInput() && renderActions());
// the server's answer replaces the list: put the actions back on top
document.addEventListener("htmx:afterSwap", (e) => e.target.id === "search-suggest" && renderActions());
// ">" is for commands only: no search request for it
document.addEventListener("htmx:beforeRequest", (e) => {
	if (e.target === searchInput() && window.commands.parse(e.target.value).only) e.preventDefault();
});
document.addEventListener("click", (e) => {
	const b = e.target instanceof Element && e.target.closest("[data-command]");
	if (b) runCommand(b.dataset.command);
});

document.addEventListener("keydown", (e) => {
	const t = e.target;
	const inField = t instanceof Element && t.closest("input, textarea, select, [contenteditable]");
	if (e.key === "?" && !inField && !e.metaKey && !e.ctrlKey && !e.altKey) {
		const dialog = document.getElementById("shortcuts-dialog");
		if (dialog && !dialog.open) {
			e.preventDefault();
			dialog.showModal();
		}
		return;
	}
	const input = searchInput();
	if (!input) return;

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
	if (e.key === "Enter" && t === input && window.commands.parse(input.value).only) {
		e.preventDefault();
		const first = suggest?.querySelector("[data-command]");
		if (first) runCommand(first.dataset.command);
		return;
	}
	if (e.key === "Escape") {
		suggest?.replaceChildren();
		input.blur();
		return;
	}
	if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
	const links = [...(suggest?.querySelectorAll("a, [data-command]") ?? [])];
	if (!links.length) return;
	e.preventDefault();
	const i = links.indexOf(t);
	if (e.key === "ArrowDown") links[Math.min(i + 1, links.length - 1)].focus();
	else if (i <= 0) input.focus();
	else links[i - 1].focus();
});
