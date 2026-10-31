# Plan 23: Add songs from a MusicBrainz link

**Goal:** on the Add page (plan 12), pasting a MusicBrainz link instead of search text lists exactly what it points to. For a release (e.g. `https://musicbrainz.org/release/<mbid>`), that is its whole tracklist, each track with its own Add button, plus "Add all". For a recording, it is that one recording.

**Depends on:** plan 12 (add a song by search).

## Status

- [ ] 1. Parse MusicBrainz links
- [ ] 2. List a release's or recording's tracks
- [ ] 3. Add all tracks of a release
- [ ] 4. Docs

## Rules

As in plan 19: one commit per step; `make build`, `go vet ./...` and `make test` pass; no attribution lines. MusicBrainz calls go through `internal/mb`; tests use `httptest` fakes, never the real service.

## Decisions

- **Same search box, no new page.** `searchSongsMusicBrainz` checks the query first. A MusicBrainz link skips `SearchSongs`. A bare MBID is ambiguous (release or recording?), so it stays a text search.
- **Link forms accepted:** `musicbrainz.org/release/<id>` and `musicbrainz.org/recording/<id>`, with any subdomain (`beta.`), http or https, and any trailing path, query or fragment, as `mb.RecordingID` already does for recordings. A `musicbrainz.org/track/<id>` or `release-group/<id>` link answers 400 "Paste a release or recording link."
- **A release** is one `mb.GetRelease` call. Every track becomes a `SongCandidate` with `ReleaseID` set to *this* release (not `tagging.PickRelease`), the track's own title and length (`firstPositive(t.Length, t.Recording.Length)`, as `addSongFromMusicBrainz` does), disc and position shown, and ownership marked with `ownedSong` as for search results. The cover is the release's front cover. A header shows the release title, artist credit, year and track count.
- **A recording** is one `mb.GetRecording` call (with releases included; check its `inc`), shown as a single candidate with `tagging.PickRelease`, exactly as one search result.
- **Per-track Add** posts the existing `POST /ui/add/musicbrainz` with `recording_id` and `release_id`. No change to it.
- **Add all:** `POST /ui/add/musicbrainz/release` with `release_id`, `handleNormal`. One `GetRelease`, then for each track in disc/position order that the library doesn't already have, the same `db.AddSong` + download queueing that `addSongFromMusicBrainz` does. Factor that per-track part out into a helper both handlers call, so there is one code path. All tracks land on one library release (`AddSong` already reuses it). It answers the refreshed candidate list, so already-added tracks show as owned. A release with more than 100 tracks is refused (400), to avoid a runaway queue.
- **Normal mode only,** like the rest of the Add page.

## Steps

### 1. Parse MusicBrainz links (`internal/mb`)
- `func ParseLink(s string) (kind, id string, ok bool)`, kind "release" or "recording" (and "track", "release-group" so the handler can give the right error). `RecordingID` keeps working; make it use `ParseLink` for links if that's simpler.
- Table tests: both kinds, beta subdomain, http, trailing `/`, query, fragment, uppercase ID (lowercased), other sites, malformed IDs, plain text.

### 2. List a release's or recording's tracks (`internal/api/addsong.go`, `addsong.templ`)
- Branch in `searchSongsMusicBrainz` as decided. Add a release header component above the candidates, and disc/position on each row when they come from a release.
- API tests with a fake MusicBrainz:
  - a release link lists all tracks in order, with `release_id` set to that release in each Add form;
  - a track the library already has shows as owned;
  - a recording link shows one candidate;
  - a track or release-group link gives 400 with the message;
  - an unknown release ID (MB 404) gives a clear 404-style message, not a 502;
  - plain text still searches.

### 3. Add all tracks of a release
- The endpoint and the shared per-track helper as decided. "Add all (n)" in the release header, hidden when every track is owned.
- Tests: adds every missing track once, onto one library release, with one download job each; owned tracks are skipped; a second "Add all" adds nothing; more than 100 tracks is refused; secret mode is 404.

### 4. Docs
- `FEATURES.md`, Add a song: "Paste a MusicBrainz release or recording link to list exactly its tracks; add one or all".
- `docs/plans/README.md`: add row 23 and update the status line to include 23.
