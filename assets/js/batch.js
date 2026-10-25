// Batch upload: builds one row per picked file and uploads every row
// with its own htmx request (see batchRow in internal/components/batch.templ).
// Each row goes to the release picked in its Release select, or becomes a new single.
(() => {
	const root = document.getElementById("batch");
	if (!root) return;

	// track-hash.js; if it failed to load, skip the early check (the server still rejects duplicates)
	const { trackFingerprint } = window;
	const picker = document.getElementById("batch-picker");
	const start = document.getElementById("batch-start");
	const table = document.getElementById("batch-table");
	const rows = document.getElementById("batch-rows");
	const result = document.getElementById("batch-result");
	const defaultArtist = document.getElementById("batch-default-artist");
	const defaultRelease = document.getElementById("batch-default-release");
	const template = document.getElementById("batch-row-template");

	let uploading = false;
	let checks = Promise.resolve(); // pending duplicate checks; uploads wait for them

	const stripExt = (name) => name.replace(/\.[^.]+$/, "");
	const releaseOf = (row) => row.querySelector("select[name=release_id]").value;

	// A file already in the library can't become a new single, but can be
	// added to an album (e.g. the earlier single's song), so whether such a
	// row is uploaded depends on its current release choice.
	const skipped = (row) => row.hasAttribute("data-skip") || (row.hasAttribute("data-in-library") && !releaseOf(row));
	const pendingRows = () => [...rows.querySelectorAll(".batch-row:not([data-done])")].filter((row) => !skipped(row));

	function makeRow(file) {
		const row = template.content.firstElementChild.cloneNode(true);

		// a file input only accepts a FileList, which DataTransfer can build
		const dt = new DataTransfer();
		dt.items.add(file);
		row.querySelector("input[name=track]").files = dt.files;

		row.querySelector("[data-file-name]").textContent = file.name;
		row.querySelector("input[name=title]").value = stripExt(file.name);
		if (defaultArtist.value) setArtist(row, defaultArtist.value);
		updateArtistSummary(row);
		row.querySelector("select[name=release_id]").value = defaultRelease.value;
		updateRelease(row);
		return row;
	}

	function setArtist(row, id) {
		row.querySelectorAll("input[name=artist_id]").forEach((c) => (c.checked = c.value === id));
		updateArtistSummary(row);
	}

	function updateArtistSummary(row) {
		const names = [...row.querySelectorAll("input[name=artist_id]:checked")].map((c) => c.dataset.name);
		row.querySelector("[data-artist-summary]").textContent = names.length ? names.join(", ") : "Select artists";
	}

	const isLocked = (row) => row.querySelector("input[name=title]").disabled;

	// keeps the disc field and the in-library message in line with the release choice
	function updateRelease(row) {
		const release = releaseOf(row);
		row.querySelector("input[name=disc]").disabled = !release || isLocked(row);
		if (!row.hasAttribute("data-in-library") || row.hasAttribute("data-done")) return;
		if (release) setStatus(row, "link", "Already in the library: it will be added to this release as is.");
		else setStatus(row, "exists", "Already in the library. Pick an album to add it to, or remove it.");
	}

	function setStatus(row, state, msg) {
		const status = row.querySelector(".row-status");
		status.dataset.state = state;
		status.textContent = msg;
	}

	function lockRow(row, locked) {
		row.querySelectorAll("input, button, select").forEach((el) => (el.disabled = locked));
		row.querySelector("details").open = false;
		row.querySelector("summary").style.pointerEvents = locked ? "none" : "";
		if (!locked) updateRelease(row);
		// skipped rows can still be taken out of the batch
		if (row.hasAttribute("data-skip") && !uploading) {
			row.querySelector("[data-action=remove]").disabled = false;
		}
	}

	function skipRow(row, msg) {
		row.dataset.skip = "";
		setStatus(row, "exists", msg);
		lockRow(row, true);
	}

	function setActionsLocked(locked) {
		root
			.querySelectorAll("[data-action=submit], [data-action=pick], #batch-default-artist, #batch-default-release")
			.forEach((el) => (el.disabled = locked));
	}

	// Marks rows whose file is already in the library, or already earlier in
	// this batch. The server checks again on upload, so this is only a preview.
	async function checkDuplicates(newRows) {
		if (!trackFingerprint?.supported()) return;

		const fps = await Promise.all(newRows.map((row) => trackFingerprint.of(row.querySelector("input[name=track]").files[0])));
		newRows.forEach((row, i) => {
			row.dataset.size = fps[i].size;
			row.dataset.hash = fps[i].hash;
		});

		const toCheck = [];
		for (const row of newRows) {
			const twin = [...rows.querySelectorAll(".batch-row[data-hash]")].find(
				(other) => other !== row && other.dataset.hash === row.dataset.hash && other.dataset.size === row.dataset.size,
			);
			if (twin && twin.compareDocumentPosition(row) & Node.DOCUMENT_POSITION_FOLLOWING) {
				skipRow(row, "Same file is already in this batch.");
			} else {
				toCheck.push(row);
			}
		}
		if (!toCheck.length) return;

		try {
			const exists = await trackFingerprint.exist(
				toCheck.map((row) => ({ size: Number(row.dataset.size), hash: row.dataset.hash })),
			);
			toCheck.forEach((row, i) => {
				if (!exists[i]) return;
				row.dataset.inLibrary = "";
				updateRelease(row);
			});
		} catch {
			// best effort: the upload itself still rejects duplicates
		}
	}

	picker.addEventListener("change", () => {
		const files = [...picker.files];
		picker.value = "";
		if (!files.length) return;

		const newRows = files.map(makeRow);
		for (const row of newRows) {
			rows.append(row);
			htmx.process(row); // rows come from a <template>, so htmx hasn't seen them yet
		}
		start.hidden = true;
		table.hidden = false;
		checks = checks.then(() => checkDuplicates(newRows));
	});

	// "New artist" form: the server's HX-Trigger carries {id, name} of the new
	// artist; add it to the default select, the row template and every row
	document.body.addEventListener("artistsChanged", function onArtistsChanged(e) {
		// body survives boosted navigation, this page's elements don't
		if (!root.isConnected) {
			document.body.removeEventListener("artistsChanged", onArtistsChanged);
			return;
		}
		const { id, name } = e.detail;
		if (!id) return;

		defaultArtist.append(new Option(name, id));
		for (const list of [template.content, rows]) {
			list.querySelectorAll(".batch-row").forEach((row) => {
				const label = artistCheckbox(id, name);
				row.querySelector("[data-artist-options]").append(label);
				if (row.querySelector("summary").style.pointerEvents === "none") {
					label.querySelector("input").disabled = true;
				}
			});
		}

		// a freshly added creator is most likely the one being uploaded
		if (!defaultArtist.value) {
			defaultArtist.value = id;
			defaultArtist.dispatchEvent(new Event("change"));
		}
	});

	// same markup as the checkboxes in batchRow
	function artistCheckbox(id, name) {
		const label = document.createElement("label");
		label.className = "flex cursor-pointer items-center gap-2 rounded px-1 py-1 text-sm hover:bg-zinc-100 dark:hover:bg-zinc-700";
		const input = document.createElement("input");
		input.type = "checkbox";
		input.name = "artist_id";
		input.value = id;
		input.dataset.name = name;
		label.append(input, name);
		return label;
	}

	defaultArtist.addEventListener("change", () => {
		if (!defaultArtist.value) return;
		pendingRows().forEach((row) => setArtist(row, defaultArtist.value));
	});

	defaultRelease.addEventListener("change", () => {
		rows.querySelectorAll(".batch-row:not([data-done]):not([data-skip])").forEach((row) => {
			row.querySelector("select[name=release_id]").value = defaultRelease.value;
			updateRelease(row);
		});
	});

	root.addEventListener("change", (e) => {
		if (e.target.name === "release_id") updateRelease(e.target.closest(".batch-row"));
	});

	root.addEventListener("click", (e) => {
		const btn = e.target.closest("[data-action]");
		if (!btn || uploading) return;

		switch (btn.dataset.action) {
			case "pick":
				picker.click();
				break;
			case "remove":
				if (rows.children.length > 1) {
					btn.closest(".batch-row").remove();
				} else if (confirm("Remove the last track and close the batch?")) {
					// through htmx, so the player keeps playing
					htmx.ajax("GET", root.dataset.cancelUrl, { target: "body", push: "true" });
				}
				break;
			case "submit":
				uploadAll();
				break;
		}
	});

	// Gives each pending row that goes to a release its position: the next
	// free slot of its release and disc, in row order. A row keeps its
	// position across retries, so earlier attempts hold on to theirs.
	function assignPositions(pending) {
		const serverNext = JSON.parse(root.dataset.nextPositions || "{}");
		const next = {};
		const nextFor = (key) => {
			if (next[key] === undefined) {
				const [release, disc] = key.split(":");
				next[key] = Number(serverNext[release]?.[disc]) || 1;
			}
			return next[key];
		};

		rows.querySelectorAll(".batch-row[data-position]").forEach((row) => {
			const key = row.dataset.positionKey;
			next[key] = Math.max(nextFor(key), Number(row.dataset.position) + 1);
		});
		for (const row of pending) {
			const release = releaseOf(row);
			const input = row.querySelector("input[name=position]");
			if (!release) {
				input.value = "";
				continue;
			}
			const key = `${release}:${row.querySelector("input[name=disc]").value}`;
			if (row.dataset.positionKey !== key) {
				row.dataset.position = nextFor(key);
				row.dataset.positionKey = key;
				next[key] = Number(row.dataset.position) + 1;
			}
			input.value = row.dataset.position;
		}
	}

	async function uploadAll() {
		uploading = true;
		setActionsLocked(true);
		await checks;

		const pending = pendingRows();
		if (!pending.length) {
			uploading = false;
			setActionsLocked(false);
			result.textContent = "Nothing to upload: every track already exists.";
			return;
		}

		// validate everything first: a row that fails htmx's own validation
		// never sends a request, which would break the completion count
		let valid = true;
		for (const row of pending) {
			if (!row.hasAttribute("data-in-library")) setStatus(row, "", "");
			if (!row.querySelector("input[name=artist_id]:checked")) {
				setStatus(row, "error", "Pick at least one artist.");
				valid = false;
			}
			if (!row.reportValidity()) valid = false;
		}
		if (!valid) {
			uploading = false;
			setActionsLocked(false);
			return;
		}

		assignPositions(pending);

		let remaining = pending.length;
		const counts = { uploaded: 0, failed: 0 };
		result.textContent = `Uploading ${pending.length} track(s)…`;

		for (const row of pending) {
			setStatus(row, "pending", "Uploading…");
			row.addEventListener(
				"htmx:afterRequest",
				(e) => {
					const xhr = e.detail.xhr;
					if (e.detail.successful) {
						// the server's Notice is already swapped into .row-status
						counts.uploaded++;
						row.dataset.done = "";
						row.querySelector(".row-status").dataset.state = "done";
					} else if (xhr.status === 409 && xhr.responseText.includes("release")) {
						row.dataset.skip = "";
						setStatus(row, "exists", "Already on this release.");
					} else if (xhr.status === 409) {
						// lost the race to an identical upload: same as found in the library
						row.dataset.inLibrary = "";
						setStatus(row, "exists", "Already in the library. Pick an album to add it to, or remove it.");
					} else {
						counts.failed++;
						// htmx doesn't swap 4xx/5xx responses, so show the error text here
						setStatus(row, "error", xhr.responseText.trim() || "Upload failed.");
					}
					if (--remaining === 0) finish(counts);
				},
				{ once: true },
			);
		}

		// htmx reads each form's values synchronously when the event fires,
		// so locking the rows right after is safe
		pending.forEach((row) => htmx.trigger(row, "batch-submit"));
		rows.querySelectorAll(".batch-row").forEach((row) => lockRow(row, true));
	}

	function finish({ uploaded, failed }) {
		uploading = false;
		const existed = [...rows.querySelectorAll(".batch-row:not([data-done])")].filter(skipped).length;
		const parts = [`${uploaded} uploaded`];
		if (existed) parts.push(`${existed} skipped as already existing`);
		if (failed) parts.push(`${failed} failed`);

		if (failed === 0 && existed === 0) {
			result.textContent = parts.join(", ") + ". ";
			const link = document.createElement("a");
			link.href = root.dataset.doneUrl;
			link.textContent = root.dataset.doneUrl.includes("/releases/") ? "View release" : "View tracks";
			result.append(link);
			return; // batch is done: everything stays locked
		}

		result.textContent = parts.join(", ") + (failed ? ". Fix the failed rows and click Upload all again." : ".");
		// rows that weren't uploaded stay editable, so an existing file can still be pointed at an album
		rows.querySelectorAll(".batch-row:not([data-done])").forEach((row) => lockRow(row, row.hasAttribute("data-skip")));
		setActionsLocked(false);
	}
})();
