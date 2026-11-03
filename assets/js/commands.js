// The actions of the search box (⌘K): commands that are listed with the
// search results and run on Enter or click. nav.js draws them. Each is
// {id, label, keywords, when(), run()}; `when` says whether it applies right
// now (which mode, which player state) and is called each time the list is
// built, `run` only when picked.
//
// parse and match are pure, for the tests: parse splits what was typed into
// "commands only" (a leading ">") and the text to match; match keeps the
// commands whose words begin with every word of the text.
(() => {
	const words = (s) => s.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter(Boolean);

	// parse reads the search box: ">sh" is {only: true, text: "sh"}
	function parse(raw) {
		const t = raw.trimStart();
		return t.startsWith(">") ? { only: true, text: t.slice(1).trim() } : { only: false, text: t.trim() };
	}

	// match returns the commands (in list order) that apply and whose label or
	// keywords have a word starting with each word of text. No text matches
	// nothing, unless all is set (the ">" alone, which lists them all).
	function match(cmds, text, all = false) {
		const q = words(text);
		if (!q.length && !all) return [];
		return cmds.filter((c) => {
			if (c.when && !c.when()) return false;
			const have = words(c.label + " " + (c.keywords || ""));
			return q.every((w) => have.some((h) => h.startsWith(w)));
		});
	}

	const secret = () => document.body.dataset.mode === "secret";
	const hasLink = (href) => Boolean(document.querySelector(`a[href="${href}"]`));
	// go follows a link of the page when there is one: hx-boost then swaps the
	// body and the player keeps playing
	const go = (href) => () => {
		const a = document.querySelector(`a[href="${href}"]`) ?? Object.assign(document.createElement("a"), { href });
		a.click();
	};
	const player = () => window.player?.state();
	const page = (id, label, href, keywords = "") => ({ id, label: "Go to " + label, keywords: "open page " + keywords, when: () => hasLink(href), run: go(href) });

	const list = [
		{ id: "add", label: "Add / upload", keywords: "upload song import new", run: go("/add") },
		page("go-tracks", "Tracks", "/tracks", "songs library"),
		page("go-releases", "Releases", "/releases", "albums eps singles"),
		{ id: "go-artists", label: "Go to Artists", keywords: "open page", when: () => !secret(), run: go("/artists") },
		{ id: "go-creators", label: "Go to Creators", keywords: "open page artists", when: () => secret(), run: go("/artists") },
		page("go-playlists", "Playlists", "/playlists"),
		page("go-podcasts", "Podcasts", "/shows", "shows episodes"),
		page("go-inbox", "Inbox", "/inbox", "new releases"),
		page("go-tagging", "Tagging", "/tagging", "review musicbrainz"),
		page("go-downloads", "Downloads", "/downloads", "queue"),
		page("go-tags", "Tags", "/tags"),
		{
			id: "check-follows",
			label: "Check followed artists now",
			keywords: "follow releases new tracking refresh",
			when: () => !secret(),
			run: () => window.htmx?.ajax("POST", "/ui/follows/check", { target: "body" }),
		},
		{ id: "shuffle-on", label: "Shuffle on", keywords: "random", when: () => player() && !player().shuffle, run: () => window.player.toggleShuffle() },
		{ id: "shuffle-off", label: "Shuffle off", keywords: "random", when: () => player()?.shuffle, run: () => window.player.toggleShuffle() },
		...["off", "all", "one"].map((mode) => ({
			id: "repeat-" + mode,
			label: "Repeat " + mode,
			keywords: "loop mode",
			when: () => player() && player().repeat !== mode,
			run: () => window.player.setRepeat(mode),
		})),
		{ id: "autoplay-on", label: "Autoplay on", keywords: "similar tracks", when: () => player() && !player().autoplay, run: () => window.player.toggleAutoplay() },
		{ id: "autoplay-off", label: "Autoplay off", keywords: "similar tracks", when: () => player()?.autoplay, run: () => window.player.toggleAutoplay() },
		{ id: "sleep-30", label: "Sleep in 30 minutes", keywords: "timer sleep stop", when: () => window.player, run: () => window.player.sleepIn(30) },
		{ id: "sleep-off", label: "Sleep timer off", keywords: "timer cancel", when: () => player()?.sleeping, run: () => window.player.sleepIn(null) },
		{ id: "theme-dark", label: "Dark theme", keywords: "appearance mode", run: () => window.setTheme?.("dark") },
		{ id: "theme-light", label: "Light theme", keywords: "appearance mode", run: () => window.setTheme?.("light") },
		{ id: "vault-open", label: "Open the vault", keywords: "secret unlock", when: () => !secret() && hasLink("/vault"), run: go("/vault") },
		{
			id: "vault-lock",
			label: "Lock the vault",
			keywords: "secret mode leave",
			when: () => secret(),
			run: () => document.querySelector('form[action="/vault/lock"]')?.requestSubmit(),
		},
		{ id: "shortcuts", label: "Keyboard shortcuts", keywords: "help keys", run: () => document.getElementById("shortcuts-dialog")?.showModal() },
	];

	window.commands = { parse, match, list };
})();
