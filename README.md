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
| `AUDIOLOADER_CONTACT` | Contact (email or URL) for the User-Agent MusicBrainz asks for. Defaults to the repository URL. |
| `ACOUSTID_KEY` | AcoustID API key, for identifying tracks by their audio. Also needs `fpcalc` (Chromaprint). |
| `FPCALC` | Path to `fpcalc`, if it isn't on `PATH`. |
| `YTDLP`, `FFMPEG` | Paths to `yt-dlp` and `ffmpeg`, if they aren't on `PATH`. Without them YouTube downloads are off. |
| `AUDIOLOADER_WATCH` | A folder searched for missing tracks (store downloads, free downloads), matched by MusicBrainz ID, ISRC or tags. |
| `AUDIOLOADER_SOURCES` | Download source priority when quality ties, default `folder,yt-dlp`. |
| `AUDIOLOADER_SECRET` | bcrypt hash of the vault passphrase; enables secret tracks. Make one with `./audioloader -hash-secret`. |

Data lives in `database.db` (SQLite) and `data/` (track files, covers, the vault's signing key).

## Development

- `make dev`: live reload with `air`
- `make build`: templ, Tailwind and the binary
- `make test`: Go tests, plus the browser scripts' tests when `node` is installed
