# Plan 16: Add an artist from MusicBrainz

**Goal:** an "Add from MusicBrainz" box on the Artists page: type a name, pick one of MusicBrainz's top 5 artists (with the same details and "Expand" as the Follow page), and get that artist in the library with its MusicBrainz name and ID, with "Follow" one click away.

**Covers (REQUESTS.md):** Add an artist from MusicBrainz.

**Depends on:** plan 8 (candidate component, `getCandidateReleases`, "Link to MusicBrainz"), plan 2 (Follow page).

## Decisions
- **Normal mode only.** Both new routes use `handleNormal` and the box is rendered only when `!secretMode(ctx)`: secret artists never come from MusicBrainz. The plain "Add artist" form stays as it is, in both modes, for artists that aren't on MusicBrainz.
- **Reuse, don't copy.** Search is `mb.Default().SearchArtists(ctx, q, 5)` (the 1 req/s limiter applies); each row is the existing `artistCandidate(c) { children }` with its "Expand" button hitting the existing `GET /ui/musicbrainz/artists/{mbid}/releases`. No new `internal/mb` code, no new fake.
- **Library state per candidate**, decided by one DB function so the list and the POST agree:
  - an artist the viewer can see has this MBID → "In your library" link, no add;
  - an artist with this name (case-insensitive) and no MBID → "Link <name>" (sets the ID on that artist, like `linkArtist`);
  - an artist with this name and another MBID → names are unique, so offer "Add as <Name> (<disambiguation>)"; without a disambiguation, say the name is taken and link to that artist;
  - otherwise → "Add".
- **The POST trusts the picked name and ID** from the form (`hx-vals`), as `linkArtist` trusts `mbid`; the ID is checked with `validMBID`, the name trimmed and required. No second MusicBrainz request.
- **Creation goes through `resolveArtistTx`** (`internal/db/inbox.go`): by MBID, else by name giving it the ID, else insert with the ID. That's the rule confirming an inbox release already uses.
- **After a pick** the result replaces the candidate list: "Added X." / "Linked X to MusicBrainz." / "X is already in your library.", with "Open" (`/artists/<id>`) and "Follow" (`/artists/<id>/follow`; the Follow page already pre-selects a linked artist's MBID). It sends the same `artistsChanged` HX-Trigger as `createArtist`, so the list below refreshes.
- **No schema change.**

## Steps

### 1. Find and add a MusicBrainz artist (`internal/db/artists.go`)
- Add:
  ```go
  // MBArtistMatch is how a MusicBrainz artist relates to the library.
  type MBArtistMatch int

  const (
      MBArtistNew       MBArtistMatch = iota // not in the library
      MBArtistLinked                         // an artist has this MBID
      MBArtistSameName                       // an artist has this name and no MBID
      MBArtistNameTaken                      // an artist has this name and another MBID
  )

  // FindMBArtist reports which library artist, if any, stands for the
  // MusicBrainz artist (mbid, name), among the artists v can see.
  func FindMBArtist(v Viewer, mbid, name string) (*audio.Artist, MBArtistMatch, error)

  // AddMBArtist adds the MusicBrainz artist, or links the same-name artist
  // without an ID, or returns the artist that has the ID. It returns what it
  // found before the change. ErrArtistExists: the name belongs to an artist
  // with another MBID.
  func AddMBArtist(v Viewer, mbid, name string) (*audio.Artist, MBArtistMatch, error)
  ```
- `FindMBArtist`: `SELECT a.uuid FROM artists a WHERE a.mbid = ? AND <v.artistCond("a")>` → `MBArtistLinked`; else `SELECT a.uuid, COALESCE(a.mbid, '') FROM artists a WHERE lower(a.name) = lower(?) AND <v.artistCond("a")>` → `MBArtistSameName` (empty mbid) or `MBArtistNameTaken`; else `nil, MBArtistNew`. Return artists via `GetArtist(v, id)`. Make the body a `findMBArtist(q querier, v Viewer, mbid, name string) (id string, m MBArtistMatch, err error)` so `AddMBArtist` can run it inside its transaction.
- `AddMBArtist`: one transaction. `findMBArtist(tx, ...)`; `MBArtistNameTaken` → `ErrArtistExists`; `MBArtistLinked` → no write; `MBArtistNew` / `MBArtistSameName` → `resolveArtistTx(tx, MatchArtist{ID: mbid, Name: name})`. Commit, then `GetArtist(All, id)`; if its MBID isn't `mbid` (a same-name artist hidden from v that has another ID) return `ErrArtistExists`. Map a unique violation on `mbid` (an artist hidden from v has it) to `ErrMBIDTaken`.
- **Tests** (`internal/db/artists_test.go`, `TestAddMBArtist`):
  - new name → `MBArtistNew`, artist created with the MBID; calling again → `MBArtistLinked`, same ID, still one artist;
  - "massive attack" without an MBID, add "Massive Attack" → `MBArtistSameName`, that artist now has the MBID, no new artist;
  - name taken by an artist with another MBID → `ErrArtistExists`, nothing changed;
  - `FindMBArtist` for each case without writing anything.

### 2. Search and add (handlers, components)
- **Routes** (`internal/api/routes.go`, both `handleNormal`, next to `/ui/artists`):
  - `GET /ui/artists/musicbrainz?q=` → `searchArtistsMusicBrainz`
  - `POST /ui/artists/musicbrainz` → `addArtistFromMusicBrainz` (form: `mbid`, `name`)
- **Handlers** (`internal/api/artists.go`):
  - Move the `HX-Trigger` block of `createArtist` into `func triggerArtistsChanged(w http.ResponseWriter, a *audio.Artist) error` and call it from both handlers.
  - `searchArtistsMusicBrainz`: trim `q`; empty → 400 "type a name". `mb.Default().SearchArtists(r.Context(), q, 5)`; on error log it and render the list with `searchErr = "Couldn't search MusicBrainz."` (as `getArtistLink` does). For each result build:
    ```go
    // components.MBArtistCandidate
    type MBArtistCandidate struct {
        Artist  mb.Artist
        Match   db.MBArtistMatch
        Library *audio.Artist // the matching library artist, nil for MBArtistNew
        AddName string        // name to add it under; "" when it can't be added
    }
    ```
    from `db.FindMBArtist(viewer(r), c.ID, c.Name)`. `AddName` is `c.Name`, except for `MBArtistNameTaken`: `c.Name + " (" + c.Disambiguation + ")"` when there is a disambiguation and `FindMBArtist` reports that name free, else `""`. Render `components.MBArtistCandidates(found, searchErr)`.
  - `addArtistFromMusicBrainz`: `mbid := strings.ToLower(validMBID(r.FormValue("mbid")))`, `name := strings.TrimSpace(r.FormValue("name"))`; either empty → 400. `db.AddMBArtist(viewer(r), mbid, name)`: `ErrArtistExists` → 409 "<name> is already in the library as another artist."; `ErrMBIDTaken` → 409 with its message; other errors → `serverError`. On success `triggerArtistsChanged` (only when the match was `MBArtistNew`, so the list isn't refetched for nothing) and render `components.MBArtistAdded(a, match)`.
- **Components** (`internal/components/artists.templ`):
  - `templ AddArtistFromMusicBrainz()`: a `card` section "Add from MusicBrainz" with a form `hx-get="/ui/artists/musicbrainz" hx-target="#mb-artist-results" hx-disabled-elt="find button"`, one input `name="q"` (required, placeholder "Artist name"), a "Search" button, and `<div id="mb-artist-results" aria-live="polite">`.
  - `ArtistsPage`: render `if !secretMode(ctx) { @AddArtistFromMusicBrainz() }` after `@AddArtistForm()`.
  - `templ MBArtistCandidates(found []MBArtistCandidate, searchErr string)`: a `card grid gap-3` with `hx-target="#mb-artist-results"` and the same `hx-on::after-request` error line as `ArtistLinkCandidates` (`data-link-result` span). Each row is `@artistCandidate(c.Artist) { ... }` with, by `c.Match`:
    - `MBArtistNew`: `<button class="btn shrink-0" hx-post="/ui/artists/musicbrainz" hx-vals={ templ.JSONString(map[string]string{"mbid": c.Artist.ID, "name": c.AddName}) }>Add</button>`
    - `MBArtistSameName`: the same button, text "Link " + `c.Library.Name`
    - `MBArtistLinked`: `<a class="btn shrink-0" href={ artistURL(c.Library) }>In your library</a>`
    - `MBArtistNameTaken`: with `AddName != ""` the Add button labelled "Add as " + `AddName`; else a muted "Name taken by" link to `c.Library`.
    Empty list with no error: "MusicBrainz found no artist with that name. Add it by name above." A "None of these" button clears `#mb-artist-results`.
  - `templ MBArtistAdded(a *audio.Artist, m db.MBArtistMatch)`: the message for `m` (see Decisions), an "Open" link and, if `!a.Followed`, a "Follow" link to `/artists/<id>/follow`; else "Following".
  - `components` already imports `internal/db` in other templ files; add it to `artists.templ`'s imports.
- `make generate` before `make build`.
- **Tests** (`internal/api/artists_test.go`, `TestAddArtistFromMusicBrainz`, modelled on `TestLinkArtist`):
  - `fakeMusicBrainz(t, map[string]string{"/artist": <3 artists>})` with "Massive Attack" (`10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8`, country GB), "Portishead" and "Tricky" (disambiguation "UK rapper"). In the library first: "portishead" without an MBID, and "Tricky" with another MBID (`db.SetArtistMBID`).
  - `GET /ui/artists/musicbrainz?q=Massive` → 200; body has "Add", "Link portishead", "Add as Tricky (UK rapper)" and the Expand URL `/ui/musicbrainz/artists/10adbe5e-.../releases`.
  - `GET` with empty `q` → 400.
  - `POST` `mbid=10adbe5e-...&name=Massive Attack` → 200, `HX-Trigger` contains `artistsChanged`, body has `/artists/<id>/follow`; `db.ArtistByMBID` finds it. Posting again → 200 "already in your library", still one "Massive Attack" (`db.ListArtists`).
  - `POST` Portishead's ID with `name=Portishead` → the existing artist gets the MBID, no new artist.
  - `POST` `name=Tricky` with a new ID → 409; `name=Tricky (UK rapper)` → 200, a second artist.
  - Bad `mbid` → 400.
  - Secret mode: both routes 404 through the `NormalOnly` loop in `leak_test.go`; also check `GET /artists` in secret mode has no "Add from MusicBrainz" (extend `TestSecretModeHidesReleaseControls` in `modes_test.go` or add a case next to it).

### 3. Docs
- `FEATURES.md` (Library): after the "Artists page" bullet add "Add an artist from MusicBrainz (normal mode): search by name, the top 5 with the same details and Expand as the Follow page; picking adds it with its MusicBrainz name and ID, links a same-name artist without an ID, or links to the one already there; then Follow".
- `docs/plans/README.md`: add the row `| 16 | [Add an artist from MusicBrainz](16-add-artist-from-musicbrainz.md) | Add an artist from MusicBrainz (REQUESTS.md) |` and update the status line once implemented.
- `REQUESTS.md`: remove the "Add an artist from MusicBrainz" section.

## Verification
- `make build`, `go vet ./...` and `make test` pass after each step.
- **Manual check:** on `/artists` search "Massive Attack": five rows with country and years; "Expand" lists release groups. Add it: the list below shows it, and "Follow" opens the Follow page with its ID already selected. Search it again: it says "In your library". Add "Portishead" by name with the plain form, then from MusicBrainz: the button says "Link portishead" and no duplicate appears. Unlock the vault: the box is gone.

## Out of scope
Adding and following in one step, importing an artist's discography straight away, MusicBrainz artist search in secret mode.
