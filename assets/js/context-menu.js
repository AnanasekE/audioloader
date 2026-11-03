// Context menus: right-click, long-press or the ContextMenu key on a
// [data-menu-host] opens that host's own <details data-menu>, the same ⋯ menu
// the server renders, so its htmx attributes keep working. At the pointer on
// wide screens, as a bottom sheet below 720px. One menu is open at a time.
// Listeners are delegated on document because hx-boost swaps the body.
(() => {
	const MARGIN = 8;

	// placeMenu returns the top-left corner for a w x h menu opened at the
	// point x, y in a vw x vh viewport: flipped to the other side of the
	// point when it would run out, then kept inside with a small margin.
	function placeMenu(x, y, w, h, vw, vh) {
		const fit = (p, size, view) => {
			if (p + size > view - MARGIN) p -= size;
			return Math.max(MARGIN, Math.min(p, view - size - MARGIN));
		};
		return { left: fit(x, w, vw), top: fit(y, h, vh) };
	}

	window.contextMenu = { placeMenu };
	if (typeof document === "undefined") return;

	const LONG_PRESS_MS = 500;
	const LONG_PRESS_SLOP = 10;
	const sheetQuery = matchMedia("(max-width: 719px)");

	let backdrop = null;

	const openMenus = () => [...document.querySelectorAll("details[data-menu][open]")];

	// menuOf is the menu of a host, not one belonging to a host inside it
	function menuOf(host) {
		return [...host.querySelectorAll("details[data-menu]")].find((m) => m.closest("[data-menu-host]") === host) ?? null;
	}

	function close(menu) {
		menu.removeAttribute("open");
		cleanup(menu);
	}

	// cleanup undoes the context placement; the toggle event calls it as well,
	// for menus whose own buttons close them
	function cleanup(menu) {
		delete menu.dataset.ctx;
		const pop = menu.querySelector(":scope > .pop");
		if (pop) pop.style.cssText = "";
		// the toggle event is async: it may be a menu closed earlier
		if (backdrop?.menu === menu) {
			backdrop.remove();
			backdrop = null;
		}
	}

	function closeAll(except) {
		openMenus().forEach((m) => m !== except && close(m));
	}

	// openAt opens the host's menu at the point x, y (viewport coordinates)
	function openAt(host, x, y, { focus = false } = {}) {
		const menu = menuOf(host);
		if (!menu) return null;
		closeAll();
		const sheet = sheetQuery.matches;
		menu.dataset.ctx = sheet ? "sheet" : "pointer";
		menu.setAttribute("open", "");
		const pop = menu.querySelector(":scope > .pop");
		if (sheet) {
			backdrop = document.createElement("div");
			backdrop.className = "menu-backdrop";
			backdrop.menu = menu;
			backdrop.addEventListener("click", () => close(menu));
			document.body.append(backdrop);
		} else if (pop) {
			pop.style.left = pop.style.top = "0";
			const { left, top } = placeMenu(x, y, pop.offsetWidth, pop.offsetHeight, innerWidth, innerHeight);
			pop.style.left = left + "px";
			pop.style.top = top + "px";
		}
		if (focus) items(menu)[0]?.focus();
		return menu;
	}

	const items = (menu) => [...menu.querySelectorAll(".menu-item")].filter((el) => el.offsetParent !== null);

	// the host to return focus to after Esc
	const focusBack = (menu) => {
		const host = menu.closest("[data-menu-host]");
		(host?.hasAttribute("tabindex") ? host : menu.querySelector("summary"))?.focus();
	};

	// the browser's own menu stays for fields, selected text, audio and Shift
	function keepNative(e) {
		if (e.shiftKey) return true;
		const t = e.target;
		if (!(t instanceof Element)) return true;
		if (t.closest("input, textarea, select, audio, [contenteditable]")) return true;
		const sel = getSelection();
		return Boolean(sel && !sel.isCollapsed && sel.containsNode(t, true));
	}

	document.addEventListener("contextmenu", (e) => {
		if (!(e.target instanceof Element) || keepNative(e)) return;
		const host = e.target.closest("[data-menu-host]");
		if (!host || !menuOf(host)) return;
		e.preventDefault();
		const menu = menuOf(host);
		// a long press already opened it; touch browsers then fire this too
		if (menu.open && menu.dataset.ctx) return;
		openAt(host, e.clientX, e.clientY);
	});

	// long-press on touch
	let press = null;
	function endPress() {
		if (press) clearTimeout(press.timer);
		press = null;
	}
	document.addEventListener(
		"touchstart",
		(e) => {
			endPress();
			if (e.touches.length !== 1 || !(e.target instanceof Element)) return;
			const host = e.target.closest("[data-menu-host]");
			if (!host || !menuOf(host) || e.target.closest("input, textarea, select")) return;
			const { clientX: x, clientY: y } = e.touches[0];
			press = {
				x,
				y,
				timer: setTimeout(() => {
					press = null;
					openAt(host, x, y);
					// the finger lifting must not click what is under it
					const swallow = (ev) => (ev.preventDefault(), ev.stopPropagation());
					document.addEventListener("click", swallow, { capture: true, once: true });
					setTimeout(() => document.removeEventListener("click", swallow, true), 600);
				}, LONG_PRESS_MS),
			};
		},
		{ passive: true },
	);
	document.addEventListener(
		"touchmove",
		(e) => {
			if (!press) return;
			const t = e.touches[0];
			if (Math.hypot(t.clientX - press.x, t.clientY - press.y) > LONG_PRESS_SLOP) endPress();
		},
		{ passive: true },
	);
	document.addEventListener("touchend", endPress);
	document.addEventListener("touchcancel", endPress);

	// closing: a click outside, scrolling, a page swap
	document.addEventListener("click", (e) => {
		if (!(e.target instanceof Element)) return;
		openMenus().forEach((m) => m.contains(e.target) || close(m));
	});
	document.addEventListener(
		"scroll",
		(e) => {
			if (e.target instanceof Element && e.target.closest("details[data-menu]")) return;
			openMenus().forEach((m) => m.dataset.ctx && close(m));
		},
		true,
	);
	// only a page change: other swaps (a lazily loaded card) leave the menu be
	document.addEventListener("htmx:beforeSwap", (e) => e.detail?.boosted && closeAll());
	document.addEventListener("htmx:historyRestore", () => closeAll());
	// a menu removed with its row (Delete) leaves its backdrop behind
	document.addEventListener("htmx:afterSettle", () => {
		if (backdrop && !document.querySelector("details[data-menu][data-ctx]")) (backdrop.remove(), (backdrop = null));
	});
	// toggle doesn't bubble: capture it. A menu that closes itself is cleaned
	// up, and opening one by its ⋯ button closes any other.
	document.addEventListener(
		"toggle",
		(e) => {
			const menu = e.target;
			if (!(menu instanceof HTMLDetailsElement) || !menu.hasAttribute("data-menu")) return;
			if (menu.open) closeAll(menu);
			else cleanup(menu);
		},
		true,
	);

	document.addEventListener("keydown", (e) => {
		const t = e.target;
		if (!(t instanceof Element)) return;
		const open = openMenus().find((m) => m.contains(t)) ?? openMenus()[0];
		if (e.key === "Escape" && open) {
			e.preventDefault();
			close(open);
			focusBack(open);
			return;
		}
		if (e.key === "ContextMenu" || (e.key === "F10" && e.shiftKey)) {
			const host = t.closest("[data-menu-host]");
			if (!host || !menuOf(host) || t.closest("input, textarea, select, [contenteditable]")) return;
			e.preventDefault();
			const r = host.getBoundingClientRect();
			openAt(host, r.left + Math.min(r.width / 2, 120), r.bottom - r.height / 2, { focus: true });
			return;
		}
		if ((e.key === "ArrowDown" || e.key === "ArrowUp") && open && open.contains(t)) {
			const list = items(open);
			if (!list.length) return;
			e.preventDefault();
			const i = list.indexOf(t);
			if (e.key === "ArrowDown") list[(i + 1) % list.length].focus();
			else list[(i <= 0 ? list.length : i) - 1].focus();
		}
	});

	// a hook for tests and screenshots
	Object.assign(window.contextMenu, { openAt, closeAll });
})();
