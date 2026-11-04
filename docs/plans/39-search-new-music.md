# Plan 39: Search for new music to download

**Goal:** find music that isn't in the library by song, artist or release on MusicBrainz, browse an artist's releases and a release's tracklist with owned tracks marked, and add one song, a whole release or picked tracks as placeholders with queued downloads.

**Depends on:** plan 12 (add a song by search), plan 16 (add an artist from MusicBrainz), plan 23 (add from a MusicBrainz link), plan 26 (one Add page).

## Status

- [x] 1. MusicBrainz search by field (`internal/mb`)
- [x] 2. The Find page: search form and results
- [x] 3. Browse an artist and a release
- [ ] 4. Add a song, a release or picked tracks

## Rules

As in plan 19: `make build`, `go vet ./...` and `make test` pass; one commit per step, app runnable after each; no attribution lines in commits; plain JS in `assets/js`, no build step. All MusicBrainz calls go through `internal/mb` (its 1 req/s limiter); tests use `httptest` fakes, never the real service. Never run anything on port 8080. Normal mode only, like the Add page.

## Decisions

- **New page `/add/find`**, linked from the Add page ("Search MusicBrainz…" next to the Add box) and from ⌘K actions (`assets/js/commands.js`: "Find music to download").
- **Form:** a mode switch Song / Release / Artist, a text field, and for Song and Release optional fields Artist, Type (Album, EP, Single, Any) and Year (exact, or a `1990-1999` range). `hx-get` on submit to `/ui/add/find/results`, URL kept in the address (`hx-push-url`) so results can be reloaded.
- **Queries** (Lucene, escaped with the existing helper or a new `mb.Quote`):
  - Song: `recording:"T" AND artist:"A"` (+ `date:[Y1 TO Y2]`, `primarytype:X`) via `SearchRecordings`.
  - Release: new `SearchReleaseGroups(ctx, query, limit)` hitting `/release-group?query=`: `releasegroup:"T" AND artist:"A" AND primarytype:X AND firstreleasedate:[...]`.
  - Artist: existing `SearchArtists`.
  - Limit 25. Fixture tests for the new call and the query builder (`FindQuery(mode, fields) string`, table-tested).
- **Results:** songs as rows (title, artist, release, year, length, "In your library" when a track has that recording ID, else "Add"); release groups as cards (title, artist, type, year, owned count "3/12" when the library has the release); artists as rows (name, disambiguation, "Following" when followed).
- **Browse:** an artist result opens `/add/find/artist/{mbid}`: its release groups by type then date, using `BrowseReleaseGroups` (paged "More"), each linking to the release view. A release group opens `/add/find/release-group/{mbid}`: pick the release with `tracking.PickRelease` over `BrowseReleases`, show its tracklist via `GetRelease`, each track ticked unless owned, owned ones marked "In your library". A "Follow artist" button reuses plan 16.
- **Adding** reuses the plan 23 per-track helper (the code behind `POST /ui/add/musicbrainz`): each picked recording becomes a placeholder on its MusicBrainz release, with artists, IDs, cover and "Cover of" as there, and a download queued in the usual source order. "Add" on a song row adds that one; on the tracklist "Add selected" and "Add whole release". The response replaces the button with "Added — queued" and a link to the release page. Nothing new is written when every picked track is owned.
- **Owned check** by recording MBID, then release MBID; one DB function `OwnedRecordings(ids []string) map[string]string` (recording → track ID) and `OwnedReleaseCounts(releaseGroupIDs []string)`.

## Steps

### 1. MusicBrainz search by field
- `internal/mb`: `SearchReleaseGroups`, `FindQuery` builder, fixture JSON in `internal/mb/testdata`, tests.

### 2. The Find page
- Route, handler (`internal/api/find.go`), `internal/components/find.templ`, the owned-check DB functions with tests, link from the Add page, ⌘K command.

### 3. Browse an artist and a release
- The two browse pages with fake-MB API tests (owned marking, paging).

### 4. Add
- Add one / selected / whole release endpoints reusing the plan 23 helper; API tests (placeholders created, download jobs queued, owned skipped). Headless Firefox screenshots at 1440×900 and 390×844 of results and a tracklist (fake data), in the scratchpad. `FEATURES.md`; remove the request from `REQUESTS.md`.
