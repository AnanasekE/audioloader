# Plan 8: Artist links (MusicBrainz pick, Twitch)

**Goal:** link artists to MusicBrainz by picking from the top 5 with enough detail to tell them apart, and to a Twitch channel for a profile picture.

**Covers (REQUESTS.md):** Pick the right match from the top 5 (MusicBrainz artists), Twitch channel on artists.

**Depends on:** plan 7 (artist page, profile pictures).

## Schema (one migration)
```sql
ALTER TABLE artists ADD COLUMN twitch_login TEXT;  -- lowercase login, NULL = none
```

## Steps

### 1. Richer MusicBrainz candidates
- Extend `mb.Artist` with `life-span` (begin, end, ended) and `area`; the search already returns them.
- Candidate component (shared): name, disambiguation, type, country, active years. An "Expand" button fetches a few release-group titles for that one candidate (`BrowseReleaseGroups`, first page, top 5 by date), under the shared 1 req/s limiter. Only when expanded.
- Use it on the Follow page in place of today's list.

### 2. "Link to MusicBrainz" on the artist page
- Shown when the artist has no MBID: opens the same candidate list. "Use this" sets `artists.mbid`; "None of these" closes it. A pasted `musicbrainz.org/artist/<mbid>` URL still works (parse and validate the UUID).
- If another artist already has that MBID, offer "Merge into <name>" instead of setting a duplicate.

### 3. Picks from tagging
- When an applied tagging match credits an artist whose MBID isn't set and the names are close but not exact (normalized, then a small edit distance in `internal/match`), add a "link artist?" row on the Tagging page with the candidate list for that artist.
- **Never guess:** the MBID is set only from a pick, an exact embedded tag, or an exact name match (as now). Test that a close-but-not-exact name leaves the MBID empty.

### 4. Twitch client (`internal/twitch`)
- Config: `TWITCH_CLIENT_ID`, `TWITCH_CLIENT_SECRET`. Without both, the client is nil and fetching is off; the field can still be saved.
- Token: `POST https://id.twitch.tv/oauth2/token` with `grant_type=client_credentials`. Cache it until `expires_in` minus a margin; on a 401, drop it and retry once.
- `User(ctx, login)`: `GET https://api.twitch.tv/helix/users?login=<login>` with `Client-Id` and `Authorization: Bearer`. Empty `data` means unknown channel.
- Fetch the image only from the URL Helix returns, and only if its host is `static-cdn.jtvnw.net`. Size cap (e.g. 5 MB) and type check, then store it with plan 7's picture storage.
- Tests: an `httptest` fake for both hosts (make the base URLs settable), token caching, expiry, unknown login.

### 5. Twitch field on the artist page
- Input accepts a login or a `twitch.tv/<login>` URL. Normalize to lowercase and validate `^[a-z0-9_]{3,25}$`.
- **On save:** if the artist still has the default picture and the client is configured, fetch the profile image and store it.
- **"Update picture from Twitch"** button: replaces the picture with the channel's current one. Nothing does this automatically.
- **Errors:** unknown channel or failed fetch shows a message; the picture is left alone.
- Show a link to `https://www.twitch.tv/<login>` on the artist page.

### 6. Docs
- `README.md`: the two Twitch variables. `FEATURES.md`: both features.

## Verification
- Tests above.
- **Manual check:** link an artist with a common name by expanding two candidates and comparing releases. Add a Twitch login to an artist with no picture and see it appear; change the channel's picture and press Update.

## Out of scope
Scheduled picture refreshes, Twitch live status.
