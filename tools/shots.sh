#!/usr/bin/env bash
# Screenshots app pages and the matching mockup views, for comparing them.
#
#   tools/shots.sh [page...]
#
# A page is a path (/tracks, /search?q=low), a path for the unlocked vault
# (secret:/tags), or an alias for a page with a generated id: @release (two
# discs), @release-single, @release-edit, @artist, @playlist, @playlist-edit,
# @show, @follow, @track-edit. With no pages it shoots all of them.
# Output: tmp/shots/<name>-<w>.png, and mock-<view>-<w>.png for the mockup.
# SHOTS_RESEED=1 starts over with a fresh seeded database.
# SHOTS_H=2400 makes the screenshots taller, to see a long page.
# SHOTS_THEME=light shoots the light theme (name gets a -light suffix).
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT=$PWD
OUT=$ROOT/tmp/shots
RUN=$OUT/run
PORT=${SHOTS_PORT:-18090}
SPORT=$((PORT + 1))
BASE=http://127.0.0.1:$PORT
SBASE=http://127.0.0.1:$SPORT
SIZES=("1440 ${SHOTS_H:-900}" "390 ${SHOTS_H:-844}")
THEME=${SHOTS_THEME:-dark}
SUFFIX=; [ "$THEME" = light ] && SUFFIX=-light

mkdir -p "$OUT"
[ -n "${SHOTS_RESEED:-}" ] && rm -rf "$RUN"
if [ ! -f "$RUN/database.db" ]; then
	rm -rf "$RUN"
	go run ./tools/seed -db "$RUN/database.db" -files "$RUN/data"
fi
ln -sfn "$ROOT/assets" "$RUN/assets"
go build -o "$OUT/cookieproxy" ./tools/cookieproxy

PASS=shots
HASH=$(echo "$PASS" | "$ROOT/audioloader" -hash-secret 2>/dev/null)

pids=()
cleanup() { for p in "${pids[@]:-}"; do kill "$p" 2>/dev/null || true; done; }
trap cleanup EXIT

(cd "$RUN" && AUDIOLOADER_DB="$RUN/database.db" AUDIOLOADER_ADDR=127.0.0.1:$PORT AUDIOLOADER_SECRET="$HASH" \
	exec "$ROOT/audioloader" >"$OUT/server.log" 2>&1) &
pids+=($!)
for _ in $(seq 50); do curl -sf -o /dev/null "$BASE/tracks" && break; sleep 0.2; done
curl -sf -o /dev/null "$BASE/tracks" || { echo "server did not start; see $OUT/server.log" >&2; exit 1; }

COOKIE=$(curl -s -D - -o /dev/null -d "passphrase=$PASS" "$BASE/vault" | tr -d '\r' | sed -n 's/^[Ss]et-[Cc]ookie: \([^;]*\).*/\1/p' | head -1)
[ -n "$COOKIE" ] || { echo "vault unlock gave no cookie" >&2; exit 1; }
"$OUT/cookieproxy" -listen 127.0.0.1:$SPORT -upstream "$BASE" -cookie "$COOKIE" >/dev/null 2>&1 &
pids+=($!)
sleep 0.5

q() { sqlite3 "$RUN/database.db" "$1"; }
alias_path() {
	case $1 in
	@release) echo "/releases/$(q "select uuid from releases where title='Slow Weather'")" ;;
	@release-single) echo "/releases/$(q "select uuid from releases where title='Harbor Lights'")" ;;
	@release-edit) echo "/releases/$(q "select uuid from releases where title='Slow Weather'")/edit" ;;
	@artist) echo "/artists/$(q "select uuid from artists where name='Lumen Park'")" ;;
	@follow) echo "/artists/$(q "select uuid from artists where name='Mara Voss'")/follow" ;;
	@playlist) echo "/playlists/$(q "select uuid from playlists where name='Late night'")" ;;
	@playlist-edit) echo "/playlists/$(q "select uuid from playlists where name='Late night'")/edit" ;;
	@show) echo "/shows/$(q "select uuid from shows where title='The Long Tape'")" ;;
	@track-edit) echo "/tracks/$(q "select uuid from tracks where title='Salt and Static'")/edit" ;;
	*) echo "unknown alias $1" >&2; return 1 ;;
	esac
}

DEFAULT=(/ /tracks "/search?q=low" @release @release-single /releases /artists @artist /playlists @playlist @playlist-edit
	/shows @show /inbox /downloads /tagging /upload /upload/batch /upload/import @release-edit @track-edit
	secret:/tracks secret:/tags secret:/artists)
[ $# -eq 0 ] && set -- "${DEFAULT[@]}"

FF=$(mktemp -d)
shoot() { # url png width height
	rm -rf "$FF/p"; mkdir "$FF/p"
	if [ "$THEME" = light ]; then
		echo 'user_pref("ui.systemUsesDarkTheme", 0);' >"$FF/p/user.js"
	fi
	timeout 90 firefox --headless --no-remote --profile "$FF/p" --window-size="$3,$4" --screenshot "$2" "$1" >/dev/null 2>&1 || true
	[ -s "$2" ] || echo "no screenshot for $1" >&2
}

mock_view() { # name -> "view secret?"
	case $1 in
	secret-tracks) echo "tracks 1" ;;
	secret-tags) echo "tags 1" ;;
	tracks*|search*) echo "tracks 0" ;;
	release*) echo "release 0" ;;
	playlist*) echo "playlists 0" ;;
	show*) echo "podcasts 0" ;;
	inbox*) echo "inbox 0" ;;
	downloads*) echo "downloads 0" ;;
	esac
}

shoot_mock() { # view secret
	local view=$1 sec=$2 name=$1
	[ "$sec" = 1 ] && name=secret-$1
	local html=$OUT/mock-$name.html
	{
		cat "$ROOT/docs/design/console-mockup.html"
		echo "<script>"
		[ "$THEME" = light ] && echo "theme('light');"
		[ "$sec" = 1 ] && echo "secret(true);"
		echo "go('$view');</script>"
	} >"$html"
	local s w h
	for s in "${SIZES[@]}"; do
		read -r w h <<<"$s"
		shoot "file://$html" "$OUT/mock-$name-$w$SUFFIX.png" "$w" "$h"
	done
}

declare -A mocked
for page in "$@"; do
	base=$BASE
	case $page in secret:*) base=$SBASE; page=${page#secret:}; pre=secret- ;; *) pre= ;; esac
	if [[ $page == @* ]]; then name=${page#@}; path=$(alias_path "$page"); else path=$page
		name=$(echo "${page#/}" | tr -c 'A-Za-z0-9\n' '-' | sed 's/-*$//'); [ -n "$name" ] || name=index; fi
	name=$pre$name
	url=$base$path
	if [ "$THEME" = light ]; then
		case $path in *\?*) url="$url&theme=light" ;; *) url="$url?theme=light" ;; esac
	fi
	for s in "${SIZES[@]}"; do
		read -r w h <<<"$s"
		shoot "$url" "$OUT/$name-$w$SUFFIX.png" "$w" "$h"
	done
	echo "shot $name"
	read -r view sec <<<"$(mock_view "$name")"
	if [ -n "${view:-}" ] && [ -z "${mocked[$view$sec]:-}" ]; then
		mocked[$view$sec]=1
		shoot_mock "$view" "$sec"
		echo "shot mock-$view"
	fi
done
rm -rf "$FF"
