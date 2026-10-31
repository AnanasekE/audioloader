// Sleep timer logic for player.js, without any DOM or audio. All times are
// milliseconds since the epoch and passed in as `now`, so it can be tested.
//
// A timer ends at a wall-clock time (pausing does not stop the countdown), or
// at the end of the current track. save() gives the plain object that goes
// into the player's saved state: {endsAt} or {endOfTrack: true}, or null.
window.sleepTimer = {
	FADE_MS: 10000,
	DEFAULT_MINUTES: 30,
	MINUTES: [15, 30, 45, 60, 75, 90, 105, 120],

	// create makes a timer, from a saved object (see playerState.sleepFrom) or off.
	create(saved) {
		let endsAt = null; // ms timestamp, when counting down
		let endOfTrack = false;
		if (saved && typeof saved.endsAt === "number") endsAt = saved.endsAt;
		else if (saved && saved.endOfTrack === true) endOfTrack = true;

		return {
			active: () => endsAt !== null || endOfTrack,
			endOfTrack: () => endOfTrack,
			start(minutes, now) {
				endsAt = now + minutes * 60000;
				endOfTrack = false;
			},
			startEndOfTrack() {
				endsAt = null;
				endOfTrack = true;
			},
			// extend adds minutes to a running countdown (or starts one)
			extend(minutes, now) {
				if (endsAt === null) return this.start(minutes, now);
				endsAt += minutes * 60000;
			},
			cancel() {
				endsAt = null;
				endOfTrack = false;
			},
			// remaining is the ms left, or null when off or waiting for the track to end
			remaining: (now) => (endsAt === null ? null : Math.max(0, endsAt - now)),
			// minutesLeft is what the button shows: rounded up, so "1m" lasts until the end
			minutesLeft: (now) => (endsAt === null ? null : Math.ceil(Math.max(0, endsAt - now) / 60000)),
			shouldFade: (now) => endsAt !== null && endsAt - now <= window.sleepTimer.FADE_MS && endsAt - now > 0,
			// fadeFactor goes from 1 to 0 over the last FADE_MS
			fadeFactor: (now) => (endsAt === null ? 1 : Math.min(1, Math.max(0, (endsAt - now) / window.sleepTimer.FADE_MS))),
			shouldStop: (now) => endsAt !== null && endsAt <= now,
			save: () => (endsAt !== null ? { endsAt } : endOfTrack ? { endOfTrack: true } : null),
		};
	},
};
