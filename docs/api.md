# JSON API

Read-only endpoints under `/api/v1/` for the mobile app, plus the writes playback needs. They read through the same viewer as the HTML pages, so the rules are the same: normal mode sees only normal tracks, secret mode only secret ones, and anything of the other side is a 404.

## Auth and modes

The vault cookie is the only auth. A request without it is in normal mode; with a valid one, in secret mode. The app keeps the cookie like a browser does.

| Route | Answers |
|---|---|
| `GET /api/v1/me` | `{"secret": bool, "vault": bool, "secretSource": bool}`: the mode of this request, whether a vault is configured, and whether the secret source is on (true in secret mode only) |
| `POST /api/v1/vault` | Body `{"passphrase": "..."}` (a form works too). 204 with the `Set-Cookie`, 401 `{"error": ...}` after a 1 s sleep when wrong, 404 when the vault is off |
| `POST /api/v1/vault/lock` | 204 with the clearing cookie |

## Conventions

- Names are camelCase, IDs are strings, durations and positions are seconds (float). Arrays are never `null`.
- Lists answer `{"items": [...], "total": n}`. `?limit=` (default 100, at most 500) and `?offset=` page them; `total` counts all items.
- Errors are `{"error": "message"}` with the status (400 for a bad ID or body, 401 for a wrong passphrase). A 404 comes from the router or the mode check as plain text: treat any 404 as "not there".
- Releases and their covers are normal mode only: `/api/v1/releases...` is 404 in secret mode, and `imageUrl` is never a `/covers/` URL there.
- Images are paths on the same server (`imageUrl`), `""` when there is only the placeholder. An artist's `imageUrl` is never `""`: `/artist-images/{id}` serves their own picture, else (normal mode) the cover of their newest release, else a pixel initials tile. Audio is `streamUrl`, which serves byte ranges; it is `""` for a placeholder track that has no file yet.

## Endpoints

| Route | Answers |
|---|---|
| `GET /api/v1/tracks?sort=&q=&limit=&offset=` | list of tracks with files. `sort` is `added` (default), `creator` or `tag`; `q` keeps tracks whose title or creators contain every word |
| `GET /api/v1/tracks/{track_id}` | one track, plus `releases`, `tags`, `description`, `chapters` |
| `GET /api/v1/search?q=` | `{tracks, artists, releases, playlists, shows}`, each an array of at most 20 |
| `GET /api/v1/autoplay?seed=&exclude=&n=` | list of up to `n` (default 10, at most 50) tracks like the `seed` ones, for when the queue runs out. `seed` is repeated for the tracks just played, the current one first (the first 5 count); `exclude` is repeated for the tracks played recently, which are never suggested, nor are the seeds. Owned music of the viewer's mode only, scored by shared artist, artist on a shared release, release type and year, and in secret mode shared tags; ties and the rest come in random order, so fewer matches than `n` are filled up with random tracks. An episode as the first seed gives an empty list |
| `GET /api/v1/artists`, `GET /api/v1/artists/{artist_id}` | list of artists; one artist with `releases` (normal mode, else `[]`) and `tracks` (those on none of the artist's releases, as on the artist page) |
| `GET /api/v1/releases`, `GET /api/v1/releases/{release_id}` | normal mode only: list of releases; one release with `duration` and `tracks` in order |
| `GET /api/v1/playlists`, `GET /api/v1/playlists/{playlist_id}` | list of playlists; one with `tracks` in order (a track may repeat) |
| `GET /api/v1/shows`, `GET /api/v1/shows/{show_id}` | list of shows; one with `episodes`, newest first |

Playback state uses the existing route `POST /api/tracks/{track_id}/position`, with `{"seconds": 12.5, "finished": false}` as JSON or `seconds=12.5&finished=false` as a form. It answers 204. Streaming is `GET /api/tracks/{track_id}/stream`. The queue lives in the app.

## Shapes

A track (`kind` is `"music"` or `"episode"`; `position` and `finished` are always there, 0 and false for music):

```json
{
  "id": "…", "title": "…", "kind": "music", "duration": 183,
  "streamUrl": "/api/tracks/…/stream", "imageUrl": "/covers/…?v=1",
  "artists": [{"id": "…", "name": "…"}],
  "showId": "", "position": 0, "finished": false
}
```

Track detail adds:

```json
{ "releases": [ <release> ], "tags": ["…"], "description": "…", "chapters": [{"start": 0, "title": "…"}] }
```

A release (also an item of `releases` lists and of track detail):

```json
{ "id": "…", "title": "…", "type": "album", "date": "2001", "imageUrl": "…", "artists": [{"id": "…", "name": "…"}] }
```

`type` is `album`, `ep`, `single` or `compilation`; `date` is `YYYY`, `YYYY-MM`, `YYYY-MM-DD` or `""`. Release detail adds `"duration"` and `"tracks": [{"disc": 1, "number": 1, "track": <track>}]`. The tracklist wraps each track because a track's own `position` is a playback position.

An artist: `{"id", "name", "imageUrl"}`; detail adds `"releases": [<release>]` and `"tracks": [<track>]`.

A playlist: `{"id", "name", "imageUrl", "count", "duration"}`; detail adds `"tracks": [<track>]`. `count` and `duration` are of the tracks the viewer sees.

A show: `{"id", "title", "author", "imageUrl", "count", "unplayed"}`; detail adds `"episodes": [<track>]`, each with its `position` and `finished`.
