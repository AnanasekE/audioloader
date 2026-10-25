// Global player (player in internal/components/layout.templ). Track rows hand
// their track to it via [data-play-track] buttons; playing one inside a
// [data-track-list] queues the rest of that list after it. Playback state,
// including the queue, is saved to localStorage, so a reload resumes the
// same track at the same position.
(() => {
	const STORAGE_KEY = "audioloader.player";

	const root = document.getElementById("player");
	const audio = document.getElementById("player-audio");
	const cover = document.getElementById("player-cover");
	const title = document.getElementById("player-title");
	const artists = document.getElementById("player-artists");
	const shuffleBtn = document.getElementById("player-shuffle");
	const queueList = document.getElementById("player-queue-list");
	const queueEmpty = document.getElementById("player-queue-empty");

	let current = null; // {id, title, artists, img, src}
	// queue holds tracks in list order; order is the play order as indexes
	// into queue (shuffled or not), and pos is where in order we are
	let queue = [];
	let order = [];
	let pos = 0;
	let shuffle = false;
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
				JSON.stringify({ track: current, time: audio.currentTime, volume: audio.volume, paused: audio.paused, queue, order, pos, shuffle }),
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
		renderQueue();
		// when restoring, saving now would overwrite the stored position with 0
		// before loadedmetadata has applied it
		if (!restoring) save();
	}

	function clear() {
		current = null;
		queue = [];
		order = [];
		pos = 0;
		root.hidden = true;
		audio.removeAttribute("src");
		audio.load();
		markRows();
		renderQueue();
		save();
	}

	// shuffled returns the indexes in random order (Fisher–Yates)
	function shuffled(indexes) {
		const a = indexes.slice();
		for (let i = a.length - 1; i > 0; i--) {
			const j = Math.floor(Math.random() * (i + 1));
			[a[i], a[j]] = [a[j], a[i]];
		}
		return a;
	}

	const identity = (n) => Array.from({ length: n }, (_, i) => i);

	// playList replaces the queue with tracks and plays the one at start
	// (a random one when shuffling without a start).
	function playList(tracks, start = 0) {
		if (!tracks.length) return;
		queue = tracks.slice();
		if (shuffle) {
			const rest = identity(queue.length).filter((i) => i !== start);
			order = [start, ...shuffled(rest)];
			pos = 0;
		} else {
			order = identity(queue.length);
			pos = start;
		}
		load(queue[order[pos]]);
	}

	function enqueue(track) {
		if (!current) return playList([track]);
		queue.push(track);
		order.push(queue.length - 1);
		renderQueue();
		save();
	}

	function playNext(track) {
		if (!current) return playList([track]);
		queue.push(track);
		order.splice(pos + 1, 0, queue.length - 1);
		renderQueue();
		save();
	}

	function next() {
		if (pos + 1 >= order.length) return false;
		pos++;
		load(queue[order[pos]]);
		return true;
	}

	function prev() {
		// like most players: back restarts the track unless it just started
		if (audio.currentTime > 3 || pos === 0) {
			audio.currentTime = 0;
			return;
		}
		pos--;
		load(queue[order[pos]]);
	}

	function toggleShuffle() {
		shuffle = !shuffle;
		if (order.length) {
			const playing = order[pos];
			if (shuffle) {
				// only what hasn't played yet gets shuffled
				const upcoming = order.slice(pos + 1);
				order = [...order.slice(0, pos + 1), ...shuffled(upcoming)];
			} else {
				order = identity(queue.length);
				pos = playing;
			}
		}
		shuffleBtn.setAttribute("aria-pressed", String(shuffle));
		renderQueue();
		save();
	}

	// removeUpcoming drops the track at play position p from the queue
	function removeUpcoming(p) {
		if (p <= pos) return;
		order.splice(p, 1);
		renderQueue();
		save();
	}

	function renderQueue() {
		queueList.replaceChildren();
		const upcoming = order.slice(pos + 1);
		queueEmpty.hidden = upcoming.length > 0;
		upcoming.forEach((qi, k) => {
			const t = queue[qi];
			const li = document.createElement("li");
			li.className = "flex items-center gap-2 py-1";
			const name = document.createElement("span");
			name.className = "min-w-0 flex-1 truncate";
			name.textContent = t.title;
			name.title = `${t.title} · ${t.artists}`;
			const rm = document.createElement("button");
			rm.type = "button";
			rm.className = "text-xs text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100";
			rm.textContent = "✕";
			rm.setAttribute("aria-label", `Remove ${t.title} from the queue`);
			rm.addEventListener("click", () => removeUpcoming(pos + 1 + k));
			li.append(name, rm);
			queueList.append(li);
		});
		document.getElementById("player-prev").disabled = !current;
		document.getElementById("player-next").disabled = pos + 1 >= order.length;
	}

	function markRows() {
		const playingID = current && !audio.paused ? current.id : null;
		document.querySelectorAll("[data-play-track]").forEach((btn) => {
			const playing = btn.dataset.id === playingID;
			btn.setAttribute("aria-pressed", String(playing));
			btn.textContent = playing ? "❚❚" : "▶";
		});
	}

	function trackOf(btn) {
		const d = btn.dataset;
		return { id: d.id, title: d.title, artists: d.artists, img: d.img, src: d.src };
	}

	// tracksIn returns the playable tracks of a list, once each, in order
	function tracksIn(list) {
		const seen = new Set();
		return [...list.querySelectorAll("[data-play-track]")].map(trackOf).filter((t) => !seen.has(t.id) && seen.add(t.id));
	}

	document.addEventListener("click", (e) => {
		const btn = e.target.closest("[data-play-track]");
		if (btn) {
			if (current?.id === btn.dataset.id) {
				if (audio.paused) audio.play().catch(() => {});
				else audio.pause();
				return;
			}
			const list = btn.closest("[data-track-list]");
			if (!list) return playList([trackOf(btn)]);
			const tracks = tracksIn(list);
			playList(tracks, Math.max(0, tracks.findIndex((t) => t.id === btn.dataset.id)));
			return;
		}

		const listBtn = e.target.closest("[data-play-list]");
		if (listBtn) {
			const list = document.getElementById(listBtn.dataset.playList);
			const tracks = list ? tracksIn(list) : [];
			if (!tracks.length) return;
			if ("shuffle" in listBtn.dataset && !shuffle) toggleShuffle();
			if (!("shuffle" in listBtn.dataset) && shuffle) toggleShuffle();
			playList(tracks, shuffle ? Math.floor(Math.random() * tracks.length) : 0);
			return;
		}

		const queueBtn = e.target.closest("[data-queue]");
		if (queueBtn) {
			const play = queueBtn.closest("[data-track-row]")?.querySelector("[data-play-track]");
			if (!play) return;
			if (queueBtn.dataset.queue === "next") playNext(trackOf(play));
			else enqueue(trackOf(play));
			queueBtn.closest("details")?.removeAttribute("open");
		}
	});

	document.getElementById("player-prev").addEventListener("click", prev);
	document.getElementById("player-next").addEventListener("click", next);
	shuffleBtn.addEventListener("click", toggleShuffle);
	document.getElementById("player-queue-clear").addEventListener("click", () => {
		// keep the current track, drop what's after it
		order = order.slice(0, pos + 1);
		renderQueue();
		save();
	});

	audio.addEventListener("play", () => (markRows(), save()));
	audio.addEventListener("pause", () => (markRows(), save()));
	audio.addEventListener("ended", () => {
		if (!next()) (markRows(), save());
	});
	audio.addEventListener("volumechange", save);
	audio.addEventListener("timeupdate", () => {
		if (Date.now() - lastSave > 2000) save();
	});
	// the track may have been deleted since it was queued: skip it, and
	// don't keep a broken player around when nothing is left
	audio.addEventListener("error", () => {
		if (!current || !audio.getAttribute("src")) return;
		if (pos + 1 < order.length) {
			order.splice(pos, 1);
			load(queue[order[pos]]);
		} else {
			clear();
		}
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
		ms.setActionHandler("previoustrack", prev);
		ms.setActionHandler("nexttrack", next);
	}

	window.player = { playList, enqueue, playNext, next, prev, toggleShuffle, clear };

	const state = restoreState();
	if (state?.track) {
		if (typeof state.volume === "number") audio.volume = state.volume;
		if (Array.isArray(state.queue) && Array.isArray(state.order) && state.order.every((i) => state.queue[i])) {
			queue = state.queue;
			order = state.order;
			pos = Math.min(state.pos || 0, order.length - 1);
		} else {
			// saved before the queue existed
			queue = [state.track];
			order = [0];
			pos = 0;
		}
		shuffle = Boolean(state.shuffle);
		shuffleBtn.setAttribute("aria-pressed", String(shuffle));
		load(state.track, { time: state.time, autoplay: !state.paused, restoring: true });
	} else {
		renderQueue();
	}
})();
