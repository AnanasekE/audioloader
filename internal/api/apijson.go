package api

import (
	"strings"

	"audioloader/internal/audio"
	"audioloader/internal/db"
)

// The JSON shapes of the API (see docs/api.md). They are built from audio.*
// types field by field and never encoded directly, so a new field there
// can't leak into the API by accident. IDs are strings, durations and
// positions are seconds, and slices are never null.

type jsonList[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// listOf pages items with ?limit= and ?offset=; Total counts all of them.
func listOf[T any](items []T, limit, offset int) jsonList[T] {
	out := jsonList[T]{Items: []T{}, Total: len(items)}
	if offset < len(items) {
		out.Items = items[offset:min(offset+limit, len(items))]
	}
	return out
}

type jsonArtistRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type jsonTrack struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Kind      string          `json:"kind"` // "music" or "episode"
	Duration  float64         `json:"duration"`
	StreamURL string          `json:"streamUrl"` // "" for a placeholder without a file
	ImageURL  string          `json:"imageUrl"`
	Artists   []jsonArtistRef `json:"artists"`
	ShowID    string          `json:"showId"`   // "" for music
	Position  float64         `json:"position"` // where playback of an episode stopped
	Finished  bool            `json:"finished"`
}

type jsonChapter struct {
	Start float64 `json:"start"`
	Title string  `json:"title"`
}

type jsonTrackDetail struct {
	jsonTrack
	Releases    []jsonRelease `json:"releases"`
	Tags        []string      `json:"tags"`
	Description string        `json:"description"`
	Chapters    []jsonChapter `json:"chapters"`
}

type jsonRelease struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Type     string          `json:"type"`
	Date     string          `json:"date"`
	ImageURL string          `json:"imageUrl"`
	Artists  []jsonArtistRef `json:"artists"`
}

// jsonReleaseTrack is a track on a release's tracklist. It wraps the track,
// since a track's own "position" is a playback position.
type jsonReleaseTrack struct {
	Disc   int       `json:"disc"`
	Number int       `json:"number"`
	Track  jsonTrack `json:"track"`
}

type jsonReleaseDetail struct {
	jsonRelease
	Duration float64            `json:"duration"`
	Tracks   []jsonReleaseTrack `json:"tracks"`
}

type jsonArtist struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
}

// jsonArtistDetail lists the artist's tracks that are on none of their
// releases, as the artist page does; releases are normal mode's only.
type jsonArtistDetail struct {
	jsonArtist
	Releases []jsonRelease `json:"releases"`
	Tracks   []jsonTrack   `json:"tracks"`
}

type jsonPlaylist struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ImageURL string  `json:"imageUrl"`
	Count    int     `json:"count"`
	Duration float64 `json:"duration"`
}

type jsonPlaylistDetail struct {
	jsonPlaylist
	Tracks []jsonTrack `json:"tracks"`
}

type jsonShow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	ImageURL string `json:"imageUrl"`
	Count    int    `json:"count"`
	Unplayed int    `json:"unplayed"`
}

type jsonShowDetail struct {
	jsonShow
	Episodes []jsonTrack `json:"episodes"` // newest first
}

type jsonSearch struct {
	Tracks    []jsonTrack    `json:"tracks"`
	Artists   []jsonArtist   `json:"artists"`
	Releases  []jsonRelease  `json:"releases"`
	Playlists []jsonPlaylist `json:"playlists"`
	Shows     []jsonShow     `json:"shows"`
}

// imageURL is the picture to show for img: "" for the placeholders, which
// the app draws itself. A secret mode viewer never gets a release cover.
func imageURL(v db.Viewer, img string) string {
	if img == db.DefaultCover || img == db.SecretCover {
		return ""
	}
	if v.IsSecret() && strings.HasPrefix(img, "/covers/") {
		return ""
	}
	return img
}

func artistRefs(as []*audio.Artist) []jsonArtistRef {
	out := make([]jsonArtistRef, len(as))
	for i, a := range as {
		out[i] = jsonArtistRef{a.ID.String(), a.Name}
	}
	return out
}

func trackJSON(v db.Viewer, t *audio.Track) jsonTrack {
	j := jsonTrack{
		ID: t.ID.String(), Title: t.Title, Kind: t.Kind, Duration: t.Duration.Seconds(),
		ImageURL: imageURL(v, t.ImgURL), Artists: artistRefs(t.Artists), ShowID: t.ShowID,
		Position: t.Position.Seconds(), Finished: t.Finished,
	}
	if t.HasFile {
		j.StreamURL = "/api/tracks/" + j.ID + "/stream"
	}
	return j
}

func tracksJSON(v db.Viewer, ts []*audio.Track) []jsonTrack {
	out := make([]jsonTrack, len(ts))
	for i, t := range ts {
		out[i] = trackJSON(v, t)
	}
	return out
}

func releaseJSON(v db.Viewer, r *audio.Release) jsonRelease {
	return jsonRelease{
		ID: r.ID.String(), Title: r.Title, Type: string(r.Type), Date: r.Date,
		ImageURL: imageURL(v, r.ImgURL), Artists: artistRefs(r.Artists),
	}
}

func releasesJSON(v db.Viewer, rs []*audio.Release) []jsonRelease {
	out := make([]jsonRelease, len(rs))
	for i, r := range rs {
		out[i] = releaseJSON(v, r)
	}
	return out
}

func artistJSON(v db.Viewer, a *audio.Artist) jsonArtist {
	return jsonArtist{ID: a.ID.String(), Name: a.Name, ImageURL: imageURL(v, a.ImgURL)}
}

func artistsJSON(v db.Viewer, as []*audio.Artist) []jsonArtist {
	out := make([]jsonArtist, len(as))
	for i, a := range as {
		out[i] = artistJSON(v, a)
	}
	return out
}

func playlistJSON(v db.Viewer, p *audio.Playlist) jsonPlaylist {
	return jsonPlaylist{ID: p.ID.String(), Name: p.Name, ImageURL: imageURL(v, p.ImgURL), Count: p.Count, Duration: p.Duration.Seconds()}
}

func playlistsJSON(v db.Viewer, ps []*audio.Playlist) []jsonPlaylist {
	out := make([]jsonPlaylist, len(ps))
	for i, p := range ps {
		out[i] = playlistJSON(v, p)
	}
	return out
}

func showJSON(v db.Viewer, s *audio.Show) jsonShow {
	return jsonShow{ID: s.ID.String(), Title: s.Title, Author: s.Author, ImageURL: imageURL(v, s.ImgURL), Count: s.Count, Unplayed: s.Unplayed}
}

func showsJSON(v db.Viewer, ss []*audio.Show) []jsonShow {
	out := make([]jsonShow, len(ss))
	for i, s := range ss {
		out[i] = showJSON(v, s)
	}
	return out
}
