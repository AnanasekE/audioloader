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
