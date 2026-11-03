# UX audit (plan 37)

Every page walked in normal and secret mode at 1440×900 and 390×844, with `tools/shots.sh` (headless Firefox, the seeded demo data). The screenshots are `tmp/shots/<page>-1440.png` and `-390.png`, e.g. `tmp/shots/release-390.png`; they are not committed, `SHOTS_RESEED=1 tools/shots.sh` makes them again.

Ratings: **quick** (a label, a link, an empty state; done in plan 37 step 4), **medium** (a small feature or a page change), **large** (a new area or a redesign). Medium and large items are in `REQUESTS.md` under UI.

The context menus (right-click, long-press, ⋯ on every card, row and header) and the search box actions (`Ctrl K`, then `>`) came first and answer the main complaint: the actions of a thing are reachable from the thing. What is left is below.

## Pages

| Page (screenshot) | What I come to do | Is the way visible? | Fix | Rating |
|---|---|---|---|---|
| Tracks (`tracks`) | Find a song and play it; edit or queue it | Filter and Add are in the header. Edit, queue, playlist, trim and delete were behind a small ⋯ only | Right-click menu on the row (done in step 1) | done |
| | Sort by title, artist, date added | No way | A sort control like the secret Tracks page has | medium |
| Add (`add`) | Add a song from a link or search, or upload | Yes: one box first, upload forms below | none | |
| Releases (`releases`) | Open or play an album, make a release | Cards link in; "New release" in the header. Play/queue/edit/remove were only on the release page | ⋯ menu on cards and single rows (done in step 2) | done |
| | Find a release by name | No filter, unlike Tracks | Filter box on the card grids | medium |
| | Empty library | "No releases yet." without a next step | Say how: New release, or add a song, with a link | quick, done |
| Release (`release`, `release-single`) | Play it, edit it, fix the tracklist | Play, Shuffle, Edit release, Remove visible. Edit tracklist only by finding it on the edit page; the queue ⋯ was only for queueing | One ⋯ menu with everything (done in step 2) | done |
| | Same on a phone | The cover sat at the bottom of the header while the text started at the top | Align the header to the top on phones | quick, done |
| Release edit (`release-edit`) | Change title, cover, tracklist | Yes, but the page has three forms and a danger zone; "Cancel" is the only way out | none for now | |
| Artists (`artists`) | Open an artist; follow one | Cards link in; the actions (follow, rename, link) were two clicks deep on the artist page | ⋯ menu on the cards (done in step 2) | done |
| | Find an artist by name | No filter, only search | Filter box on the grid | medium |
| | Empty library | "No artists yet." | Say how, with a link to Add | quick, done |
| Artist (`artist`) | Play everything; follow; rename; link to MusicBrainz | Follow and Batch upload visible; rename and link hide in "Manage" at the bottom, which nothing points to | ⋯ menu with Play all and Edit / rename that opens Manage (done in step 2) | done |
| | An artist with nothing yet | "Nothing by this artist yet." | Links to upload tracks or search their songs | quick, done |
| Follow (`follow`) | Choose which releases reach the inbox | Clear: "Get new releases in your inbox", Save, Check now | none | |
| Playlists (`playlists`) | Open or play a playlist; make one | "New playlist" in the header; actions only inside the playlist | ⋯ menu on cards (done in step 2) | done |
| Playlist (`playlist`, `playlist-edit`) | Play, reorder, rename, delete | Play, Shuffle, Edit visible; Delete only at the bottom of the edit page | ⋯ menu with Delete (done in step 2) | done |
| | Reorder by dragging | Up / down buttons only | Drag and drop | large |
| Podcasts (`shows`, `show`) | Continue an episode; add one | "Upload episode" on the list; the show page had no action and its empty state no link | Upload episode on the show page, links in both empty states | quick, done |
| Inbox (`inbox`) | Confirm what I want, ignore the rest | Yes. Two buttons both read "Confirm all", one for the page, one per artist; artist names were not links | "Confirm all 5" and "Confirm these 2", artist names link to their page | quick, done |
| Downloads (`downloads`) | See what is stuck and fix it | Yes: status chips, Pick from YouTube, Retry | Rows do not link to the release or artist | medium |
| Tagging (`tagging`) | Review matches | Yes. The empty state did not say what feeds it | One line about Auto-tag library | quick, done |
| Search (`search-q-low`) | Find anything | Results grouped by kind; "Add as a song" when missing | A type filter, and recent searches | medium |
| Vault (`vault`) | Unlock secret mode | Lock icon on phones, Vault in the sidebar | none | |
| Secret Tracks / Tags / Creators (`secret-tracks`, `secret-tags`, `secret-artists`) | Browse by tag or creator | Sort tabs, tag chips; menus now match the normal mode | none | |
| Secret Add (`secret-add`) | Upload a track | Without a secret source the page starts with an "Or upload a file" divider above nothing | Kept: a test (`TestAddPageSecretWithoutSource`) asserts it | |

## Across pages

- **Phone: Playlists, Podcasts, Tagging and Downloads are behind "More".** A user who has never opened it does not know they exist, and `Ctrl K` is not there. A search-box action is the same one tap away, but the box itself is small. Show them on the home screen, or let the box hint "Go to…". Medium.
- **No start page.** `/` opens Tracks. A home with continue listening, recently added and the inbox count would answer "where do I go". Large.
- **Edit is always a page.** Releases, playlists and tracks open a separate edit page and a way back is "Cancel". Editing in place, or in a side panel, would keep the user in context. Large.
- **The ⋯ button is small and, on cards, shown only on hover on wide screens.** The right-click menu and the shortcuts dialog (`?`) help, but nothing tells a new user to right-click. A one-time hint in the shortcuts dialog is there; a visible "Actions" hint is not. Quick if wanted, left out.
