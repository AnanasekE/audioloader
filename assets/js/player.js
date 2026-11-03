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
	const playBtn = document.getElementById("player-play");
	const seek = document.getElementById("player-seek");
	const timeEl = document.getElementById("player-time");
	const remainingEl = document.getElementById("player-remaining");
	const shuffleBtn = document.getElementById("player-shuffle");
	const episodeControls = document.getElementById("player-episode-controls");
	const speed = document.getElementById("player-speed");
	const chapterLabel = document.getElementById("player-chapter");
	// the queue shows in the popover and in the side panel (layout.templ); both
	// are filled from the same state, found by these attributes
	const all = (attr) => document.querySelectorAll(`[${attr}]`);

	let current = null; // {id, title, artists, img, src, kind?, show?, position?}
	let chapters = []; // [{start (s), title}] of the current episode
	let lastPositionSave = 0;
	// queue holds tracks in list order; order is the play order as indexes
	// into queue (shuffled or not), and pos is where in order we are
	let queue = [];
	let order = [];
	let pos = 0;
	let shuffle = false;
	let repeat = "off"; // "off", "all" or "one"
	// how many tracks "Play next" has put after the current one since it started
	let nextAt = 0;
	// sleep timer (sleep-timer.js); fadeBase is the volume to go back to while a fade runs
	let timer = window.sleepTimer.create(null);
	let fadeBase = null;
	let fadeInterval = null;
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
				JSON.stringify({ mode: MODE, track: current, time: audio.currentTime, volume: fadeBase ?? audio.volume, muted: audio.muted, paused: audio.paused, queue, order, pos, shuffle, repeat, sleep: timer.save() }),
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
		renderNow();
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
		nextAt = 0;
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
		syncTime();
		queue = [];
		order = [];
		pos = 0;
		root.hidden = true;
		audio.removeAttribute("src");
		audio.load();
		markRows();
		renderNow();
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

	// enqueue and playNext take a track or an array of them
	function enqueue(tracks) {
		tracks = [].concat(tracks);
		if (!tracks.length) return;
		if (!current) return playList(tracks);
		order.push(...tracks.map((t) => queue.push(t) - 1));
		renderQueue();
		save();
	}

	function playNext(tracks) {
		tracks = [].concat(tracks);
		if (!tracks.length) return;
		if (!current) return playList(tracks);
		const at = window.playerState.playNextSlot(pos, nextAt, order.length);
		order.splice(at, 0, ...tracks.map((t) => queue.push(t) - 1));
		nextAt += tracks.length;
		renderQueue();
		save();
	}

	function next() {
		const p = window.playerState.nextPos(pos, order.length, repeat);
		if (p < 0) return false;
		if (p === 0 && pos > 0 && shuffle) {
			// a new pass: reshuffled, but not starting with the track that just played
			order = window.playerState.keepOutOfFirst(shuffled(order), order[pos]);
		}
		pos = p;
		load(queue[order[pos]]);
		return true;
	}

	// toggleRepeat steps off -> all -> one -> off
	function toggleRepeat() {
		repeat = { off: "all", all: "one", one: "off" }[repeat];
		syncRepeat();
		save();
	}

	function syncRepeat() {
		const label = `Repeat: ${repeat}`;
		all("data-repeat").forEach((b) => {
			b.setAttribute("aria-pressed", String(repeat !== "off"));
			b.setAttribute("aria-label", label);
			b.querySelector("[data-repeat-icon=one]").hidden = repeat !== "one";
			b.querySelector("[data-repeat-icon=all]").hidden = repeat === "one";
		});
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
		if (p <= pos + nextAt) nextAt--;
		renderQueue();
		save();
	}

	// playAt jumps to the track at play position p, after the current one:
	// those in between stay behind in the history
	function playAt(p) {
		if (p <= pos || p >= order.length) return;
		pos = p;
		load(queue[order[pos]]);
	}

	function renderQueue() {
		const upcoming = order.slice(pos + 1);
		all("data-queue-empty").forEach((el) => (el.hidden = upcoming.length > 0));
		all("data-queue-list").forEach((list) => {
			list.replaceChildren();
			upcoming.forEach((qi, k) => {
				const t = queue[qi];
				const li = document.createElement("li");
				li.className = "flex items-center gap-2";
				const name = document.createElement("button");
				name.type = "button";
				name.className = "min-w-0 flex-1 truncate py-1 text-left hover:text-accent";
				name.textContent = t.title;
				name.title = `${t.title} · ${t.artists}`;
				name.addEventListener("click", () => playAt(pos + 1 + k));
				const rm = document.createElement("button");
				rm.type = "button";
				rm.className = "text-xs text-fg-muted hover:text-fg";
				rm.append(document.getElementById("player-x-icon").content.cloneNode(true));
				rm.setAttribute("aria-label", `Remove ${t.title} from the queue`);
				rm.addEventListener("click", () => removeUpcoming(pos + 1 + k));
				li.append(name, rm);
				list.append(li);
			});
		});
		document.getElementById("player-prev").disabled = !current;
		document.getElementById("player-next").disabled = window.playerState.nextPos(pos, order.length, repeat) < 0;
	}

	// renderNow fills the "now playing" part of the side panel
	function renderNow() {
		all("data-now-empty").forEach((el) => (el.hidden = Boolean(current)));
		all("data-now").forEach((el) => (el.hidden = !current));
		if (!current) return;
		all("data-now-cover").forEach((el) => (el.src = current.img));
		all("data-now-title").forEach((el) => (el.textContent = current.title));
		all("data-now-artists").forEach((el) => (el.textContent = current.coverOf ? `${current.artists} · cover of ${current.coverOf}` : current.artists));
	}

	// setPlayIcon shows the play or the pause icon of a button (both are in its markup)
	function setPlayIcon(btn, playing) {
		btn.querySelector("[data-icon-play]").hidden = playing;
		btn.querySelector("[data-icon-pause]").hidden = !playing;
	}

	function markRows() {
		const playingID = current && !audio.paused ? current.id : null;
		document.querySelectorAll("[data-play-track]").forEach((btn) => {
			const playing = btn.dataset.id === playingID;
			btn.setAttribute("aria-pressed", String(playing));
			// data-bare buttons draw their own state from aria-pressed (CSS)
			if (!btn.hasAttribute("data-bare")) setPlayIcon(btn, playing);
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
		all("data-sleep-track").forEach((b) => (b.textContent = episode ? "End of this episode" : "End of this track"));
		syncSleep();
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
		all("data-chapter-box").forEach((el) => (el.hidden = chapters.length === 0));
		chapterLabel.hidden = chapters.length === 0;
		all("data-chapter-list").forEach((list) => {
			list.replaceChildren();
			chapters.forEach((c) => {
				const li = document.createElement("li");
				const b = document.createElement("button");
				b.type = "button";
				b.className = "w-full truncate rounded px-1 py-0.5 text-left hover:bg-panel-2";
				b.textContent = c.title || "Chapter";
				b.addEventListener("click", () => {
					audio.currentTime = c.start;
					audio.play().catch(() => {});
				});
				li.append(b);
				list.append(li);
			});
		});
		updateChapter();
	}

	function updateChapter() {
		if (!chapters.length) return;
		let i = 0;
		while (i + 1 < chapters.length && chapters[i + 1].start <= audio.currentTime) i++;
		chapterLabel.textContent = chapters[i].title;
		all("data-chapter-list").forEach((list) => list.querySelectorAll("button").forEach((b, j) => b.classList.toggle("font-semibold", j === i)));
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

	// toast shows a line above the player for a moment
	let toastTimer;
	function toast(text) {
		const el = document.querySelector("[data-queue-toast]");
		if (!el) return;
		el.textContent = text;
		clearTimeout(toastTimer);
		toastTimer = setTimeout(() => (el.textContent = ""), 3000);
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

		const allBtn = e.target.closest("[data-queue-all]");
		if (allBtn) {
			const menu = allBtn.closest("[data-queue-tracks]");
			const list = document.getElementById(menu.dataset.queueTracks);
			const tracks = list ? tracksIn(list) : [];
			menu.removeAttribute("open");
			if (!tracks.length) return;
			if (allBtn.dataset.queueAll === "next") playNext(tracks);
			else enqueue(tracks);
			toast(`Added ${tracks.length} track${tracks.length === 1 ? "" : "s"} to the queue`);
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
	all("data-repeat").forEach((b) => b.addEventListener("click", () => (toggleRepeat(), renderQueue())));
	all("data-queue-clear").forEach((btn) =>
		btn.addEventListener("click", () => {
			// keep the current track, drop what's after it
			order = order.slice(0, pos + 1);
			nextAt = 0;
			renderQueue();
			save();
		}),
	);

	// --- play button, seek bar and times ---------------------------------

	function clock(sec) {
		sec = Math.max(0, Math.floor(sec || 0));
		const h = Math.floor(sec / 3600);
		const m = Math.floor((sec % 3600) / 60);
		const s = String(sec % 60).padStart(2, "0");
		return h ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`;
	}

	// --- sleep timer -------------------------------------------------------

	function syncSleep() {
		const now = Date.now();
		const on = timer.active();
		const mins = timer.minutesLeft(now);
		const text = timer.endOfTrack() ? "End" : mins === null ? "" : `${mins}m`;
		const status = !on ? "Off" : timer.endOfTrack() ? `Pauses when this ${isEpisode(current) ? "episode" : "track"} ends` : `Pauses in ${mins} min`;
		const aria = on ? `Sleep timer: ${timer.endOfTrack() ? "stops at the end of the track" : `${mins} minutes left`}` : "Sleep timer";
		// the button is drawn once per spot (layout.templ); keep every copy the same
		all("data-sleep-label").forEach((el) => ((el.hidden = !on), (el.textContent = text)));
		all("data-sleep-summary").forEach((el) => {
			el.classList.toggle("text-accent", on);
			el.classList.toggle("text-fg-muted", !on);
			el.setAttribute("aria-label", aria);
		});
		all("data-sleep-status").forEach((el) => (el.textContent = status));
		all("data-sleep-extra").forEach((el) => {
			el.hidden = !on;
			el.querySelector('[data-sleep="extend"]').hidden = timer.endOfTrack();
		});
	}

	// endFade puts the volume back to where it was
	function endFade() {
		clearInterval(fadeInterval);
		fadeInterval = null;
		if (fadeBase === null) return;
		const base = fadeBase;
		fadeBase = null;
		audio.volume = base;
	}

	function sleepTick() {
		const now = Date.now();
		if (timer.shouldStop(now)) {
			// pause as any pause does (position saved), then the volume comes back
			audio.pause();
			endFade();
			timer.cancel();
			save();
		} else if (timer.shouldFade(now) && !audio.paused && fadeBase === null) {
			fadeBase = audio.volume;
			fadeInterval = setInterval(() => {
				if (fadeBase !== null) audio.volume = fadeBase * timer.fadeFactor(Date.now());
			}, 100);
		}
		syncSleep();
	}

	// sleepChanged runs after the user changed the timer
	function sleepChanged() {
		if (!timer.shouldFade(Date.now())) endFade();
		save();
		syncSleep();
	}

	all("data-sleep-box").forEach((box) => {
		box.querySelector("summary").addEventListener("click", () => {
			// the first click with no timer starts the default one; the menu opens as usual
			if (!timer.active() && !box.open) {
				timer.start(window.sleepTimer.DEFAULT_MINUTES, Date.now());
				sleepChanged();
			}
		});
		box.addEventListener("click", (e) => {
			const b = e.target.closest("[data-sleep]");
			if (!b) return;
			const what = b.dataset.sleep;
			if (what === "off") {
				timer.cancel();
				box.open = false;
			} else if (what === "extend") timer.extend(15, Date.now());
			else if (what === "track") timer.startEndOfTrack();
			else timer.start(Number(what), Date.now());
			sleepChanged();
		});
	});
	setInterval(sleepTick, 1000);

	// --- volume and mute ---------------------------------------------------

	const volumeEl = document.getElementById("player-volume");
	const muteBtn = document.getElementById("player-mute");
	const { volumeFromSlider, sliderFromVolume } = window.playerState;

	function syncVolume() {
		const silent = audio.muted || audio.volume === 0;
		volumeEl.value = String(audio.muted ? 0 : sliderFromVolume(audio.volume));
		muteBtn.setAttribute("aria-label", silent ? "Unmute" : "Mute");
		const level = silent ? "x" : audio.volume < 0.5 ? "1" : "2";
		muteBtn.querySelectorAll("[data-vol-icon]").forEach((el) => (el.hidden = el.dataset.volIcon !== level));
	}

	function toggleMute() {
		// at volume 0 "unmute" has to raise the volume too, or nothing would change
		if (!audio.muted && audio.volume === 0) audio.volume = 0.25;
		else audio.muted = !audio.muted;
		syncVolume();
	}

	function setVolume(vol) {
		// during a sleep fade the new level is what the volume returns to
		if (fadeBase !== null) {
			fadeBase = Math.min(1, Math.max(0, vol));
			return;
		}
		audio.volume = Math.min(1, Math.max(0, vol));
		if (audio.volume > 0) audio.muted = false;
	}

	volumeEl.addEventListener("input", () => setVolume(volumeFromSlider(volumeEl.value)));
	muteBtn.addEventListener("click", toggleMute);
	// m toggles mute; arrow keys stay free so the page scrolls
	document.addEventListener("keydown", (e) => {
		if (!current || e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey) return;
		const t = e.target;
		if (t instanceof Element && t.closest("input, textarea, select, [contenteditable]")) return;
		if (e.key === "m") {
			e.preventDefault();
			toggleMute();
		}
	});

	let seeking = false;

	function syncTime() {
		const dur = Number.isFinite(audio.duration) ? audio.duration : 0;
		seek.max = String(dur);
		if (!seeking) seek.value = String(audio.currentTime);
		timeEl.textContent = clock(seeking ? Number(seek.value) : audio.currentTime);
		remainingEl.textContent = "-" + clock(dur - (seeking ? Number(seek.value) : audio.currentTime));
	}

	function syncPlay() {
		const playing = !audio.paused;
		setPlayIcon(playBtn, playing);
		playBtn.setAttribute("aria-label", playing ? "Pause" : "Play");
	}

	function togglePlay() {
		if (!current) return;
		if (audio.paused) audio.play().catch(() => {});
		else audio.pause();
	}

	playBtn.addEventListener("click", togglePlay);
	seek.addEventListener("input", () => {
		seeking = true;
		audio.currentTime = Number(seek.value);
		syncTime();
		seeking = false;
	});
	audio.addEventListener("loadedmetadata", syncTime);
	audio.addEventListener("durationchange", syncTime);
	audio.addEventListener("timeupdate", syncTime);
	audio.addEventListener("play", syncPlay);
	audio.addEventListener("pause", syncPlay);
	document.addEventListener("keydown", (e) => {
		if (e.code !== "Space" || e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey) return;
		const t = e.target;
		if (t instanceof Element && t.closest("input, textarea, select, button, summary, a, [contenteditable]")) return;
		if (!current) return;
		e.preventDefault();
		togglePlay();
	});

	audio.addEventListener("play", () => (markRows(), save()));
	audio.addEventListener("pause", () => (markRows(), save(), audio.ended || savePosition()));
	audio.addEventListener("ended", () => {
		savePosition({ ended: true });
		// "end of this track": stop here instead of moving on
		if (timer.endOfTrack()) {
			timer.cancel();
			syncSleep();
			return (markRows(), save());
		}
		// repeat one: the same track again (Next still moves on)
		if (repeat === "one") {
			audio.currentTime = 0;
			audio.play().catch(() => {});
			return;
		}
		if (!next()) (markRows(), save());
	});
	audio.addEventListener("volumechange", () => fadeBase === null && (syncVolume(), save()));
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
		timer.cancel();
		endFade();
		syncSleep();
		audio.pause();
		clear();
		try {
			localStorage.removeItem(STORAGE_KEY);
		} catch {}
		if ("mediaSession" in navigator) navigator.mediaSession.metadata = null;
	}

	window.player = { playList, enqueue, playNext, playAt, next, prev, toggleShuffle, clear, reset };

	syncVolume();
	syncSleep();
	syncRepeat();

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
		// a timer that ended while the page was closed is dropped
		timer = window.sleepTimer.create(window.playerState.sleepFrom(state, Date.now()));
		const av = window.playerState.audioSettings(state);
		if (av.volume !== null) audio.volume = av.volume;
		if (av.muted !== null) audio.muted = av.muted;
		syncVolume();
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
		repeat = window.playerState.repeatFrom(state);
		syncRepeat();
		load(state.track, { time: state.time, autoplay: !state.paused, restoring: true });
	} else {
		renderQueue();
	}
})();
