// Library import (ImportPage in internal/components/import.templ): reads the
// tags of a whole folder in the browser, proposes releases, lets the user
// regroup and relabel, then creates the releases and uploads the tracks.
(() => {
	const root = document.getElementById("import");
	if (!root) return;

	const { trackFingerprint, readTags } = window; // track-hash.js, tags.js
	const $ = (id) => document.getElementById(id);
	const folderInput = $("import-folder");
	const filesInput = $("import-files");
	const creatorInput = $("import-creator");
	const progress = $("import-progress");
	const review = $("import-review");
	const groupsEl = $("import-groups");
	const singlesEl = $("import-singles");
	const summary = $("import-summary");
	const result = $("import-result");
	const moveSelect = $("import-move");
	const templates = {
		group: $("import-group-template"),
		row: $("import-row-template"),
		single: $("import-single-template"),
		artist: $("import-artist-template"),
		option: $("import-artist-option-template"),
	};
	const artistList = $("import-artist-list");
	const artistRows = $("import-artist-rows");
	const artistStatus = $("import-artist-status");

	// lower-cased name -> name, for artists already in the library (the server
	// renders them into the datalist) plus any added from this page
	const knownArtists = new Map([...artistList.options].map((o) => [o.value.toLowerCase(), o.value]));

	const AUDIO = /\.(mp3|opus|ogg|oga|flac|m4a|mp4|aac|wav)$/i;
	const RELEASE_DATE = /^\d{4}(-\d{2}(-\d{2})?)?$/;
	const UPLOADS_AT_ONCE = 3;

	const items = new Map(); // id -> {id, file, path, tags, fp, inLibrary, dupOf, groupID, values, el, done}
	let groups = []; // {id, key, title, type, typeSource, date, artists, itemIDs, el, releaseID}
	let nextID = 1;
	let busy = false;

	const norm = (s) => String(s || "").toLowerCase().replace(/\s+/g, " ").trim();
	const creator = () => creatorInput.value.trim();

	// "A, B; C feat. D" -> ["A", "B", "C", "D"]; "&" is left alone, it's often part of a band name
	function splitArtists(s) {
		const names = String(s || "")
			.split(/\s*[,;]\s*|\s+(?:feat\.?|ft\.?|featuring)\s+/i)
			.map((n) => n.trim())
			.filter(Boolean);
		return [...new Set(names)];
	}

	// Titles from video sites carry noise; tag titles from music stores don't, so this is mild.
	function cleanTitle(title, artists) {
		let t = title.replace(/\s*[([]\s*(official\s+)?(music\s+)?(video|audio|lyric video|lyrics?|visualizer|hd|hq)\s*[)\]]/gi, "");
		for (const a of artists) {
			const prefix = `${a} - `;
			if (a && t.toLowerCase().startsWith(prefix.toLowerCase())) t = t.slice(prefix.length);
		}
		return t.trim() || title.trim();
	}

	function titleFromFile(path) {
		const name = path.split("/").pop().replace(/\.[^.]+$/, "");
		return name.replace(/^\d{1,3}\s*[-._)]?\s+/, ""); // leading track number
	}

	function trackFromFile(path) {
		const m = path.split("/").pop().match(/^(\d{1,3})\s*[-._)]?\s+/);
		return m ? parseInt(m[1], 10) : 0;
	}

	// --- reading files -----------------------------------------------------

	async function addFiles(fileList) {
		const files = [...fileList].filter((f) => AUDIO.test(f.name));
		if (!files.length) {
			progress.textContent = "No audio files found there.";
			return;
		}
		busy = true;
		const added = [];
		let read = 0;
		progress.textContent = `Reading tags… 0 / ${files.length}`;

		// a few files at a time: fast, without opening thousands of reads at once
		await pool(files, 6, async (file) => {
			const path = file.webkitRelativePath || file.name;
			const [tags, fp] = await Promise.all([
				readTags(file),
				trackFingerprint?.supported() ? trackFingerprint.of(file).catch(() => null) : null,
			]);
			const item = { id: nextID++, file, path, tags, fp, inLibrary: false, dupOf: null, groupID: null, done: false };
			const tagArtists = tags.artists.flatMap(splitArtists);
			item.values = {
				title: cleanTitle(tags.title || titleFromFile(path), tagArtists.length ? tagArtists : [creator()]),
				artists: (tagArtists.length ? tagArtists : splitArtists(creator())).join(", "),
				disc: tags.disc || 1,
				date: tags.date,
			};
			items.set(item.id, item);
			added.push(item);
			progress.textContent = `Reading tags… ${++read} / ${files.length}`;
		});

		markDuplicates(added);
		await checkLibrary(added);
		groupItems(added);
		progress.textContent = "";
		busy = false;
		render();
		$("import-start").hidden = true;
		review.hidden = false;
	}

	async function pool(list, size, fn) {
		let i = 0;
		const worker = async () => {
			while (i < list.length) await fn(list[i++]);
		};
		await Promise.all(Array.from({ length: Math.min(size, list.length) }, worker));
	}

	function markDuplicates(added) {
		const seen = new Map();
		for (const item of items.values()) {
			if (!item.fp) continue;
			const key = `${item.fp.size}:${item.fp.hash}`;
			if (seen.has(key) && added.includes(item)) item.dupOf = seen.get(key);
			else if (!seen.has(key)) seen.set(key, item.id);
		}
	}

	async function checkLibrary(added) {
		const withFP = added.filter((it) => it.fp && !it.dupOf);
		for (let i = 0; i < withFP.length; i += 400) {
			const chunk = withFP.slice(i, i + 400);
			try {
				const exists = await trackFingerprint.exist(chunk.map((it) => it.fp));
				chunk.forEach((it, j) => (it.inLibrary = Boolean(exists[j])));
			} catch {
				// best effort: the server still rejects duplicates on upload
			}
		}
	}

	// --- proposing releases ------------------------------------------------

	function groupItems(added) {
		// files without an album tag in a folder of numbered files are an album too
		const numberedByFolder = new Map();
		for (const it of added) {
			if (it.tags.album) continue;
			const folder = it.path.includes("/") ? it.path.slice(0, it.path.lastIndexOf("/")) : "";
			if (!folder || !trackFromFile(it.path)) continue;
			numberedByFolder.set(folder, (numberedByFolder.get(folder) || 0) + 1);
		}

		for (const it of added) {
			if (it.dupOf) continue;
			const t = it.tags;
			let key = null;
			let title = "";
			let date = t.date;
			if (t.album) {
				key = `album:${norm(t.album)}|${norm(t.albumArtist)}`;
				title = t.album;
			} else {
				const folder = it.path.includes("/") ? it.path.slice(0, it.path.lastIndexOf("/")) : "";
				if ((numberedByFolder.get(folder) || 0) >= 2) {
					key = `folder:${folder}`;
					title = folder.split("/").pop();
					// "Album (2021)" -> title and date
					const m = title.match(/^(.*?)\s*[([](\d{4})[)\]]$/);
					if (m) [title, date] = [m[1], date || m[2]];
					it.tags.track ||= trackFromFile(it.path);
				}
			}

			// a lone track tagged as its own one-track album is a single
			const lone = t.trackTotal === 1 || t.releaseType === "single" || (t.album && norm(t.album) === norm(t.title));
			const sameAlbum = added.filter((o) => !o.dupOf && o.tags.album && norm(o.tags.album) === norm(t.album)).length;
			if (!key || (lone && !groups.some((g) => g.key === key) && sameAlbum === 1)) {
				it.groupID = null;
				continue;
			}

			let group = groups.find((g) => g.key === key);
			if (!group) {
				group = { id: nextID++, key, title, type: "", typeSource: "", date: date || "", artists: "", itemIDs: [], el: null, releaseID: null };
				groups.push(group);
			}
			group.itemIDs.push(it.id);
			it.groupID = group.id;
		}

		for (const g of groups) {
			sortGroup(g);
			if (!g.artists) g.artists = groupArtists(g.itemIDs.map((id) => items.get(id)));
			if (!g.typeSource || g.typeSource !== "chosen") suggestType(g);
		}
		groups.sort((a, b) => a.title.localeCompare(b.title));
	}

	function sortGroup(g) {
		g.itemIDs.sort((a, b) => {
			const x = items.get(a);
			const y = items.get(b);
			return (x.values.disc || 1) - (y.values.disc || 1) || (x.tags.track || 1e9) - (y.tags.track || 1e9) || x.path.localeCompare(y.path);
		});
	}

	function groupArtists(list) {
		const albumArtist = list.map((it) => it.tags.albumArtist).find(Boolean);
		if (albumArtist) return splitArtists(albumArtist).join(", ");
		const first = list[0]?.values.artists;
		if (first && list.every((it) => it.values.artists === first)) return first;
		return creator();
	}

	// The type is only suggested: it's shown as such and one click changes it.
	function suggestType(g) {
		const list = g.itemIDs.map((id) => items.get(id));
		const tagged = list.map((it) => it.tags.releaseType).find(Boolean);
		if (tagged) {
			g.type = tagged;
			g.typeSource = "from tags";
			return;
		}
		const n = Math.max(list.length, ...list.map((it) => it.tags.trackTotal || 0));
		g.type = n <= 3 ? "single" : n <= 6 ? "ep" : "album";
		g.typeSource = `suggested, ${n} ${n === 1 ? "track" : "tracks"}`;
	}

	// --- rendering ---------------------------------------------------------

	function render() {
		syncAll();
		groups = groups.filter((g) => g.itemIDs.length);
		groupsEl.replaceChildren(...groups.map(renderGroup));
		const singles = [...items.values()].filter((it) => !it.groupID && !it.dupOf);
		singlesEl.replaceChildren(...singles.map((it) => renderItem(it, templates.single)));
		$("import-singles-count").textContent = `(${singles.length})`;
		$("import-singles-section").hidden = !singles.length;

		const dups = [...items.values()].filter((it) => it.dupOf).length;
		const inLib = [...items.values()].filter((it) => it.inLibrary).length;
		const count = (type) => groups.filter((g) => g.type === type).length;
		summary.textContent =
			`${items.size} files: ${count("album")} albums, ${count("ep")} EPs, ${count("compilation")} compilations, ` +
			`${count("single") + singles.length} singles` +
			(inLib ? `, ${inLib} already in the library` : "") +
			(dups ? `, ${dups} duplicate files left out` : "") +
			".";

		moveSelect.replaceChildren(new Option("Move to release…", ""), ...groups.map((g) => new Option(g.title || "Untitled", g.id)));
		updateSelection();
		renderArtists();
	}

	// --- artists -----------------------------------------------------------

	// every artist name used in the review, with how many tracks and releases use it
	function detectedArtists() {
		const counts = new Map();
		const add = (field) => splitArtists(field).forEach((n) => counts.set(n, (counts.get(n) || 0) + 1));
		for (const g of groups) add(g.artists);
		for (const it of items.values()) if (!it.dupOf) add(it.values.artists);
		return [...counts].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
	}

	function renderArtists() {
		const detected = detectedArtists();
		const fresh = detected.filter(([name]) => !knownArtists.has(name.toLowerCase())).length;
		$("import-artists-count").textContent = `(${detected.length}, ${fresh} new)`;
		root.querySelector("[data-action=add-all-artists]").disabled = busy || !fresh;
		artistRows.replaceChildren(
			...detected.map(([name, count]) => {
				const row = templates.artist.content.firstElementChild.cloneNode(true);
				const known = knownArtists.has(name.toLowerCase());
				row.dataset.name = name;
				row.querySelector("[data-artist-name]").value = name;
				row.querySelector("[data-count]").textContent = `${count}×`;
				const badge = row.querySelector("[data-badge]");
				badge.dataset.state = known ? "known" : "new";
				badge.textContent = known ? "In library" : "New";
				row.querySelector("[data-action=add-detected]").hidden = known;
				row.querySelectorAll("input, button").forEach((el) => (el.disabled = busy));
				return row;
			}),
		);
	}

	// creates the artists now, so they exist before the import and show up as known
	async function addArtists(names) {
		names = names.map((n) => n.trim()).filter((n) => n && !knownArtists.has(n.toLowerCase()));
		if (!names.length) return;
		artistStatus.textContent = "";
		try {
			const ids = await postJSON("/api/artists/resolve", { names });
			for (const name of Object.keys(ids)) {
				knownArtists.set(name.toLowerCase(), name);
				artistList.append(new Option(name));
			}
		} catch (err) {
			artistStatus.textContent = `Could not add: ${err.message}`;
		}
		renderArtists();
	}

	// renames an artist in every artists field; typing an existing artist's
	// name (in any case) merges into that artist
	function replaceArtist(from, to) {
		to = knownArtists.get(to.trim().toLowerCase()) || to.trim();
		if (!to || to === from) return renderArtists();
		const swap = (field) =>
			[...new Set(splitArtists(field).map((n) => (n.toLowerCase() === from.toLowerCase() ? to : n)))].join(", ");
		syncAll();
		for (const g of groups) g.artists = swap(g.artists);
		for (const it of items.values()) it.values.artists = swap(it.values.artists);
		render();
	}

	// --- artist pickers ----------------------------------------------------

	// every name a picker offers: library artists plus names used in the review
	function artistUniverse() {
		const names = new Map(knownArtists);
		for (const [name] of detectedArtists()) if (!names.has(name.toLowerCase())) names.set(name.toLowerCase(), name);
		return [...names.values()].sort((a, b) => a.localeCompare(b));
	}

	// the item or group a picker edits, with get/set for its artists string
	function pickerOwner(picker) {
		const row = picker.closest(".import-row");
		if (row) {
			const it = items.get(Number(row.dataset.item));
			return { done: it.done, get: () => it.values.artists, set: (v) => (it.values.artists = v) };
		}
		const g = groups.find((g) => g.id === Number(picker.closest(".import-group").dataset.group));
		return { done: Boolean(g.releaseID), get: () => g.artists, set: (v) => (g.artists = v) };
	}

	function setSummary(picker, artists) {
		picker.querySelector("[data-artist-summary]").textContent = splitArtists(artists).join(", ") || "Select artists";
	}

	// Options are built when a picker opens, not per row up front: an import
	// can have thousands of rows.
	function fillPicker(picker) {
		const owner = pickerOwner(picker);
		const selected = new Set(splitArtists(owner.get()).map((n) => n.toLowerCase()));
		picker.querySelector("[data-artist-options]").replaceChildren(
			...artistUniverse().map((name) => {
				const label = templates.option.content.firstElementChild.cloneNode(true);
				const box = label.querySelector("input");
				box.value = name;
				box.dataset.name = name;
				box.checked = selected.has(name.toLowerCase());
				box.disabled = busy || owner.done;
				label.querySelector("[data-option-name]").textContent = name;
				label.querySelector("[data-option-new]").hidden = knownArtists.has(name.toLowerCase());
				return label;
			}),
		);
	}

	// toggle doesn't bubble, so listen in the capture phase
	document.addEventListener(
		"toggle",
		(e) => {
			if (e.target.matches?.("[data-import-picker]") && e.target.open) fillPicker(e.target);
		},
		true,
	);

	// Enter with no matching artist adds the typed name. root sees the event
	// before artist-picker.js's document listener, which is stopped here so it
	// doesn't toggle whatever option comes first once the list resets.
	root.addEventListener("keydown", (e) => {
		if (e.key !== "Enter" || !e.target.matches("[data-import-picker] [data-artist-search]")) return;
		const picker = e.target.closest("[data-import-picker]");
		const add = picker.querySelector("[data-action=picker-add]");
		if (add.hidden || picker.querySelector("[data-artist-options] label:not([hidden])")) return;
		e.preventDefault();
		e.stopPropagation();
		add.click();
	});

	// offers "Add …" when the search matches no artist exactly
	root.addEventListener("input", (e) => {
		if (!e.target.matches("[data-import-picker] [data-artist-search]")) return;
		const picker = e.target.closest("[data-import-picker]");
		const query = e.target.value.trim();
		const add = picker.querySelector("[data-action=picker-add]");
		const exists = artistUniverse().some((n) => n.toLowerCase() === query.toLowerCase());
		add.hidden = !query || exists;
		add.textContent = `Add “${query}”`;
		add.dataset.name = query;
	});

	function renderGroup(g) {
		const el = templates.group.content.firstElementChild.cloneNode(true);
		el.dataset.group = g.id;
		el.querySelector("[data-field=title]").value = g.title;
		el.querySelector("[data-field=date]").value = g.date;
		setSummary(el.querySelector("[data-import-picker]"), g.artists);
		el.querySelector("[data-type-source]").textContent = g.typeSource ? `(${g.typeSource})` : "";
		el.querySelectorAll("[data-type]").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.type === g.type)));
		el.querySelector("[data-rows]").replaceChildren(...g.itemIDs.map((id) => renderItem(items.get(id), templates.row)));
		g.el = el;
		renumber(g);
		return el;
	}

	function renderItem(it, template) {
		const el = template.content.firstElementChild.cloneNode(true);
		el.dataset.item = it.id;
		el.querySelector("[data-field=title]").value = it.values.title;
		setSummary(el.querySelector("[data-import-picker]"), it.values.artists);
		el.querySelector("[data-file]").textContent = it.path;
		el.querySelector("[data-file]").title = it.path;
		const disc = el.querySelector("[data-field=disc]");
		if (disc) disc.value = it.values.disc || 1;
		const date = el.querySelector("[data-field=date]");
		if (date) date.value = it.values.date || "";
		it.el = el;
		showItemState(it);
		return el;
	}

	function showItemState(it) {
		const status = it.el.querySelector("[data-status]");
		if (it.done) return;
		if (it.inLibrary && it.groupID) setStatus(it, "link", "Already in the library: it will be added to this release as is.");
		else if (it.inLibrary) setStatus(it, "exists", "Already in the library: skipped. Move it into a release to add it there.");
		else if (status.dataset.state !== "error") setStatus(it, "", "");
	}

	function setStatus(it, state, msg) {
		const status = it.el.querySelector("[data-status]");
		status.dataset.state = state;
		status.textContent = msg;
	}

	// Positions on the release: the tags' track numbers when every track has
	// one and none repeat within a disc, so a partial album leaves gaps where
	// its missing tracks go; otherwise the row order within each disc.
	function positionsFor(g, discOf = (it) => it.values.disc || 1) {
		const list = g.itemIDs.map((id) => items.get(id));
		const seen = new Set();
		const fromTags = list.every((it) => {
			const key = `${discOf(it)}:${it.tags.track}`;
			if (!it.tags.track || seen.has(key)) return false;
			seen.add(key);
			return true;
		});
		const positions = new Map();
		const next = {};
		for (const it of list) {
			const disc = discOf(it);
			next[disc] = (next[disc] || 0) + 1;
			positions.set(it.id, fromTags ? it.tags.track : next[disc]);
		}
		return positions;
	}

	function renumber(g) {
		const discOf = (it) => parseInt(it.el.querySelector("[data-field=disc]").value, 10) || 1;
		const positions = positionsFor(g, discOf);
		for (const id of g.itemIDs) {
			const it = items.get(id);
			it.el.querySelector("[data-number]").textContent = positions.get(id);
		}
	}

	// copies what the user typed back into the model before re-rendering
	function syncAll() {
		for (const it of items.values()) {
			if (!it.el?.isConnected) continue;
			const field = (name) => it.el.querySelector(`[data-field=${name}]`);
			it.values.title = field("title").value;
			if (field("disc")) it.values.disc = parseInt(field("disc").value, 10) || 1;
			if (field("date")) it.values.date = field("date").value.trim();
		}
		for (const g of groups) {
			if (!g.el?.isConnected) continue;
			g.title = g.el.querySelector("[data-field=title]").value;
			g.date = g.el.querySelector("[data-field=date]").value.trim();
		}
	}

	// --- regrouping --------------------------------------------------------

	const selectedIDs = () => [...root.querySelectorAll("[data-select]:checked")].map((c) => Number(c.closest(".import-row").dataset.item));

	function updateSelection() {
		const n = selectedIDs().length;
		$("import-selected").textContent = n ? `${n} selected` : "Select tracks to regroup them";
		root.querySelectorAll("[data-action$=-selected]").forEach((b) => (b.disabled = !n || busy));
		moveSelect.disabled = !n || busy || !groups.length;
	}

	function detach(ids) {
		for (const g of groups) g.itemIDs = g.itemIDs.filter((id) => !ids.includes(id));
	}

	function toGroup(ids, group) {
		syncAll();
		detach(ids);
		ids.forEach((id) => {
			items.get(id).groupID = group.id;
			group.itemIDs.push(id);
		});
		render();
	}

	function newGroupFrom(ids) {
		syncAll();
		const list = ids.map((id) => items.get(id));
		const album = list.map((it) => it.tags.album).find(Boolean);
		const g = { id: nextID++, key: `manual:${nextID}`, title: album || list[0].values.title, type: "", typeSource: "", date: list.map((it) => it.values.date || it.tags.date).find(Boolean) || "", artists: groupArtists(list), itemIDs: [], el: null, releaseID: null };
		groups.push(g);
		toGroup(ids, g);
		sortGroup(g);
		suggestType(g);
		render();
	}

	function toSingles(ids) {
		syncAll();
		detach(ids);
		ids.forEach((id) => (items.get(id).groupID = null));
		render();
	}

	function removeItems(ids) {
		syncAll();
		detach(ids);
		ids.forEach((id) => items.delete(id));
		render();
	}

	root.addEventListener("click", (e) => {
		const btn = e.target.closest("button");
		if (!btn || busy) return;
		const groupEl = btn.closest(".import-group");
		const group = groupEl && groups.find((g) => g.id === Number(groupEl.dataset.group));

		if (btn.dataset.type && group) {
			group.type = btn.dataset.type;
			group.typeSource = "chosen";
			groupEl.querySelectorAll("[data-type]").forEach((b) => b.setAttribute("aria-pressed", String(b === btn)));
			groupEl.querySelector("[data-type-source]").textContent = "";
			render();
			return;
		}
		switch (btn.dataset.action) {
			case "pick-folder":
				folderInput.click();
				break;
			case "pick-files":
				filesInput.click();
				break;
			case "group-selected":
				newGroupFrom(selectedIDs());
				break;
			case "singles-selected":
				toSingles(selectedIDs());
				break;
			case "remove-selected":
				removeItems(selectedIDs());
				break;
			case "split":
				toSingles([...group.itemIDs]);
				break;
			case "remove-group":
				removeItems([...group.itemIDs]);
				break;
			case "import":
				runImport();
				break;
			case "picker-add": {
				const picker = btn.closest("[data-import-picker]");
				const owner = pickerOwner(picker);
				owner.set([...splitArtists(owner.get()), btn.dataset.name].join(", "));
				const search = picker.querySelector("[data-artist-search]");
				search.value = "";
				search.dispatchEvent(new Event("input", { bubbles: true }));
				fillPicker(picker);
				setSummary(picker, owner.get());
				renderArtists();
				break;
			}
			case "add-detected":
				addArtists([btn.closest(".import-artist").dataset.name]);
				break;
			case "add-all-artists":
				addArtists(detectedArtists().map(([name]) => name));
				break;
			case "add-artist": {
				const input = $("import-new-artist");
				addArtists([input.value]).then(() => (input.value = ""));
				break;
			}
		}
	});

	$("import-new-artist").addEventListener("keydown", (e) => {
		if (e.key !== "Enter") return;
		e.preventDefault();
		root.querySelector("[data-action=add-artist]").click();
	});

	root.addEventListener("change", (e) => {
		if (e.target.matches("[data-select]")) updateSelection();
		if (e.target.matches("[data-import-picker] input[name=artist_id]")) {
			const picker = e.target.closest("[data-import-picker]");
			const checked = [...picker.querySelectorAll("input[name=artist_id]:checked")].map((c) => c.dataset.name);
			pickerOwner(picker).set(checked.join(", "));
			renderArtists();
		}
		if (e.target.matches("[data-artist-name]")) replaceArtist(e.target.closest(".import-artist").dataset.name, e.target.value);
		if (e.target.matches("[data-field=disc]")) {
			const g = groups.find((g) => g.id === Number(e.target.closest(".import-group").dataset.group));
			renumber(g);
		}
	});

	moveSelect.addEventListener("change", () => {
		const g = groups.find((g) => g.id === Number(moveSelect.value));
		if (g) toGroup(selectedIDs(), g);
		moveSelect.value = "";
	});

	folderInput.addEventListener("change", () => addFiles(folderInput.files).finally(() => (folderInput.value = "")));
	filesInput.addEventListener("change", () => addFiles(filesInput.files).finally(() => (filesInput.value = "")));

	// --- importing ---------------------------------------------------------

	function validate() {
		syncAll();
		const problems = [];
		for (const g of groups) {
			const status = g.el.querySelector("[data-group-status]");
			const errs = [];
			if (!g.title.trim()) errs.push("needs a title");
			if (!g.type) errs.push("needs a type");
			if (g.date && !RELEASE_DATE.test(g.date)) errs.push("date must be YYYY, YYYY-MM or YYYY-MM-DD");
			if (!splitArtists(g.artists || creator()).length) errs.push("needs an artist");
			status.textContent = errs.length ? `This release ${errs.join(", ")}.` : "";
			problems.push(...errs);
		}
		for (const it of items.values()) {
			if (it.dupOf || it.done || !it.el) continue;
			const errs = [];
			if (!it.values.title.trim()) errs.push("needs a title");
			if (!splitArtists(it.values.artists || artistsFallback(it)).length) errs.push("needs an artist");
			if (!it.groupID && it.values.date && !RELEASE_DATE.test(it.values.date)) errs.push("date must be YYYY, YYYY-MM or YYYY-MM-DD");
			if (errs.length) setStatus(it, "error", `This track ${errs.join(", ")}.`);
			problems.push(...errs);
		}
		return problems.length === 0;
	}

	function artistsFallback(it) {
		const g = groups.find((g) => g.id === it.groupID);
		return (g && g.artists) || creator();
	}

	async function postJSON(url, body) {
		const res = await fetch(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
		if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
		return res.json();
	}

	function lock(locked) {
		busy = locked;
		root.querySelectorAll("input, button, select").forEach((el) => {
			const row = el.closest(".import-row");
			const item = row && items.get(Number(row.dataset.item));
			el.disabled = locked || Boolean(item?.done) || (item?.groupID && Boolean(groups.find((g) => g.id === item.groupID)?.releaseID) && el.matches("[data-field=disc]"));
		});
		// a created release keeps its details; edit them on its page afterwards
		for (const g of groups) {
			if (g.releaseID) g.el.querySelectorAll(".import-group > div input, .import-group > div button, .import-group > label input").forEach((el) => (el.disabled = true));
		}
		if (!locked) updateSelection();
	}

	async function runImport() {
		if (!validate()) {
			result.textContent = "Fix the highlighted releases and tracks first.";
			return;
		}
		lock(true);
		result.textContent = "Creating artists and releases…";

		const allNames = new Set();
		for (const g of groups) splitArtists(g.artists || creator()).forEach((n) => allNames.add(n));
		for (const it of items.values()) if (!it.dupOf) splitArtists(it.values.artists || artistsFallback(it)).forEach((n) => allNames.add(n));

		let artistIDs;
		try {
			artistIDs = await postJSON("/api/artists/resolve", { names: [...allNames] });
		} catch (err) {
			result.textContent = `Could not create the artists: ${err.message}`;
			lock(false);
			return;
		}
		for (const name of Object.keys(artistIDs)) {
			if (knownArtists.has(name.toLowerCase())) continue;
			knownArtists.set(name.toLowerCase(), name);
			artistList.append(new Option(name));
		}
		renderArtists();
		const idsFor = (s) => splitArtists(s).map((n) => artistIDs[n]).filter(Boolean);

		const tasks = [];
		for (const g of groups) {
			if (!g.releaseID) {
				try {
					const res = await postJSON("/api/releases", { title: g.title.trim(), type: g.type, date: g.date, artist_ids: idsFor(g.artists || creator()) });
					g.releaseID = res.id;
				} catch (err) {
					g.el.querySelector("[data-group-status]").textContent = `Could not create this release: ${err.message}`;
					continue;
				}
			}
			// the same numbers the review shows; deterministic, so a retry reuses them
			const positions = positionsFor(g);
			for (const id of g.itemIDs) {
				const it = items.get(id);
				if (!it.done) tasks.push({ it, release: { id: g.releaseID, disc: it.values.disc || 1, position: positions.get(id) } });
			}
		}
		for (const it of items.values()) {
			if (!it.groupID && !it.dupOf && !it.inLibrary && !it.done) tasks.push({ it, release: null });
		}

		let finished = 0;
		const counts = { done: 0, existed: 0, failed: 0 };
		result.textContent = `Uploading 0 / ${tasks.length}…`;
		tasks.forEach(({ it }) => setStatus(it, "pending", "Waiting…"));

		await pool(tasks, UPLOADS_AT_ONCE, async ({ it, release }) => {
			setStatus(it, "pending", "Uploading…");
			const fd = new FormData();
			fd.append("title", it.values.title.trim());
			idsFor(it.values.artists || artistsFallback(it)).forEach((id) => fd.append("artist_id", id));
			if (release) {
				fd.append("release_id", release.id);
				fd.append("disc", release.disc);
				fd.append("position", release.position);
			} else if (it.values.date) {
				fd.append("date", it.values.date);
			}
			// MusicBrainz tags let the auto-tagger match the track without a lookup
			if (it.tags.mb.recording) fd.append("mbid", it.tags.mb.recording);
			if (it.tags.isrc) fd.append("isrc", it.tags.isrc);
			fd.append("track", it.file, it.file.name);
			try {
				const res = await fetch("/ui/tracks", { method: "POST", body: fd });
				const text = (await res.text()).replace(/<[^>]*>/g, "").trim();
				if (res.ok) {
					it.done = true;
					counts.done++;
					setStatus(it, "done", text);
				} else if (res.status === 409) {
					it.done = true;
					counts.existed++;
					setStatus(it, "exists", text.includes("release") ? "Already on this release." : "Already in the library: skipped.");
				} else {
					counts.failed++;
					setStatus(it, "error", text || `Upload failed (HTTP ${res.status}).`);
				}
			} catch {
				counts.failed++;
				setStatus(it, "error", "Upload failed: network error.");
			}
			result.textContent = `Uploading ${++finished} / ${tasks.length}…`;
		});

		const parts = [`${counts.done} imported`];
		if (counts.existed) parts.push(`${counts.existed} already existed`);
		if (counts.failed) parts.push(`${counts.failed} failed`);
		result.textContent = parts.join(", ") + (counts.failed ? ". Fix or remove the failed tracks and click Import everything again." : ".");
		if (!counts.failed) {
			const link = document.createElement("a");
			link.href = "/releases";
			link.textContent = " View releases";
			result.append(link);
		}
		lock(false);
	}
})();
