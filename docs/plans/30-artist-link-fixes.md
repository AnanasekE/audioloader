# Plan 30: Artist link fixes

**Goal:** linking an artist to MusicBrainz works for artists whose MusicBrainz name differs from the library name (Hatsune Miku is `初音ミク` there, with "Hatsune Miku" as an alias), and the panel is clear about what each button does.

**Depends on:** plans 8 and 16 (artist link panel, add artist from MusicBrainz).

## Status

- [x] 1. Search aliases and sort names
- [x] 2. A visible manual field and clear buttons

## Rules

As in plan 19: `make build`, `go vet ./...` and `make test` pass; one commit per step; no attribution lines.

## Decisions

- **Why Miku is not found:** `mb.SearchArtists` sends `artist:"hatsune miku"`, which only matches the primary name. The search becomes `artist:P OR alias:P OR sortname:P` (P = `Phrase(name)`), so every caller (link panel, Follow page, Add artist) finds artists by alias too. MusicBrainz scores the results; keep its order.
- **Why the pasted link can't be set:** in `ArtistLinkCandidates` the `<label class="sr-only">` wraps the input, so the input is clipped to 1px and only its "Use" button shows. That lone button, sitting under the candidates, is the "unclear Use button". Fix: a visible input with a visible label "Or paste a MusicBrainz artist link or ID"; the `sr-only` goes away.
- **Button names:** the per-candidate button becomes "Link to this artist", the manual one "Link". Each candidate shows its alias when the hit came from an alias ("alias: Hatsune Miku"). The `aliases` field is already in the search JSON; add it to `mb.Artist` if it is missing.
- **Errors:** a pasted link that isn't a musicbrainz.org artist link shows "That is not a MusicBrainz artist link." in `data-link-result`, as other errors do.

## Steps

### 1. Search aliases and sort names
- Change the query in `internal/mb/api.go`, plus a test with the httptest fake that checks the query string.
- Add `Aliases` to `mb.Artist` if needed, and show the matching alias in `artistCandidate`.

### 2. A visible manual field and clear buttons
- Templ changes as decided, in both the link panel and the Add artist / Follow panels if they share the markup.
- A test in `internal/api` that `POST /ui/artists/{id}/link` with `mbid_manual=https://musicbrainz.org/artist/130d679a-9a92-4373-8348-0800b6b93a30` sets the ID.
- Headless Firefox screenshot of the open panel at 1440×900 in the scratchpad.
- `FEATURES.md`: the alias search under artist links. `REQUESTS.md`: remove the Hatsune Miku entry.
