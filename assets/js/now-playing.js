// The phone Now Playing sheet (nowPlaying in internal/components/layout.templ):
// opening and closing it, the drag down to close, the back gesture, and its
// cover, tint and title. player.js owns playback and feeds the sheet through
// setTrack and setPlaying; the sheet's buttons and bars are wired there.
(() => {
	// a drag down closes past this share of the sheet's height, or faster than FLICK (px/ms)
	const CLOSE_AT = 0.25;
	const FLICK = 0.5;
	const SLIDE_MS = 200;

	// shouldClose says whether letting go after dragging down dy px ends the sheet
	function shouldClose(dy, velocity, height) {
		return dy > 0 && (dy > height * CLOSE_AT || velocity > FLICK);
	}

	window.nowPlaying = { shouldClose };
	if (typeof document === "undefined") return;

	const dialog = document.getElementById("now-playing");
	if (!dialog) return;
	const cover = dialog.querySelector("[data-np-cover]");
	const meta = dialog.querySelector("[data-np-meta]");
	const grab = dialog.querySelector("[data-np-grab]");
	const queueToggle = dialog.querySelector("[data-np-queue-toggle]");
	const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)");
	const phone = matchMedia("(max-width: 719px)");

	let track = null;

	// loadMeta shows the title and artists as text at once, then the server's
	// version with links and the ⋯ menu
	async function loadMeta(t) {
		const title = document.createElement("strong");
		title.className = "block truncate text-xl font-bold";
		title.textContent = t.title;
		const artists = document.createElement("span");
		artists.className = "block truncate text-lg text-white/70";
		artists.textContent = t.artists;
		meta.replaceChildren(title, artists);
		try {
			const res = await fetch(`/ui/tracks/${t.id}/playing`);
			if (!res.ok || t !== track || !dialog.open) return;
			meta.innerHTML = await res.text();
			window.htmx?.process(meta);
		} catch {
			// the plain text stays
		}
	}

	// sync fills the sheet for the playing track; it runs when the sheet opens
	// and when the track changes while it is open
	function sync() {
		if (!dialog.open || !track) return;
		cover.src = track.img;
		loadMeta(track);
		const src = track.img;
		window.coverTint?.fromImage(src).then((tint) => {
			if (track?.img === src) dialog.style.background = tint ? window.coverTint.gradient(tint) : "";
		});
	}

	function open() {
		if (dialog.open || !track || !phone.matches) return;
		dialog.showModal();
		history.pushState({ nowPlaying: true }, "", "#now-playing");
		sync();
	}

	// finish takes the sheet away at once
	function finish() {
		dialog.style.transition = dialog.style.transform = "";
		if (dialog.open) dialog.close();
	}

	// slideOut moves the sheet off the screen, then closes it
	function slideOut() {
		if (!dialog.open) return;
		if (reducedMotion.matches) return finish();
		dialog.style.transition = `transform ${SLIDE_MS}ms ease-in`;
		dialog.style.transform = "translateY(100%)";
		setTimeout(finish, SLIDE_MS + 10);
	}

	// close is closing by hand: it pops the history entry opening pushed, and
	// the popstate does the rest
	function close() {
		if (!dialog.open) return;
		if (history.state?.nowPlaying) history.back();
		else slideOut();
	}

	// closeForNavigation closes at once for a page change, and swaps the
	// history entry for the plain address, so nothing is left behind it
	function closeForNavigation() {
		if (!dialog.open) return;
		if (history.state?.nowPlaying) history.replaceState(null, "", location.pathname + location.search);
		finish();
	}

	window.addEventListener("popstate", () => dialog.open && slideOut());
	dialog.addEventListener("cancel", (e) => (e.preventDefault(), close()));
	dialog.addEventListener("click", (e) => e.target.closest("a[href]") && closeForNavigation());
	document.addEventListener("htmx:beforeSwap", (e) => e.detail?.boosted && closeForNavigation());
	document.addEventListener("htmx:historyRestore", closeForNavigation);
	phone.addEventListener("change", () => phone.matches || closeForNavigation());
	// a reload on the sheet's address must not leave the hash behind
	if (location.hash === "#now-playing") history.replaceState(null, "", location.pathname + location.search);

	// the handle: a tap closes, a drag down moves the sheet with the finger
	let drag = null;
	grab.addEventListener("pointerdown", (e) => {
		grab.setPointerCapture(e.pointerId);
		dialog.style.transition = "";
		drag = { y0: e.clientY, dy: 0, last: { y: e.clientY, t: e.timeStamp }, velocity: 0 };
	});
	grab.addEventListener("pointermove", (e) => {
		if (!drag) return;
		drag.dy = Math.max(0, e.clientY - drag.y0);
		const dt = e.timeStamp - drag.last.t;
		if (dt > 0) drag.velocity = (e.clientY - drag.last.y) / dt;
		drag.last = { y: e.clientY, t: e.timeStamp };
		if (drag.dy > 0) dialog.style.transform = `translateY(${drag.dy}px)`;
	});
	const endDrag = (e) => {
		if (!drag) return;
		const { dy, velocity } = drag;
		drag = null;
		if (e.type === "pointerup" && dy < 6) return close();
		if (e.type === "pointerup" && shouldClose(dy, velocity, dialog.clientHeight)) return close();
		// back where it was
		dialog.style.transition = `transform ${SLIDE_MS}ms ease-out`;
		dialog.style.transform = "";
		setTimeout(() => (dialog.style.transition = ""), SLIDE_MS + 10);
	};
	grab.addEventListener("pointerup", endDrag);
	grab.addEventListener("pointercancel", endDrag);
	// the keyboard's Enter and Space click the handle too (a pointer tap is handled above)
	grab.addEventListener("click", (e) => e.detail === 0 && close());

	// the ⋯ opens the menu as a bottom sheet, as a long press on a row does
	meta.addEventListener("click", (e) => {
		const summary = e.target.closest("summary");
		const host = meta.querySelector("[data-menu-host]");
		if (!summary || !host) return;
		e.preventDefault();
		const r = summary.getBoundingClientRect();
		window.contextMenu?.openAt(host, r.right, r.bottom);
	});

	// the Queue button swaps the cover and controls for the queue
	function showQueue(on) {
		queueToggle.setAttribute("aria-pressed", String(on));
		dialog.querySelector("[data-np-art-view]").hidden = on;
		dialog.querySelector("[data-np-controls]").hidden = on;
		const q = dialog.querySelector("[data-np-queue-view]");
		q.hidden = !on;
		// the history above stays out of view until scrolled up to
		if (on) {
			const scroll = q.querySelector("[data-np-scroll]");
			scroll.scrollTop = q.querySelector("[data-np-upnext]").offsetTop;
		}
	}
	queueToggle.addEventListener("click", () => showQueue(queueToggle.getAttribute("aria-pressed") !== "true"));

	Object.assign(window.nowPlaying, {
		open,
		close,
		// setTrack is the track that plays, or null when none does
		setTrack(t) {
			track = t;
			if (!t) closeForNavigation();
			else sync();
		},
		// setPlaying shrinks the cover while paused
		setPlaying(playing) {
			cover.toggleAttribute("data-paused", !playing);
		},
	});
})();
