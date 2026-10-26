// Which saved player state a page may resume (used by player.js). The state
// belongs to the mode it was saved in, normal or secret, and the tracks in it
// carry a `secret` flag: a page never resumes a track of the other mode.
window.playerState = {
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
