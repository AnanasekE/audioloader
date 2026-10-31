// Tag lens (the Tags page, secret mode). Every control on the page is a
// link that works without JS; this adds keyboard and touch shortcuts on top.
// Listeners sit on document because htmx swaps the lens region, and the
// guard keeps a second visit to the page from binding them twice.
(() => {
	if (window.__tagLensBound) return;
	window.__tagLensBound = true;

	const LONG_PRESS_MS = 500;

	// follow loads href into the lens the way a chip link does, pushing the URL
	let lastFollow = 0;
	function follow(href, { keepFocus = false } = {}) {
		// a long press can raise both the timer and a contextmenu event
		if (!href || Date.now() - lastFollow < 800) return;
		lastFollow = Date.now();
		const a = document.createElement("a");
		a.href = href;
		a.hidden = true;
		a.setAttribute("hx-get", href);
		a.setAttribute("hx-target", "#tag-lens");
		a.setAttribute("hx-select", "#tag-lens");
		a.setAttribute("hx-swap", "outerHTML");
		a.setAttribute("hx-push-url", "true");
		a.setAttribute("hx-boost", "false");
		(document.getElementById("tag-lens") ?? document.body).append(a);
		if (keepFocus) refocus = true;
		window.htmx.process(a);
		a.click();
		a.remove();
	}

	let refocus = false;
	// after a keyboard-driven swap, put the cursor back in the filter box
	document.addEventListener("htmx:afterSettle", () => {
		if (!refocus) return;
		refocus = false;
		document.getElementById("tag-filter")?.focus();
	});

	const mainChips = () => [...document.querySelectorAll("[data-tag-field] > [data-tag-chip]")];

	document.addEventListener("input", (e) => {
		if (e.target.id !== "tag-filter") return;
		const prefix = e.target.value.trim().toLowerCase();
		document.querySelectorAll("#tag-lens [data-tag-chip]").forEach((chip) => {
			chip.hidden = prefix !== "" && !chip.dataset.tag.toLowerCase().startsWith(prefix);
		});
	});

	document.addEventListener("keydown", (e) => {
		if (e.target.id !== "tag-filter") return;
		if (e.key === "Enter") {
			e.preventDefault();
			const top = mainChips().find((c) => !c.hidden && !c.classList.contains("hidden"));
			if (top) follow(e.shiftKey ? top.dataset.exHref : top.dataset.inHref, { keepFocus: true });
		} else if (e.key === "Backspace" && e.target.value === "") {
			const removes = document.querySelectorAll("#tag-lens [data-token-remove]");
			const last = removes[removes.length - 1];
			if (last) {
				e.preventDefault();
				follow(last.getAttribute("href"), { keepFocus: true });
			}
		}
	});

	// right-click or long-press excludes
	const chipOf = (e) => (e.target instanceof Element ? e.target.closest("[data-tag-chip]") : null);

	document.addEventListener("contextmenu", (e) => {
		const chip = chipOf(e);
		if (!chip) return;
		e.preventDefault();
		if (!pressed) follow(chip.dataset.exHref);
	});

	let timer = null;
	let startX = 0;
	let startY = 0;
	let pressed = false; // a long press fired; swallow the click that follows
	const cancel = () => {
		clearTimeout(timer);
		timer = null;
	};

	document.addEventListener("pointerdown", (e) => {
		const chip = chipOf(e);
		if (!chip || e.pointerType === "mouse") return;
		cancel();
		pressed = false;
		startX = e.clientX;
		startY = e.clientY;
		timer = setTimeout(() => {
			pressed = true;
			follow(chip.dataset.exHref);
		}, LONG_PRESS_MS);
	});
	for (const type of ["pointerup", "pointercancel"]) document.addEventListener(type, cancel);
	document.addEventListener("pointermove", (e) => {
		if (timer && Math.hypot(e.clientX - startX, e.clientY - startY) > 10) cancel();
	});

	document.addEventListener(
		"click",
		(e) => {
			if (pressed && chipOf(e)) {
				e.preventDefault();
				e.stopImmediatePropagation();
			}
			pressed = false;
		},
		true,
	);
})();
