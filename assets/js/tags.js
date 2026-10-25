// Reads the embedded tags of an audio file in the browser, before anything is
// uploaded: ID3v2 (MP3), Vorbis comments (FLAC, Ogg Opus, Ogg Vorbis), MP4
// atoms (M4A) and RIFF chunks (WAV). Only the bytes that are needed are read,
// via File.slice.
// readTags(file) resolves to:
//   { title, artists: [], albumArtist, album, track, trackTotal, disc, discTotal, date, releaseType,
//     isrc, mb: { recording, release, releaseGroup, artists: [] } }
// with "" or 0 for anything missing. mb holds MusicBrainz IDs, as written by Picard. It never rejects: unreadable files give empty tags.
window.readTags = (() => {
	// cover art can make tags large; give up beyond this
	const MAX_TAG_BYTES = 32 << 20;

	// Reader serves byte ranges from a 1 MiB window, so walking many small
	// headers (Ogg pages, MP4 boxes) doesn't mean one File.slice per header.
	class Reader {
		constructor(file) {
			this.file = file;
			this.buf = new Uint8Array(0);
			this.start = 0;
		}
		async bytes(pos, len) {
			if (pos < this.start || pos + len > this.start + this.buf.length) {
				const size = Math.max(len, 1 << 20);
				this.buf = new Uint8Array(await this.file.slice(pos, pos + size).arrayBuffer());
				this.start = pos;
			}
			return this.buf.subarray(pos - this.start, Math.min(pos - this.start + len, this.buf.length));
		}
	}

	const ascii = (b) => String.fromCharCode(...b);
	const be32 = (b, i = 0) => ((b[i] << 24) | (b[i + 1] << 16) | (b[i + 2] << 8) | b[i + 3]) >>> 0;
	const le32 = (b, i = 0) => (b[i] | (b[i + 1] << 8) | (b[i + 2] << 16) | (b[i + 3] << 24)) >>> 0;
	const synchsafe = (b, i = 0) => (b[i] << 21) | (b[i + 1] << 14) | (b[i + 2] << 7) | b[i + 3];
	const utf8 = new TextDecoder("utf-8");

	function empty() {
		return {
			title: "", artists: [], albumArtist: "", album: "", track: 0, trackTotal: 0, disc: 0, discTotal: 0, date: "", releaseType: "",
			isrc: "", mb: { recording: "", release: "", releaseGroup: "", artists: [] },
		};
	}

	const MBID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

	// setMB stores a MusicBrainz ID or ISRC tag. key is the tag's name with the
	// format's prefix removed and lowercased, e.g. "musicbrainz album id".
	// Returns whether key was one of them.
	function setMB(t, key, value) {
		const ids = () => String(value).split(/[\0/;,]\s*|\s+/).map((s) => s.trim().toLowerCase()).filter((s) => MBID.test(s));
		switch (key.replace(/_/g, " ")) {
			// Vorbis and MP4 call the recording ID "track id"; ID3 keeps it in UFID
			case "musicbrainz track id": case "musicbrainz trackid": t.mb.recording ||= ids()[0] || ""; return true;
			case "musicbrainz album id": case "musicbrainz albumid": t.mb.release ||= ids()[0] || ""; return true;
			case "musicbrainz release group id": case "musicbrainz releasegroupid": t.mb.releaseGroup ||= ids()[0] || ""; return true;
			case "musicbrainz artist id": case "musicbrainz artistid": if (!t.mb.artists.length) t.mb.artists = ids(); return true;
			case "isrc": t.isrc ||= String(value).trim().toUpperCase().replace(/-/g, ""); return true;
		}
		return false;
	}

	// "3/12" -> [3, 12]
	function numberPair(s) {
		const [n, total] = String(s).split("/");
		return [parseInt(n, 10) || 0, parseInt(total, 10) || 0];
	}

	function setTrack(t, v) {
		const [n, total] = numberPair(v);
		t.track = n || t.track;
		t.trackTotal = total || t.trackTotal;
	}

	function setDisc(t, v) {
		const [n, total] = numberPair(v);
		t.disc = n || t.disc;
		t.discTotal = total || t.discTotal;
	}

	function setDate(t, v) {
		const m = String(v).match(/^\d{4}(-\d{2}(-\d{2})?)?/);
		if (m && (!t.date || m[0].length > t.date.length)) t.date = m[0];
	}

	// MusicBrainz types can be "album; soundtrack" etc.; keep the first one we model
	function setReleaseType(t, v) {
		for (const word of String(v).toLowerCase().split(/[;,/\s]+/)) {
			if (["album", "ep", "single", "compilation"].includes(word)) {
				t.releaseType ||= word;
				return;
			}
		}
	}

	// --- ID3v2 -------------------------------------------------------------

	function decodeID3Text(enc, b) {
		let text;
		if (enc === 0) text = String.fromCharCode(...b); // ISO-8859-1
		else if (enc === 3) text = utf8.decode(b);
		else if (enc === 2) text = new TextDecoder("utf-16be").decode(b);
		else {
			// UTF-16 with BOM
			const bigEndian = b[0] === 0xfe && b[1] === 0xff;
			text = new TextDecoder(bigEndian ? "utf-16be" : "utf-16le").decode(b.subarray(2));
		}
		// v2.4 separates multiple values with NUL; v2.3 values end with it
		return text.split("\0").map((s) => s.trim()).filter(Boolean);
	}

	// undoes ID3 unsynchronisation: every 0xFF 0x00 becomes 0xFF
	function unsync(b) {
		const out = [];
		for (let i = 0; i < b.length; i++) {
			out.push(b[i]);
			if (b[i] === 0xff && b[i + 1] === 0x00) i++;
		}
		return Uint8Array.from(out);
	}

	// readID3 parses an ID3v2 tag starting at base and returns where it ends.
	async function readID3(reader, base, t) {
		const h = await reader.bytes(base, 10);
		const version = h[3];
		const flags = h[5];
		const size = synchsafe(h, 6);
		if (size > MAX_TAG_BYTES) return base + 10 + size;

		let data = (await reader.bytes(base + 10, size)).slice();
		if (flags & 0x80 && version < 4) data = unsync(data);

		let pos = 0;
		if (flags & 0x40) pos = version === 3 ? be32(data) + 4 : synchsafe(data);

		const short = version === 2; // v2.2: 3-char IDs and 3-byte sizes
		const headerLen = short ? 6 : 10;
		while (pos + headerLen <= data.length) {
			const id = ascii(data.subarray(pos, pos + (short ? 3 : 4)));
			if (!/^[A-Z0-9]+$/.test(id)) break; // padding
			let len = short
				? (data[pos + 3] << 16) | (data[pos + 4] << 8) | data[pos + 5]
				: version === 4
					? synchsafe(data, pos + 4)
					: be32(data, pos + 4);
			let frame = data.subarray(pos + headerLen, pos + headerLen + len);
			if (version === 4) {
				const formatFlags = data[pos + 9];
				if (formatFlags & 0x02) frame = unsync(frame);
				if (formatFlags & 0x01) frame = frame.subarray(4); // data length indicator
			}
			pos += headerLen + len;

			if (id === "TXXX" || id === "TXX") {
				const [desc, value] = decodeID3Text(frame[0], frame.subarray(1));
				if (/^(musicbrainz album type|releasetype)$/i.test(desc || "")) setReleaseType(t, value || "");
				else setMB(t, (desc || "").toLowerCase(), value || "");
				continue;
			}
			if (id === "UFID" || id === "UFI") {
				// owner URL, NUL, then the identifier as plain bytes
				const nul = frame.indexOf(0);
				if (nul > 0 && ascii(frame.subarray(0, nul)) === "http://musicbrainz.org") setMB(t, "musicbrainz track id", ascii(frame.subarray(nul + 1)));
				continue;
			}
			if (id[0] !== "T") continue;
			const values = decodeID3Text(frame[0], frame.subarray(1));
			if (!values.length) continue;
			switch (id) {
				case "TIT2": case "TT2": t.title ||= values[0]; break;
				case "TPE1": case "TP1": if (!t.artists.length) t.artists = values; break;
				case "TPE2": case "TP2": t.albumArtist ||= values[0]; break;
				case "TALB": case "TAL": t.album ||= values[0]; break;
				case "TRCK": case "TRK": setTrack(t, values[0]); break;
				case "TPOS": case "TPA": setDisc(t, values[0]); break;
				case "TSRC": case "TRC": setMB(t, "isrc", values[0]); break;
				case "TDRC": case "TDRL": case "TYER": case "TYE": setDate(t, values[0]); break;
				case "TDAT": case "TDA":
					// v2.3 day and month as DDMM, completing TYER
					if (/^\d{4}$/.test(t.date) && /^\d{4}$/.test(values[0])) t.date += `-${values[0].slice(2)}-${values[0].slice(0, 2)}`;
					break;
			}
		}
		return base + 10 + size;
	}

	// --- WAV ---------------------------------------------------------------

	// WAV tags sit in RIFF chunks, usually after the audio data: an "ID3 "
	// chunk (preferred, it's richer) and/or a LIST/INFO chunk.
	async function readWAV(reader, t) {
		let info = null;
		for (let pos = 12; pos + 8 <= reader.file.size; ) {
			const h = await reader.bytes(pos, 12);
			const id = ascii(h.subarray(0, 4));
			const size = le32(h, 4);
			if (id.toLowerCase() === "id3 ") await readID3(reader, pos + 8, t);
			else if (id === "LIST" && ascii(h.subarray(8, 12)) === "INFO") info = { start: pos + 12, end: pos + 8 + size };
			// chunks are padded to an even size
			pos += 8 + size + (size & 1);
		}
		if (!info) return;
		for (let pos = info.start; pos + 8 <= info.end; ) {
			const h = await reader.bytes(pos, 8);
			const size = le32(h, 4);
			const value = utf8.decode(await reader.bytes(pos + 8, size)).replace(/\0+$/, "").trim();
			switch (ascii(h.subarray(0, 4))) {
				case "INAM": t.title ||= value; break;
				case "IART": if (!t.artists.length && value) t.artists = [value]; break;
				case "IPRD": t.album ||= value; break;
				case "ICRD": setDate(t, value); break;
				case "ITRK": case "IPRT": setTrack(t, value); break;
			}
			pos += 8 + size + (size & 1);
		}
	}

	// --- Vorbis comments ---------------------------------------------------

	function readVorbisComments(b, t) {
		let pos = 4 + le32(b);
		const count = le32(b, pos);
		pos += 4;
		for (let i = 0; i < count && pos + 4 <= b.length; i++) {
			const len = le32(b, pos);
			pos += 4;
			const entry = b.subarray(pos, pos + len);
			pos += len;
			// skip embedded cover art without decoding megabytes of base64
			const key = ascii(entry.subarray(0, Math.min(32, entry.length))).split("=")[0].toUpperCase();
			if (key === "METADATA_BLOCK_PICTURE" || key === "COVERART") continue;
			const text = utf8.decode(entry);
			const value = text.slice(text.indexOf("=") + 1).trim();
			if (!value) continue;
			switch (key) {
				case "TITLE": t.title ||= value; break;
				case "ARTIST": t.artists.push(value); break;
				case "ALBUMARTIST": case "ALBUM ARTIST": case "ALBUM_ARTIST": t.albumArtist ||= value; break;
				case "ALBUM": t.album ||= value; break;
				case "TRACKNUMBER": setTrack(t, value); break;
				case "TRACKTOTAL": case "TOTALTRACKS": t.trackTotal ||= parseInt(value, 10) || 0; break;
				case "DISCNUMBER": setDisc(t, value); break;
				case "DISCTOTAL": case "TOTALDISCS": t.discTotal ||= parseInt(value, 10) || 0; break;
				case "DATE": case "YEAR": case "ORIGINALDATE": setDate(t, value); break;
				case "RELEASETYPE": case "MUSICBRAINZ_ALBUMTYPE": setReleaseType(t, value); break;
				default: setMB(t, key.toLowerCase(), value);
			}
		}
	}

	async function readFLAC(reader, offset, t) {
		let pos = offset + 4;
		while (pos < reader.file.size) {
			const h = await reader.bytes(pos, 4);
			const last = h[0] & 0x80;
			const type = h[0] & 0x7f;
			const len = (h[1] << 16) | (h[2] << 8) | h[3];
			if (type === 4) {
				readVorbisComments(await reader.bytes(pos + 4, len), t);
				return;
			}
			pos += 4 + len;
			if (last) return;
		}
	}

	// The comment header is the second Ogg packet; with cover art it spans many pages.
	async function readOgg(reader, t) {
		let pos = 0;
		let packet = 0;
		let chunks = [];
		while (pos < reader.file.size && pos < MAX_TAG_BYTES) {
			const h = await reader.bytes(pos, 27);
			if (ascii(h.subarray(0, 4)) !== "OggS") return;
			const segments = h[26];
			const lacing = (await reader.bytes(pos + 27, segments)).slice();
			let bodyPos = pos + 27 + segments;
			for (const len of lacing) {
				if (packet === 1) chunks.push((await reader.bytes(bodyPos, len)).slice());
				bodyPos += len;
				if (len < 255) {
					// end of a packet
					if (packet === 1) {
						const b = new Uint8Array(chunks.reduce((n, c) => n + c.length, 0));
						let o = 0;
						for (const c of chunks) {
							b.set(c, o);
							o += c.length;
						}
						if (ascii(b.subarray(0, 8)) === "OpusTags") readVorbisComments(b.subarray(8), t);
						else if (b[0] === 3 && ascii(b.subarray(1, 7)) === "vorbis") readVorbisComments(b.subarray(7), t);
						return;
					}
					packet++;
					chunks = [];
				}
			}
			pos = bodyPos;
		}
	}

	// --- MP4 ---------------------------------------------------------------

	// childBoxes lists the boxes between start and end as {type, start (payload), end}.
	async function childBoxes(reader, start, end) {
		const boxes = [];
		for (let pos = start; pos + 8 <= end; ) {
			const h = await reader.bytes(pos, 16);
			let size = be32(h);
			let header = 8;
			if (size === 1) {
				size = be32(h, 8) * 2 ** 32 + be32(h, 12);
				header = 16;
			} else if (size === 0) {
				size = end - pos;
			}
			if (size < header) break;
			boxes.push({ type: String.fromCharCode(...h.subarray(4, 8)), start: pos + header, end: Math.min(pos + size, end) });
			pos += size;
		}
		return boxes;
	}

	async function readMP4(reader, t) {
		const find = async (start, end, type) => (await childBoxes(reader, start, end)).find((b) => b.type === type);
		const moov = await find(0, reader.file.size, "moov");
		const udta = moov && (await find(moov.start, moov.end, "udta"));
		const meta = udta && (await find(udta.start, udta.end, "meta"));
		// meta is a full box: 4 bytes of version and flags before its children
		const ilst = meta && (await find(meta.start + 4, meta.end, "ilst"));
		if (!ilst) return;

		for (const item of await childBoxes(reader, ilst.start, ilst.end)) {
			const parts = await childBoxes(reader, item.start, item.end);
			const data = parts.find((p) => p.type === "data");
			if (!data || data.end - data.start > MAX_TAG_BYTES) continue;
			// data payload: 4 bytes type, 4 bytes locale, then the value
			const value = (await reader.bytes(data.start + 8, data.end - data.start - 8)).slice();
			const text = () => utf8.decode(value).trim();
			switch (item.type) {
				case "©nam": t.title ||= text(); break;
				case "©ART": if (!t.artists.length && text()) t.artists = [text()]; break;
				case "aART": t.albumArtist ||= text(); break;
				case "©alb": t.album ||= text(); break;
				case "©day": setDate(t, text()); break;
				case "trkn": t.track ||= (value[2] << 8) | value[3]; t.trackTotal ||= (value[4] << 8) | value[5]; break;
				case "disk": t.disc ||= (value[2] << 8) | value[3]; t.discTotal ||= (value[4] << 8) | value[5]; break;
				case "----": {
					const name = parts.find((p) => p.type === "name");
					// name is a full box too: skip version and flags
					const key = name ? utf8.decode(await reader.bytes(name.start + 4, name.end - name.start - 4)) : "";
					if (/^(musicbrainz album type|releasetype)$/i.test(key)) setReleaseType(t, text());
					else setMB(t, key.toLowerCase(), text());
					break;
				}
			}
		}
	}

	return async function readTags(file) {
		const t = empty();
		try {
			const reader = new Reader(file);
			const head = await reader.bytes(0, 12);
			let offset = 0;
			if (ascii(head.subarray(0, 3)) === "ID3") offset = await readID3(reader, 0, t);

			const magic = await reader.bytes(offset, 8);
			if (ascii(magic.subarray(0, 4)) === "fLaC") await readFLAC(reader, offset, t);
			else if (ascii(magic.subarray(0, 4)) === "OggS") await readOgg(reader, t);
			else if (ascii(head.subarray(4, 8)) === "ftyp") await readMP4(reader, t);
			else if (ascii(head.subarray(0, 4)) === "RIFF" && ascii(head.subarray(8, 12)) === "WAVE") await readWAV(reader, t);
		} catch {
			// a damaged tag shouldn't stop the import; the file just has fewer details
		}
		return t;
	};
})();
