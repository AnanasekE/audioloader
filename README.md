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
| `AUDIOLOADER_CONTACT` | Contact (email or URL) for the User-Agent MusicBrainz asks for. Defaults to the repository URL. |
| `ACOUSTID_KEY` | AcoustID API key, for identifying tracks by their audio. Also needs `fpcalc` (Chromaprint). |
| `FPCALC` | Path to `fpcalc`, if it isn't on `PATH`. |
| `YTDLP`, `FFMPEG` | Paths to `yt-dlp` and `ffmpeg`, if they aren't on `PATH`. Without them YouTube downloads are off. |
| `SPOTDL` | Path to `spotdl`, if it isn't on `PATH`. Needs `ffmpeg` too. Without it spotDL downloads are off. |
| `AUDIOLOADER_WATCH` | A folder searched for missing tracks (store downloads, free downloads), matched by MusicBrainz ID, ISRC or tags. |
| `AUDIOLOADER_SECRET` | bcrypt hash of the vault passphrase; enables secret tracks. Make one with `./audioloader -hash-secret`. |

Missing tracks are looked up in a fixed order, and the first source that has one wins: your own files (`AUDIOLOADER_WATCH`, or an upload), then spotDL, then YouTube. spotDL uses a Spotify link from MusicBrainz when the release or track has one, else a search whose title and length must match. YouTube is never used on a guess: the top 5 results wait on the Downloads page for you to pick one.

Data lives in `database.db` (SQLite) and `data/` (track files, covers, the vault's signing key).

## Development

- `make dev`: live reload with `air`
- `make build`: templ, Tailwind and the binary
- `make test`: Go tests, plus the browser scripts' tests when `node` is installed
