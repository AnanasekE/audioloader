// The colour behind the Now Playing sheet (now-playing.js): the cover's average
// colour, darkened and toned down so white text stays readable on it.
(() => {
	// white text needs at least this contrast on the tint (WCAG AA)
	const MIN_CONTRAST = 4.5;
	const MAX_SATURATION = 0.55;
	const MIN_LIGHTNESS = 0.14;
	const MAX_LIGHTNESS = 0.3;

	function toHSL(r, g, b) {
		r /= 255;
		g /= 255;
		b /= 255;
		const max = Math.max(r, g, b);
		const min = Math.min(r, g, b);
		const l = (max + min) / 2;
		if (max === min) return [0, 0, l];
		const d = max - min;
		const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
		const h = max === r ? (g - b) / d + (g < b ? 6 : 0) : max === g ? (b - r) / d + 2 : (r - g) / d + 4;
		return [h * 60, s, l];
	}

	function fromHSL(h, s, l) {
		const a = s * Math.min(l, 1 - l);
		const f = (n) => {
			const k = (n + h / 30) % 12;
			return Math.round(255 * (l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1))));
		};
		return { r: f(0), g: f(8), b: f(4) };
	}

	// contrastWithWhite is the WCAG contrast ratio of white on the colour
	function contrastWithWhite({ r, g, b }) {
		const lin = (c) => ((c /= 255) <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
		return 1.05 / (0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b) + 0.05);
	}

	// tintFrom returns the {r, g, b} to put behind white text for a cover whose
	// average colour is r, g, b: less saturated, dark, and darker still if that
	// is what it takes to reach the contrast.
	function tintFrom(r, g, b) {
		const [h, s, l0] = toHSL(r, g, b);
		const sat = Math.min(s, MAX_SATURATION);
		let l = Math.min(MAX_LIGHTNESS, Math.max(MIN_LIGHTNESS, l0 * 0.55));
		let tint = fromHSL(h, sat, l);
		while (contrastWithWhite(tint) < MIN_CONTRAST && l > 0) {
			l = Math.max(0, l - 0.01);
			tint = fromHSL(h, sat, l);
		}
		return tint;
	}

	// gradient is the sheet's background: the tint at the top, fading to near black
	function gradient({ r, g, b }) {
		const deep = [r, g, b].map((c) => Math.round(c * 0.35));
		return `linear-gradient(to bottom, rgb(${r} ${g} ${b}), rgb(${deep.join(" ")}))`;
	}

	window.coverTint = { tintFrom, gradient, contrastWithWhite };
	if (typeof document === "undefined") return;

	const tints = new Map(); // image URL -> tint, or null when it can't be read

	// fromImage reads the cover at src (a 16x16 copy of it, averaged) and
	// returns its tint, or null if the image can't be read.
	async function fromImage(src) {
		if (tints.has(src)) return tints.get(src);
		let tint = null;
		try {
			const img = new Image();
			img.src = src;
			await img.decode();
			const canvas = document.createElement("canvas");
			canvas.width = canvas.height = 16;
			const ctx = canvas.getContext("2d");
			ctx.drawImage(img, 0, 0, 16, 16);
			const px = ctx.getImageData(0, 0, 16, 16).data;
			let r = 0;
			let g = 0;
			let b = 0;
			let n = 0;
			for (let i = 0; i < px.length; i += 4) {
				if (px[i + 3] < 128) continue;
				r += px[i];
				g += px[i + 1];
				b += px[i + 2];
				n++;
			}
			if (n) tint = tintFrom(r / n, g / n, b / n);
		} catch {
			// an image that is gone or can't be read: no tint
		}
		tints.set(src, tint);
		return tint;
	}

	window.coverTint.fromImage = fromImage;
})();
