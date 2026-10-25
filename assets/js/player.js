// Global player (player in internal/components/layout.templ). Track rows hand
// their track to it via [data-play-track] buttons. Playback state is saved to
// localStorage, so a reload resumes the same track at the same position.
(() => {
	const STORAGE_KEY = "audioloader.player";

	const root = document.getElementById("player");
	const audio = document.getElementById("player-audio");
	const cover = document.getElementById("player-cover");
	const title = document.getElementById("player-title");
	const artists = document.getElementById("player-artists");

	let current = null; // {id, title, artists, img, src}
	let lastSave = 0;

	function save() {
		lastSave = Date.now();
		try {
			if (!current) {
				localStorage.removeItem(STORAGE_KEY);
				return;
			}
			localStorage.setItem(
				STORAGE_KEY,
				JSON.stringify({ track: current, time: audio.currentTime, volume: audio.volume, paused: audio.paused }),
			);
		} catch {
			// storage can be full or disabled (private mode); playback still works
		}
	}

	function restoreState() {
		try {
			return JSON.parse(localStorage.getItem(STORAGE_KEY));
		} catch {
			return null;
		}
	}

	function show(track) {
		current = track;
		root.hidden = false;
		cover.src = track.img;
		title.textContent = track.title;
		artists.textContent = track.artists;
		if ("mediaSession" in navigator) {
			navigator.mediaSession.metadata = new MediaMetadata({
				title: track.title,
				artist: track.artists,
				artwork: [{ src: new URL(track.img, location.href).href }],
			});
		}
	}

	function load(track, { time = 0, autoplay = true, restoring = false } = {}) {
		show(track);
		audio.src = track.src;
		if (time) audio.addEventListener("loadedmetadata", () => (audio.currentTime = time), { once: true });
		// browsers block autoplay without a user gesture (e.g. after a reload); the user then presses play
		if (autoplay) audio.play().catch(() => {});
		markRows();
		// when restoring, saving now would overwrite the stored position with 0
		// before loadedmetadata has applied it
		if (!restoring) save();
	}

	function clear() {
		current = null;
		root.hidden = true;
		audio.removeAttribute("src");
		audio.load();
		markRows();
		save();
	}

	function markRows() {
		const playingID = current && !audio.paused ? current.id : null;
		document.querySelectorAll("[data-play-track]").forEach((btn) => {
			const playing = btn.dataset.id === playingID;
			btn.setAttribute("aria-pressed", String(playing));
			btn.textContent = playing ? "❚❚" : "▶";
		});
	}

	document.addEventListener("click", (e) => {
		const btn = e.target.closest("[data-play-track]");
		if (!btn) return;
		const d = btn.dataset;
		if (current?.id === d.id) {
			if (audio.paused) audio.play().catch(() => {});
			else audio.pause();
		} else {
			load({ id: d.id, title: d.title, artists: d.artists, img: d.img, src: d.src });
		}
	});

	audio.addEventListener("play", () => (markRows(), save()));
	audio.addEventListener("pause", () => (markRows(), save()));
	audio.addEventListener("ended", () => (markRows(), save()));
	audio.addEventListener("volumechange", save);
	audio.addEventListener("timeupdate", () => {
		if (Date.now() - lastSave > 2000) save();
	});
	// the saved track may have been deleted since; don't keep a broken player around
	audio.addEventListener("error", () => {
		if (current && audio.getAttribute("src")) clear();
	});
	window.addEventListener("pagehide", save);

	// boosted navigation swaps in new track rows; show which one is playing
	document.addEventListener("htmx:afterSettle", markRows);

	if ("mediaSession" in navigator) {
		const ms = navigator.mediaSession;
		ms.setActionHandler("play", () => audio.play());
		ms.setActionHandler("pause", () => audio.pause());
		ms.setActionHandler("seekto", (e) => (audio.currentTime = e.seekTime));
		ms.setActionHandler("seekbackward", (e) => (audio.currentTime = Math.max(0, audio.currentTime - (e.seekOffset || 10))));
		ms.setActionHandler("seekforward", (e) => (audio.currentTime += e.seekOffset || 10));
	}

	const state = restoreState();
	if (state?.track) {
		if (typeof state.volume === "number") audio.volume = state.volume;
		load(state.track, { time: state.time, autoplay: !state.paused, restoring: true });
	}
})();
