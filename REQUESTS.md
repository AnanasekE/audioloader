# Requests

Features asked for and not implemented yet. Implemented requests are removed from this file; `FEATURES.md` lists what exists.

## Mobile app (Expo): what's left

The Expo app in `mobile/` browses and plays the library (see `FEATURES.md`). Still open:

- **Offline downloads** of tracks for listening without a connection.
- **Uploading from the phone.**
- **A share extension** in the app, calling `POST /api/share`; until then the Shortcut from plan 20 covers sharing.
- **A standalone (EAS) build** instead of Expo Go.
- **Lock-screen artwork in secret mode:** the lock screen cannot send the vault cookie, so cover images show in normal mode only.

## Queue on phones

The ☰ popover is small; decide whether it becomes a full-screen sheet or part of an expanded player.

## Move YouTube imports onto their MusicBrainz release

A YouTube import (video or playlist) is auto-tagged, but it stays on the release chosen at import (often a single). When the applied match puts the recording on a different MusicBrainz release, for example an album, the track should be able to move there.

- **Offer, don't overwrite:** the "never overwrites what you set" rule stays the default. The Tagging page offers "Move to <release>" on matches whose release differs from the track's, and the track moves only when the user accepts.
- **The target release** is created from MusicBrainz as the Add page does (plan 23), with its artists, IDs and cover, or reused if the library has it. The other tracks of that release are not added; the release shows them as missing, like any partly owned release.
- **The old release** is deleted if the move leaves it empty and it was created by the import (a single named after the video).
- **Open:** whether matches scoring 90%+ should move without asking, as a setting.

## Import library from Apple Music

Bring an Apple Music library into audioloader, as placeholders on their MusicBrainz releases that queue downloads in the usual source order.

- **Favorites:** import the songs marked as favorites (loved).
- **Share a song:** a song shared from Apple Music (its `music.apple.com` link, pasted into the Add box or sent through the share Shortcut) is added like a song search result.
- **Playlists:** import a playlist by its link, adding each song and, optionally, creating the playlist in audioloader.

## missing loop / loop 1 track feature.

- missing looping feature that allows to loop the current song/album

## queue missing features

- unable to add whole album to queue.
- missing dynamic: play next, add after queue

## cover management

- while adding track from youtube cannot pick type cover or set the status of the track as cover.

## Autoplay

- after finishing the queue or a single song an autoplay should enable suggesting tracks from simillar artists or other tracks overall.

## UI

- i can never know where to go to do any kind of action. need a UX redesign.
- UX kinda sucks overall as there is no right click actions with current implementation. might need to rework the application into react. TODO this is an idea / feeling
