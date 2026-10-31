# Plan 19: Creator from the secret source URL

**Goal:** when a page from the secret source is checked in secret mode ("Download from URL"), the creator is filled in from the URL instead of always being picked by hand. The user can still change it before downloading.

**Changes a rule:** plans 5 and 11 said creators are "never filled in from the page or guessed". The user lifted that rule for the uploader in the URL. The description and title rules are unchanged. This also settles the open question in REQUESTS.md's iOS share request: a shared page takes its creator from the URL.

**Depends on:** plan 11 (secret source), plan 18 (form kit).

## Status

- [ ] 1. Suggest a creator for a page URL
- [ ] 2. Pre-fill it in the form
- [ ] 3. Docs

## Rules

As in plan 17: one commit per step; `make build`, `go vet ./...` and `make test` pass; no attribution lines; never name the site in code, tests, comments, commits or docs (tests use `secrettest`'s fake host).

## Decisions

- **The uploader comes from the URL path only**, never from page text: `Source.Parent(pageURL)` is the uploader's page, and its last path segment, unescaped, is the uploader name. There's no uploader when `Parent` is "" or the segment is empty.
- **Which creator**, in this order, among secret-mode artists (`db.Viewer` in secret mode):
  1. a creator that already has a track whose `source_url` has this same parent page (the link `uploaderPages` in `internal/api/secretsource.go` already computes, but in the other direction). If several do, take the one with the most such tracks;
  2. else a creator whose name equals the uploader name, case-insensitively;
  3. else a **new creator** named after the uploader. It is not created when the page is checked, only when the download is submitted with it still selected.
- **Always editable:** the suggestion pre-checks the picker. A new one appears as a pre-checked "New: <name>" option at the top of the picker, and unchecking it or picking others works as today. Submitting still requires at least one creator.
- **No schema change.**

## Steps

### 1. Suggest a creator for a page URL (`internal/db`, `internal/sources/secret`)
- `secret`: `func (s *Source) Uploader(pageURL string) string` returns the unescaped last segment of `Parent(pageURL)`, or "". It has table tests: a normal page, a page with no parent, a percent-escaped name, and the wrong host.
- `db`: `func CreatorForUploader(v Viewer, parentPage, name string) (*audio.Artist, error)` follows rules 1–2 above. It returns nil when neither matches. Its tests cover the rule order, case-insensitive matching, and the case where a normal-mode artist with that name is never returned in secret mode.

### 2. Pre-fill it in the form (`internal/api/secretsource.go`, `secretsource.templ`, `artist-picker.js` only if needed)
- After a successful check, the handler computes the uploader and the suggested creator and passes them to `SecretSourceDetails`.
  - An existing creator is rendered checked in `artistPicker`. Give the picker a selected-IDs argument if it lacks one.
  - A new creator is rendered as a checked option carrying `new_creator=<name>`.
- The download POST:
  - if `new_creator` is present and still checked, it creates that creator, or reuses one with the same name (case-insensitive; names are unique) in the same request before queueing;
  - it keeps refusing an empty creator set.
- The page header text changes from "you pick the creators yourself" to "the creator is filled in from the address; change it if it's wrong".
- API tests use the fake host:
  - an existing creator linked by an earlier track's URL is pre-checked;
  - a name match is pre-checked;
  - an unknown uploader shows the "New:" option, and submitting it creates the creator once (a second download doesn't duplicate it);
  - unchecking it and picking another creator works;
  - a URL with no parent shows no suggestion.
- Listing pages (the uploader's page itself) do the same for every recording picked from the list.

### 3. Docs
- `FEATURES.md`, secret mode: replace "you pick the creators (at least one, checked on the server)" with the new behaviour.
- `REQUESTS.md`, iOS share: mark the creator question as decided ("from the URL, as the Download from URL form does since plan 19"), and keep "needs creators" only as the fallback for URLs without an uploader.
- `docs/plans/README.md`: add row 19 and set the status to "plans 0–19".
