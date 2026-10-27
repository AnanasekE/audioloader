# Audio management app

## Notes

```txt
GET /api/tracks/{track_uuid}/stream - streaming a song / live transcoding if needed

```

for now file storage is just a file system with a database for metadata
like the file uuid and the original title

## Functionality

- Download songs
- Playback
- Playlists
- Albums
- Singles
- Artists

Optional / Future

- Transcoding

## Configuration

All optional; set as environment variables.

| Variable | What it does |
|---|---|
| `AUDIOLOADER_ADDR` | Address to listen on, default `:8080`. |
| `AUDIOLOADER_DB` | Path of the SQLite database, default `./database.db`. |
| `AUDIOLOADER_CONTACT` | Contact (email or URL) for the User-Agent MusicBrainz asks for. Defaults to the repository URL. |
| `ACOUSTID_KEY` | AcoustID API key, for identifying tracks by their audio. Also needs `fpcalc` (Chromaprint). |
| `TWITCH_CLIENT_ID`, `TWITCH_CLIENT_SECRET` | Credentials of a Twitch app (dev.twitch.tv, client credentials), for fetching artists' channel pictures. Both are needed; without them a Twitch channel can still be saved but no picture is fetched. |
| `FPCALC` | Path to `fpcalc`, if it isn't on `PATH`. |
| `YTDLP`, `FFMPEG` | Paths to `yt-dlp` and `ffmpeg`, if they aren't on `PATH`. Without them YouTube downloads are off. |
| `SPOTDL` | Path to `spotdl`, if it isn't on `PATH`. Needs `ffmpeg` too. Without it spotDL downloads are off. |
| `AUDIOLOADER_WATCH` | A folder searched for missing tracks (store downloads, free downloads), matched by MusicBrainz ID, ISRC or tags. |
| `AUDIOLOADER_SECRET` | bcrypt hash of the vault passphrase; enables secret tracks. Make one with `./audioloader -hash-secret`. |
| `AUDIOLOADER_SECRET_SOURCE` | The host of the pages the secret source downloads from, as a bare host name. Only `https` page URLs on exactly this host are accepted. Without it, "Download from URL" in secret mode is off. |
| `AUDIOLOADER_SECRET_SOURCE_MEDIA` | Comma-separated hosts the audio files may be fetched from, if they aren't on the page host. Redirects to any other host are refused. |

Missing tracks are looked up in a fixed order, and the first source that has one wins: your own files (`AUDIOLOADER_WATCH`, or an upload), then spotDL, then YouTube. spotDL uses a Spotify link from MusicBrainz when the release or track has one, else a search whose title and length must match. YouTube is never used on a guess: the top 5 results wait on the Downloads page for you to pick one.

Data lives in `database.db` (SQLite) and `data/` (track files, covers, pictures, the vault's signing key). Before a migration that deletes data, the app copies the database to `database.db.bak-<migration number>` (once) and logs what it removes.

## Docker

```sh
docker compose up -d --build
```

The app is served at http://localhost:8080. The database and all files are stored in `./data`, mounted at `/app/data`. Set environment variables in a `.env` file next to `compose.yaml`. The image includes ffmpeg, yt-dlp, spotDL and fpcalc. The build uses the committed templ output and `app.css`, so run `make generate css` before you build if you changed them.

To move an existing library into the container, copy `database.db` into `data/`, after you stop the app.

## Development

- `make dev`: live reload with `air`
- `make build`: templ, Tailwind and the binary
- `make test`: Go tests, plus the browser scripts' tests when `node` is installed
