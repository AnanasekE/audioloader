// Which saved player state a page may resume (used by player.js). The state
// belongs to the mode it was saved in, normal or secret, and the tracks in it
// carry a `secret` flag: a page never resumes a track of the other mode.
window.playerState = {
	// The slider is 0-1 and the audio volume is its square, so the quiet end
	// of the slider isn't all crammed together.
	volumeFromSlider: (v) => Math.min(1, Math.max(0, Number(v) || 0)) ** 2,
	sliderFromVolume: (vol) => Math.sqrt(Math.min(1, Math.max(0, Number(vol) || 0))),

	// sleepFrom returns the sleep timer to restore from a saved state: {endsAt}
	// or {endOfTrack: true}, or null. A timer that ended while the page was
	// closed, or a malformed one, is dropped.
	sleepFrom(state, now) {
		const s = state?.sleep;
		if (!s || typeof s !== "object") return null;
		if (typeof s.endsAt === "number" && Number.isFinite(s.endsAt)) return s.endsAt > now ? { endsAt: s.endsAt } : null;
		return s.endOfTrack === true ? { endOfTrack: true } : null;
	},

	// repeatFrom returns the repeat mode to restore from a saved state:
	// "off", "all" or "one"; anything else is "off".
	repeatFrom(state) {
		return state?.repeat === "all" || state?.repeat === "one" ? state.repeat : "off";
	},

	// nextPos returns the play position after pos in a queue of length, or -1
	// at the end. Only "all" wraps to the start ("one" is handled when a track
	// ends; Next still moves on).
	nextPos(pos, length, repeat) {
		if (pos + 1 < length) return pos + 1;
		return repeat === "all" && length > 0 ? 0 : -1;
	},

	// playNextSlot returns the play position where "Play next" inserts: right
	// after the current track and after the nextAt tracks already put there
	// since it started, so several in a row play in the order they were added.
	// It never goes past the end of the order.
	playNextSlot(pos, nextAt, length) {
		return Math.min(pos + 1 + Math.max(0, nextAt), length);
	},

	HISTORY_MAX: 50,
	AUTOPLAY_SEEDS: 5,

	// autoplayFrom returns whether autoplay is on in a saved state; it is on
	// unless the state says false.
	autoplayFrom(state) {
		return state?.autoplay !== false;
	},

	// historyFrom returns the recently played track IDs of a saved state (at
	// most HISTORY_MAX, oldest first); anything that isn't a list of strings gives [].
	historyFrom(state) {
		const h = state?.history;
		if (!Array.isArray(h) || !h.every((id) => typeof id === "string")) return [];
		return h.slice(-window.playerState.HISTORY_MAX);
	},

	// pushHistory returns history with id as the most recent; an earlier
	// play of the same track moves up instead of repeating.
	pushHistory(history, id) {
		return [...history.filter((h) => h !== id), id].slice(-window.playerState.HISTORY_MAX);
	},

	// autoplayURL is the request for suggestions: the last few played tracks
	// are the seeds, the newest first, and all of the history is excluded.
	autoplayURL(history, n) {
		const q = new URLSearchParams();
		history.slice(-window.playerState.AUTOPLAY_SEEDS).reverse().forEach((id) => q.append("seed", id));
		history.forEach((id) => q.append("exclude", id));
		q.set("n", String(n));
		return `/api/v1/autoplay?${q}`;
	},

	// trackFromAPI turns a track of the JSON API into a queue entry. Autoplay
	// entries carry auto: true; a track of secret mode carries secret: true.
	trackFromAPI(j, secret) {
		const t = {
			id: j.id,
			title: j.title,
			artists: (j.artists || []).map((a) => a.name).join(", "),
			img: j.imageUrl || (secret ? "/assets/img/default_secret.png" : "/assets/img/default_track.png"),
			src: j.streamUrl,
			auto: true,
		};
		if (secret) t.secret = true;
		return t;
	},

	// autoStart returns the play position of the first autoplay track after
	// pos, or the length of order when there is none: what the user adds goes
	// in before it.
	autoStart(order, queue, pos) {
		for (let p = pos + 1; p < order.length; p++) if (queue[order[p]]?.auto) return p;
		return order.length;
	},

	// autoLast returns order with the upcoming autoplay tracks moved behind
	// the others, each group keeping its order.
	autoLast(order, queue, pos) {
		const upcoming = order.slice(pos + 1);
		const auto = (i) => Boolean(queue[i]?.auto);
		return [...order.slice(0, pos + 1), ...upcoming.filter((i) => !auto(i)), ...upcoming.filter(auto)];
	},

	// keepOutOfFirst returns order with `last` (the track that just played
	// out) not in the first slot, by swapping it with the second one.
	keepOutOfFirst(order, last) {
		const a = order.slice();
		if (a.length > 1 && a[0] === last) [a[0], a[1]] = [a[1], a[0]];
		return a;
	},

	// audioSettings returns the volume and mute to restore from a saved state;
	// a missing or bad field gives null, meaning "leave as it is".
	audioSettings(state) {
		const ok = typeof state?.volume === "number" && state.volume >= 0 && state.volume <= 1;
		return { volume: ok ? state.volume : null, muted: typeof state?.muted === "boolean" ? state.muted : null };
	},

	// forMode returns the state to resume on a page of mode ("normal" or "secret"),
	// or null if nothing of it may be kept. The state is changed in place.
	forMode(state, mode) {
		if (!state || typeof state !== "object") return null;
		// saved in the other mode: all of it goes
		if (state.mode && state.mode !== mode) return null;
		const secret = mode === "secret";
		const keep = (t) => Boolean(t) && Boolean(t.secret) === secret;
		if (Array.isArray(state.queue) && Array.isArray(state.order)) {
			// rebuilt from the kept tracks in play order, so no entry of the other mode stays in the queue
			const current = state.order[state.pos];
			const kept = state.order.filter((i) => keep(state.queue[i]));
			state.pos = Math.max(0, kept.indexOf(current));
			state.queue = kept.map((i) => state.queue[i]);
			state.order = state.queue.map((_, i) => i);
		}
		if (!keep(state.track)) {
			state.track = state.order?.length ? state.queue[state.order[state.pos]] : null;
			state.time = 0;
			state.paused = true;
		}
		state.mode = mode;
		return state.track ? state : null;
	},
};
