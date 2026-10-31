# Plan 25: Icons

**Goal:** the mobile app gets tab icons and a real app icon, and the web app gets icons in the desktop sidebar and a proper home-screen icon (Safari "Add to Home Screen").

**Depends on:** plans 17/18 (web `icon` component), plan 22 (mobile app).

## Status

- [x] 1. App icon images
- [x] 2. Web: sidebar icons and home-screen icon
- [ ] 3. Mobile: tab and header icons, app icon

## Rules

As in plan 19 for Go/web steps (`make build`, `go vet ./...`, `make test`). For `mobile/`: `npx tsc --noEmit`, `npx jest` and `npx expo export --platform web` pass. One commit per step, no attribution lines.

## Decisions

- **One source image:** `assets/img/favicon.png` (32×32 pixel art). Scale it with nearest-neighbour (`magick ... -filter point -resize`, or a tiny Go/Python script if ImageMagick is missing) so it stays sharp. Put it on the app's background colour `#0e0f11` with padding, so iOS doesn't show black corners on transparency: the art at about 70% of the canvas, centred, opaque background.
- **Web home-screen icon:** `assets/img/icon-180.png` (apple-touch-icon), `icon-192.png` and `icon-512.png`, plus `assets/manifest.webmanifest` (name "audioloader", `display: standalone`, `background_color`/`theme_color` `#0e0f11`, the two icons, `start_url: "/tracks"`). Link both from `layout.templ`'s head, plus `<meta name="apple-mobile-web-app-capable" content="yes">` and `theme-color`. Check the security headers (`WithSecurityHeaders`) allow the manifest.
- **Web sidebar icons:** `navLink`/`navAnchor`/`navLinkBadge` take an icon name and render `@icon(name)` before the label, as the phone tab bar already does. Add the missing icons to `icon()` in the same stroke style (24×24, stroke 2): `shows` (microphone), `tagging` (tag with check or wand), `downloads` (arrow down to tray), `lock` exists already. The Upload button gets `upload`. Same icon for the same page in the sidebar, the tab bar and the More sheet.
- **Mobile tab icons:** `@expo/vector-icons` Ionicons (ships with Expo, works in Expo Go; install with `npx expo install @expo/vector-icons` and pin it if not already a direct dependency). Tracks `musical-notes`, Artists `person`, Releases `disc`, Playlists `list`, Podcasts `mic`, Search `search`, outline variants when inactive. Remove `tabBarIcon: () => null` and the hidden icon style, and restore normal label spacing. The header button becomes an icon: `settings-outline` normally, `lock-open` in secret mode (keep an accessibility label).
- **Mobile app icon:** `mobile/assets/images/icon.png` (1024, opaque), `splash-icon.png`, `favicon.png`, and the Android adaptive foreground (art on transparent, inside the safe zone) and monochrome images, all from the same source. The splash background stays `#0e0f11`.

## Steps

### 1. App icon images
- Generate every image above from `assets/img/favicon.png`. Commit the script (`tools/make-icons.sh` or `.py`) so they can be regenerated. Look at the 1024 and 180 images to check they're crisp and centred.

### 2. Web: sidebar icons and home-screen icon
- Head links, manifest, sidebar icons, new `icon()` cases.
- Test: the layout has the `apple-touch-icon` and manifest links; `GET /assets/manifest.webmanifest` serves with a JSON-ish content type.
- Headless Firefox screenshots at 1440×900 and 390×844 in the scratchpad.

### 3. Mobile: tab and header icons, app icon
- As decided. `mobile/README.md`: mention the icons script.
- `FEATURES.md`: one line each for the web home-screen icon and sidebar icons. `docs/plans/README.md`: row 25, status "plans 0–25".
