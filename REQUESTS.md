# Requests

Features asked for after the six plans in `docs/plans/` were done. Not implemented yet.

## Auto-download with spotDL

When a release is confirmed from the inbox, download its missing tracks with spotDL automatically if the song can be found.

- **Use the release's credits first.** MusicBrainz releases often link to streaming pages (the "Credits" section: "stream for free", "streaming page", "purchase for download"). Fetch them with `inc=url-rels` on the release lookup. A Spotify album or track link (`open.spotify.com/...`) goes straight to spotDL, with no searching.
- **Fallback:** let spotDL search by artist and title when there's no Spotify link. Accept the result only if its title and length match (the same ±3–5 s rule the playlist mapping uses).
- **Album links:** map the album's tracks onto the release's placeholders the way YouTube playlists are mapped now.
- **Needs:** `spotdl` on PATH (a `SPOTDL` env var for the path), plus ffmpeg. Add a new source adapter in `internal/sources/spotdl`, next to yt-dlp and the watched folder.
- **Note:** spotDL takes metadata from Spotify but downloads the audio from YouTube Music, so its quality is about the same as yt-dlp. It's preferred because the match comes from an exact link.

## Download source order

When a track is downloaded, use the first source that has it, in this order:

1. **My own file:** the watched folder (`AUDIOLOADER_WATCH`), or a manual upload
2. **spotDL**
3. **yt-dlp**

Today sources are ranked by quality (lossless, then bitrate), then by `AUDIOLOADER_SOURCES`. Change this to the fixed order above, with quality only breaking ties within one source. yt-dlp then runs automatically as the last resort too, not only from pasted URLs; it would need a search, such as `ytsearch` or YouTube Music.

## Secret mode

Secret tracks shouldn't mix with normal ones. Unlocking the vault switches the whole app into a separate secret mode, instead of adding secret tracks to the normal pages as it does now.

- **Two modes, never mixed:** normal mode shows only normal tracks (as when locked today). Secret mode shows only secret tracks.
- **Every page follows the mode:** tracks, releases, artists, playlists, podcasts, search, continue listening and counts. A release, artist or show appears in secret mode only if it has secret tracks.
- **Distinct look** in secret mode (another accent color, a "Secret" badge in the nav), so you can tell at a glance.
- **Uploads in secret mode** are secret automatically. That replaces today's "secret" checkboxes; "Make secret / public" in the track menu stays, to move a track between modes.
- **Player:** the queue and saved state belong to one mode. Switching modes clears the queue, and secret tracks never stay in normal mode's state (today's purge on lock already does this).
- **Switching:** unlock at `/vault` to enter secret mode; Lock returns to normal mode.
- **Code:** `db.Viewer` becomes a mode, not "can see secret". `trackCond` filters `secret = 1` in secret mode and `secret = 0` in normal mode, and the derived rules for releases, artists and shows flip the same way. The leak test then runs both ways: no secret track in normal mode, no normal track in secret mode.
- **To decide:** whether background pages (inbox, downloads, tagging) exist in secret mode at all. Secret tracks are never auto-tagged or downloaded, so they could be normal-mode only.

### Secret tracks are a different kind of track

They almost never exist on MusicBrainz and usually have no image, so the music model (releases, covers, tagging) doesn't fit them.

- **No releases:** a secret track doesn't become its own single, and secret mode has no Releases page. A secret track is a title, its creator(s), a length, and optionally a description and tags.
- **No MusicBrainz:** never auto-tagged, never followed or matched in the inbox (mostly already true).
- **No image by default:** secret mode uses its own placeholder image, not the album-cover one. An image can still be uploaded per track.
- **Lists:** organised by creator and by date added, plus tags if they're stored.

### Downloading secret tracks

They come from one particular audio-hosting website, not from MusicBrainz-linked sources.

- **Never name the site in the repo**, not in code, plans, comments, tests or commit messages. It is configured privately, with an environment variable naming the host (e.g. `AUDIOLOADER_SECRET_SOURCE`), and the code and UI only ever call it "the secret source".
- **A secret-mode "Download from URL" form:** paste a page URL from the configured host. The adapter fetches the page, finds the direct audio file URL in it, and downloads it. It takes the title and description from the page as editable defaults, and the track is created secret.
- **Creators are always picked by hand** for each track, using the artist picker (with "add new"). They're never filled in from the page or guessed, and the form can't be submitted without at least one.
- **URL check** like the yt-dlp one: only the configured host is accepted, and only the URL that was checked is ever fetched.
- **Dedupe:** store the page URL as the track's `source_url` (already a column), plus the usual fingerprint check.
- **Tests** use a fake host and fixture page, never the real site.

## Twitch channel on artists

Many artists are Twitch streamers. Link an artist to their Twitch channel to get a profile picture.

- **Field:** an optional Twitch channel per artist (`artists.twitch_login`), set on the artist's page or edit form. Accept a login or a `twitch.tv/<login>` URL, stored as the lowercase login.
- **Automatic only when there's no picture:** when a channel is added and the artist still has the default picture, fetch the channel's profile image and store it as the artist's picture (as covers are stored, in `data/`, not hotlinked).
- **Updating is manual:** an "Update picture from Twitch" button replaces the picture with the channel's current one. Nothing ever overwrites a picture automatically, not even on a schedule.
- **API:** Twitch Helix `GET /helix/users?login=<login>` returns `profile_image_url`. It needs an app (client credentials) token, so add `TWITCH_CLIENT_ID` and `TWITCH_CLIENT_SECRET`. Without them the field can still be saved, and the fetch is off. Cache the token until it expires.
- **Errors:** an unknown channel or a failed fetch shows a message and leaves the picture alone.
- **Depends on:** artist pages and profile pictures (listed under "Not implemented yet" in `FEATURES.md`), plus the cover storage code, reused for artist images.
- **Maybe:** a link to the channel on the artist page.

## Covers: link to the original song

A cover stays credited to the cover artist. The original is only a reference, so you can see who made it.

- **Field per track:** "Cover of" with the original's title and artist(s). Those artists are a reference, not credits: the track isn't listed under them, doesn't count as theirs and doesn't change the track's artists.
- **Three ways to set it,** on the track edit form or the track menu:
  - pick a track already in the library (a link to that track)
  - type the title and artist (plain text, for songs not in the library)
  - paste a MusicBrainz recording URL
- **Shown as** "Cover of *Original* by *Artist*" under the track's title on track rows, release pages and in the player. It links to the original in the library, or to MusicBrainz.
- **Auto-fill from MusicBrainz:** a cover recording has a "cover recording of" relation (seen on release pages, e.g. "Ghosting" → "cover recording of: Ghosting"). With `inc=recording-rels`, tagging can propose it; it's only filled in when the field is empty, never overwriting.
- **Data:** `tracks.original_track_id` (nullable, `ON DELETE SET NULL`), or `original_title`, `original_artists` and `original_mbid` for references outside the library.
- **Maybe:** search finds covers by the original's title or artist, and an artist page lists "Covered by" for their songs.
- **Depends on:** "Edit and delete tracks" from `FEATURES.md`, for a track edit form to set it on.

## Pick the right match from the top 5

### YouTube

When YouTube is the only source left for a missing track (no own file, no spotDL match), don't download a guess. Show the top 5 results and let me pick.

- **Search:** `ytsearch5:"<artist> - <title>"` through `yt-dlp -J --flat-playlist` (no download), or YouTube Music search if it gives better results.
- **Per candidate:** thumbnail, video title, channel, length, and the difference from the expected length. A small badge for official channels and "- Topic" auto-generated channels, which are usually the right audio.
- **Actions:** "Use this" downloads that video into the placeholder (as pasting its URL does now), "Preview" opens the video, and "None of these" leaves it under "Couldn't find".
- **Where:** a "Pick from YouTube" button on Downloads rows, and on release pages for each missing track. Results are fetched when opened, not ahead of time.
- **Never automatic:** with YouTube as the last resort, the download waits in a "needs a pick" state instead of taking result #1.

### MusicBrainz artists

Same idea for linking an artist to MusicBrainz: show the top 5 and let me pick, wherever an artist gets an MBID.

- **Already there:** the Follow page lists MusicBrainz's 5 best name matches. Extend it with more to tell them apart: disambiguation, type, country, active years, and a few release titles (one extra lookup per candidate, only when expanded).
- **Also offer it** on the artist page ("Link to MusicBrainz"), and in tagging when an applied match credits an artist whose MBID isn't set yet and the names are close but not exact.
- **"None of these":** leave the artist unlinked; a pasted MusicBrainz URL still works.
- **Never guess:** an MBID is only set from a pick, an exact embedded tag, or an exact name match in an applied tagging match (as now).

## Better inbox organization

The inbox is one long list of cards. Ideas, to be decided:

- Group by artist, collapsible, with a count per artist
- Filter by type (album / EP / single / compilation) and sort by date
- Tabs for New and Ignored, with a way to un-ignore
- Compact rows instead of large cards, with the tracklist on demand
- Mark how much is owned ("3 of 11 in library") and filter to "nothing owned yet"
- Search within the inbox

## Add an artist from MusicBrainz

Adding an artist now takes typing a name, then linking it to MusicBrainz from the artist page or the Follow page. I want to search MusicBrainz directly and add the artist I pick.

- **One box:** "Add from MusicBrainz" on the artists page. Type a name and see the top 5 artists with disambiguation, type, country and active years, and "Expand" for their newest release groups, like the existing artist picker.
- **Pick:** creates the artist with the MusicBrainz name and ID. Then "Follow" is offered right away, opening the Follow page with that ID chosen.
- **Already there:** if an artist with that ID or name exists, link to it instead of adding a duplicate. A same-name artist without an ID can be linked to the picked ID.
- **Typing a name still works** for artists that aren't on MusicBrainz.
- **Where:** reuse the artist search and candidate list in `internal/mb` and the Follow page's picker, so the 1 req/s limit and the "Expand" lookups stay as they are.
