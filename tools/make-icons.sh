#!/usr/bin/env bash
# Regenerates every app icon from assets/img/favicon.png (32x32 pixel art).
#
#   tools/make-icons.sh
#
# Needs ImageMagick. The art is scaled by a whole number with nearest-neighbour
# so the pixels stay sharp, and centred on the app background colour.
# Output: assets/img/icon-{180,192,512}.png and mobile/assets/images/*.png.
set -euo pipefail
cd "$(dirname "$0")/.."
SRC=assets/img/favicon.png
BG='#0e0f11'
M=mobile/assets/images
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# the art without its transparent margin
magick "$SRC" -trim +repage "$TMP/art.png"

# art SIZE FRACTION OUT [bg]: art at about FRACTION of SIZE (a whole-number
# scale), centred on a SIZE square that is BG, or transparent when no bg
art() {
	local size=$1 frac=$2 out=$3 bg=${4:-none} k
	k=$(awk -v s="$size" -v f="$frac" 'BEGIN { k = int(s * f / 32 + 0.5); print (k < 1 ? 1 : k) }')
	magick "$TMP/art.png" -filter point -resize "$((k * 100))%" \
		-background "$bg" -gravity center -extent "${size}x${size}" "$out"
	# an opaque background means a PNG without an alpha channel
	[ "$bg" = none ] || magick "$out" -alpha off "$out"
}

# web home-screen icons
art 180 0.70 assets/img/icon-180.png "$BG"
art 192 0.70 assets/img/icon-192.png "$BG"
art 512 0.70 assets/img/icon-512.png "$BG"

# mobile
art 1024 0.70 $M/icon.png "$BG"
art 1024 0.70 $M/splash-icon.png
art 64 0.90 $M/favicon.png
# Android adaptive icons: the safe zone is the middle 61% of the canvas
art 1024 0.50 $M/android-icon-foreground.png
art 1024 0.50 $M/android-icon-monochrome.png
magick $M/android-icon-monochrome.png -channel RGB -fill white -colorize 100 +channel $M/android-icon-monochrome.png
magick -size 1024x1024 "xc:$BG" $M/android-icon-background.png
