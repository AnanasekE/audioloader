# Plan 28: Better default artist pictures

**Goal:** artists without a picture stop sharing one orange placeholder (`assets/img/default_pfp.png`). In normal mode they show the cover of their newest release; otherwise a pixel-font initials tile, in a colour picked from the name.

**Depends on:** plan 7 (artist pictures), plan 21 (JSON API `imageUrl`), plan 25 (pixel-art icon style).

## Status

- [x] 1. Initials tile generator
- [ ] 2. Serve the fallbacks
- [ ] 3. Use them everywhere, docs

## Rules

As in plan 19: one commit per step; `make build`, `go vet ./...` and `make test` pass; no attribution lines. For `mobile/` changes: `npx tsc --noEmit` and `npx jest` pass.

## Decisions

- **Order:** the artist's own picture → (normal mode only) the front cover of their newest release that has one → the initials tile. Never the release cover in secret mode, and never a release the viewer can't see.
- **The tile is a PNG drawn in Go**, not a web font: it works the same in `<img>`, in the mobile app and on the lock screen, and it is pixel art by construction.
  - New package `internal/avatar`: `func Initials(name string) string` and `func Tile(name string) ([]byte, error)`.
  - A built-in 5×7 bitmap font (A–Z, 0–9) as a Go table, in the style of the favicon. Each glyph pixel is drawn as a 4×4 block on a 64×64 canvas, centred, with a 1-glyph-pixel gap between letters.
  - **Initials:** the first letter of the first two words ("Little V" → "LV", "CerberVT" → "C", "Hatsune Miku" → "HM"). Letters are uppercased, ASCII-folded (é → E with `golang.org/x/text` only if it's already a dependency; otherwise a small map for common accents). A name with no usable letter or digit (e.g. Japanese only) gets a pixel music-note glyph instead.
  - **Colour:** an FNV hash of the lowercased name picks one of 8 muted background colours that read on the dark theme and the light theme (take hues from the Console palette in `styles/input.css`; letters in a light ink that passes 4.5:1 contrast on each, checked in a test). Same name, same colour, always.
- **Serving:** `GET /artist-images/{artist_id}` keeps its URL. Today it serves the stored picture or `default_pfp.png`; now it follows the order above. The cover is streamed from the release's stored cover (reuse the `/covers/` file lookup, not a redirect, so the URL never reveals a release ID). Responses set `Cache-Control: private, max-age=3600` and an `ETag` from (picture version, release cover version or tile name+colour), so lists don't refetch.
- **`?v=`** cache-busting on artist image URLs must also change when the fallback would change (a release cover appears). If that's awkward, the short `max-age` above is enough; say which in the report.
- **JSON API:** artist `imageUrl` is always `/artist-images/{id}` from now on (it was "" for the placeholder). Update `docs/api.md`, the shape test, and the mobile app, which can drop its own placeholder for artists.
- **`default_pfp.png`** is deleted once nothing references it. `default_track.png` and `default_secret.png` stay.
- **Secret leak test:** in secret mode a shared artist's image is never a release cover; in normal mode a secret creator is 404 as today.

## Steps

### 1. Initials tile generator (`internal/avatar`)
- Font table, initials, colours, PNG encoding.
- Tests: initials table (one word, two words, three words, lowercase, accents, digits, emoji only, Japanese only, empty); same name gives the same bytes; every colour passes the contrast check; output is a 64×64 PNG.
- Write a few sample tiles to the scratchpad and look at them.

### 2. Serve the fallbacks (`internal/api/artists.go`, `internal/db`)
- `db.NewestReleaseCover(v Viewer, artistID string)` returning the release ID with a cover, or "", newest by date then added.
- The endpoint follows the order. Tests: own picture wins; cover used in normal mode; never in secret mode (tile instead); no releases gives the tile; ETag and 304.

### 3. Use them everywhere, docs
- Templates that special-case the default picture (e.g. a `default_pfp` check or a "has picture" flag) now always use the endpoint. Keep "Remove picture" working: it falls back to cover or tile.
- JSON API and mobile changes as decided.
- Headless Firefox screenshots of `/artists` at 1440×900 in normal and secret mode, in the scratchpad.
- `FEATURES.md`: artists without a picture show their newest release cover, else a pixel initials tile. `docs/plans/README.md`: row 28, update the status line.
