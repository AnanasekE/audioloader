// Global player (player in internal/components/layout.templ). Track rows hand
// their track to it via [data-play-track] buttons; playing one inside a
// [data-track-list] queues the rest of that list after it. Playback state,
// including the queue, is saved to localStorage, so a reload resumes the
// same track at the same position.
//
// Podcast episodes (data-kind="episode") also save their position on the
// server, so they resume where they stopped on any device, and get skip
// buttons, a speed menu (remembered per show) and chapters.
(() => {
	const STORAGE_KEY = "audioloader.player";
	// the mode of this page; the saved state carries the mode it was saved in
	const MODE = document.body.dataset.mode === "secret" ? "secret" : "normal";

	const root = document.getElementById("player");
	const audio = document.getElementById("player-audio");
	const cover = document.getElementById("player-cover");
	const title = document.getElementById("player-title");
	const artists = document.getElementById("player-artists");
	const shuffleBtn = document.getElementById("player-shuffle");
	const queueList = document.getElementById("player-queue-list");
	const queueEmpty = document.getElementById("player-queue-empty");
	const episodeControls = document.getElementById("player-episode-controls");
	const speed = document.getElementById("player-speed");
	const chapterLabel = document.getElementById("player-chapter");
	const chaptersBox = document.getElementById("player-chapters-box");
	const chapterList = document.getElementById("player-chapters");

	let current = null; // {id, title, artists, img, src, kind?, show?, position?}
	let chapters = []; // [{start (s), title}] of the current episode
	let lastPositionSave = 0;
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
				JSON.stringify({ mode: MODE, track: current, time: audio.currentTime, volume: audio.volume, paused: audio.paused, queue, order, pos, shuffle }),
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
		artists.textContent = track.coverOf ? `${track.artists} · cover of ${track.coverOf}` : track.artists;
		if ("mediaSession" in navigator) {
			navigator.mediaSession.metadata = new MediaMetadata({
				title: track.title,
				artist: track.artists,
				artwork: [{ src: new URL(track.img, location.href).href }],
			});
		}
	}

	const isEpisode = (t) => t?.kind === "episode";

	function load(track, { time = 0, autoplay = true, restoring = false } = {}) {
		if (current && current !== track) savePosition();
		show(track);
		audio.src = track.src;
		// an episode resumes where it stopped, as saved on the server
		if (!restoring && isEpisode(track) && track.position) time = Number(track.position);
		setupEpisode(track);
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
		const t = { id: d.id, title: d.title, artists: d.artists, img: d.img, src: d.src };
		if (d.coverOf) t.coverOf = d.coverOf;
		if (d.secret === "1") t.secret = true;
		if (d.kind === "episode") Object.assign(t, { kind: "episode", show: d.show, position: d.position || 0 });
		return t;
	}

	// --- episodes -----------------------------------------------------------

	const speedKey = (t) => `audioloader.speed.${t.show || "none"}`;

	function setupEpisode(track) {
		const episode = isEpisode(track);
		episodeControls.hidden = !episode;
		chapters = [];
		renderChapters();
		let rate = 1;
		if (episode) {
			try {
				rate = Number(localStorage.getItem(speedKey(track))) || 1;
			} catch {}
			loadChapters(track);
		}
		speed.value = String(rate);
		audio.playbackRate = rate;
		audio.preservesPitch = true;
	}

	async function loadChapters(track) {
		try {
			const res = await fetch(`/api/tracks/${track.id}/chapters`);
			if (!res.ok || current !== track) return;
			chapters = await res.json();
			renderChapters();
		} catch {
			// no chapters is fine
		}
	}

	function renderChapters() {
		chapterList.replaceChildren();
		chaptersBox.hidden = chapters.length === 0;
		chapterLabel.hidden = chapters.length === 0;
		chapters.forEach((c) => {
			const li = document.createElement("li");
			const b = document.createElement("button");
			b.type = "button";
			b.className = "w-full truncate rounded px-1 py-0.5 text-left hover:bg-zinc-100 dark:hover:bg-zinc-700";
			b.textContent = c.title || "Chapter";
			b.addEventListener("click", () => {
				audio.currentTime = c.start;
				audio.play().catch(() => {});
			});
			li.append(b);
			chapterList.append(li);
		});
		updateChapter();
	}

	function updateChapter() {
		if (!chapters.length) return;
		let i = 0;
		while (i + 1 < chapters.length && chapters[i + 1].start <= audio.currentTime) i++;
		chapterLabel.textContent = chapters[i].title;
		chapterList.querySelectorAll("button").forEach((b, j) => b.classList.toggle("font-semibold", j === i));
	}

	// savePosition sends an episode's position to the server. Within 30 s of
	// the end it counts as finished. beacon is for page unload, where fetch
	// may be cancelled.
	function savePosition({ beacon = false, ended = false } = {}) {
		if (!isEpisode(current) || !audio.duration) return;
		lastPositionSave = Date.now();
		const finished = ended || audio.duration - audio.currentTime < 30;
		const seconds = finished ? 0 : audio.currentTime;
		current.position = seconds;
		const url = `/api/tracks/${current.id}/position`;
		const body = JSON.stringify({ seconds, finished });
		if (beacon && navigator.sendBeacon) navigator.sendBeacon(url, body);
		else fetch(url, { method: "POST", body, keepalive: true }).catch(() => {});
	}

	document.getElementById("player-back").addEventListener("click", () => (audio.currentTime = Math.max(0, audio.currentTime - 15)));
	document.getElementById("player-forward").addEventListener("click", () => (audio.currentTime += 30));
	speed.addEventListener("change", () => {
		audio.playbackRate = Number(speed.value);
		try {
			localStorage.setItem(speedKey(current), speed.value);
		} catch {}
	});

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
	audio.addEventListener("pause", () => (markRows(), save(), audio.ended || savePosition()));
	audio.addEventListener("ended", () => {
		savePosition({ ended: true });
		if (!next()) (markRows(), save());
	});
	audio.addEventListener("volumechange", save);
	audio.addEventListener("timeupdate", () => {
		if (Date.now() - lastSave > 2000) save();
		if (Date.now() - lastPositionSave > 10000 && !audio.paused) savePosition();
		updateChapter();
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
	window.addEventListener("pagehide", () => (save(), savePosition({ beacon: true })));

	// boosted navigation swaps in new track rows; show which one is playing
	document.addEventListener("htmx:afterSettle", markRows);

	if ("mediaSession" in navigator) {
		const ms = navigator.mediaSession;
		ms.setActionHandler("play", () => audio.play());
		ms.setActionHandler("pause", () => audio.pause());
		ms.setActionHandler("seekto", (e) => (audio.currentTime = e.seekTime));
		ms.setActionHandler("seekbackward", (e) => (audio.currentTime = Math.max(0, audio.currentTime - (e.seekOffset || 15))));
		ms.setActionHandler("seekforward", (e) => (audio.currentTime += e.seekOffset || 30));
		ms.setActionHandler("previoustrack", prev);
		ms.setActionHandler("nexttrack", next);
	}

	// reset forgets everything: what plays, the queue, the stored state and the
	// lock screen metadata. Locking and unlocking call it as the form is sent,
	// so nothing of one mode is left for the other.
	function reset() {
		audio.pause();
		clear();
		try {
			localStorage.removeItem(STORAGE_KEY);
		} catch {}
		if ("mediaSession" in navigator) navigator.mediaSession.metadata = null;
	}

	window.player = { playList, enqueue, playNext, next, prev, toggleShuffle, clear, reset };

	// a page of one mode must not resume what the other mode played: not
	// playing, not queued, not in storage
	const saved = restoreState();
	const state = window.playerState.forMode(saved, MODE);
	if (saved && !state) {
		try {
			localStorage.removeItem(STORAGE_KEY);
		} catch {}
	} else if (state) {
		// write the cleaned state back now, not at the next save
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
		} catch {}
	}
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
