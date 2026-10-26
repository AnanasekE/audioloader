package mb

import (
	"context"
	"io"
	"net/url"
	"strconv"
)

// inc values are space-separated here: url.Values encodes a space as "+",
// the separator MusicBrainz expects.

// GetRecording looks up a recording with its artists, releases and ISRCs.
func (c *Client) GetRecording(ctx context.Context, id string) (*Recording, error) {
	var r Recording
	q := url.Values{"inc": {"artists releases isrcs release-groups"}}
	return &r, c.getJSON(ctx, "musicbrainz", c.mbURL("recording/"+url.PathEscape(id), q), &r)
}

// GetRelease looks up a release with its tracklist, credits and release group.
func (c *Client) GetRelease(ctx context.Context, id string) (*Release, error) {
	var r Release
	q := url.Values{"inc": {"recordings artist-credits release-groups isrcs url-rels recording-level-rels"}}
	return &r, c.getJSON(ctx, "musicbrainz", c.mbURL("release/"+url.PathEscape(id), q), &r)
}

// SearchRecordings runs a Lucene query against recordings, best first.
func (c *Client) SearchRecordings(ctx context.Context, query string, limit int) ([]Recording, error) {
	var res struct {
		Recordings []Recording `json:"recordings"`
	}
	q := url.Values{"query": {query}, "limit": {strconv.Itoa(limit)}}
	return res.Recordings, c.getJSON(ctx, "musicbrainz", c.mbURL("recording", q), &res)
}

// SearchArtists finds artists by name, best first.
func (c *Client) SearchArtists(ctx context.Context, name string, limit int) ([]Artist, error) {
	var res struct {
		Artists []Artist `json:"artists"`
	}
	q := url.Values{"query": {"artist:" + Phrase(name)}, "limit": {strconv.Itoa(limit)}}
	return res.Artists, c.getJSON(ctx, "musicbrainz", c.mbURL("artist", q), &res)
}

// BrowseReleaseGroups returns one page (up to 100) of an artist's release
// groups and the total count.
func (c *Client) BrowseReleaseGroups(ctx context.Context, artistID string, offset int) ([]ReleaseGroup, int, error) {
	var res struct {
		ReleaseGroups []ReleaseGroup `json:"release-groups"`
		Count         int            `json:"release-group-count"`
	}
	q := url.Values{"artist": {artistID}, "limit": {"100"}, "offset": {strconv.Itoa(offset)}}
	err := c.getJSON(ctx, "musicbrainz", c.mbURL("release-group", q), &res)
	return res.ReleaseGroups, res.Count, err
}

// BrowseReleases returns one page (up to 100) of a release group's releases
// with their tracklists, and the total count.
func (c *Client) BrowseReleases(ctx context.Context, releaseGroupID string, offset int) ([]Release, int, error) {
	var res struct {
		Releases []Release `json:"releases"`
		Count    int       `json:"release-count"`
	}
	q := url.Values{
		"release-group": {releaseGroupID},
		"inc":           {"recordings isrcs artist-credits release-groups url-rels recording-level-rels"},
		"limit":         {"100"},
		"offset":        {strconv.Itoa(offset)},
	}
	err := c.getJSON(ctx, "musicbrainz", c.mbURL("release", q), &res)
	return res.Releases, res.Count, err
}

// AcoustIDResult is one fingerprint match with the recordings linked to it.
type AcoustIDResult struct {
	ID         string  `json:"id"`
	Score      float64 `json:"score"`
	Recordings []struct {
		ID       string  `json:"id"`
		Title    string  `json:"title"`
		Duration float64 `json:"duration"` // seconds
		Artists  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
		ReleaseGroups []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Type  string `json:"type"`
		} `json:"releasegroups"`
	} `json:"recordings"`
}

// AcoustIDLookup matches a Chromaprint fingerprint. It needs AcoustIDKey.
func (c *Client) AcoustIDLookup(ctx context.Context, fingerprint string, durationSec int) ([]AcoustIDResult, error) {
	var res struct {
		Status  string                    `json:"status"`
		Error   *struct{ Message string } `json:"error"`
		Results []AcoustIDResult          `json:"results"`
	}
	q := url.Values{
		"client":      {c.AcoustIDKey},
		"meta":        {"recordings releasegroups"},
		"duration":    {strconv.Itoa(durationSec)},
		"fingerprint": {fingerprint},
	}
	err := c.getJSON(ctx, "acoustid", c.AcoustIDURL+"/lookup?"+q.Encode(), &res)
	if err == nil && res.Error != nil {
		err = errorString("acoustid: " + res.Error.Message)
	}
	return res.Results, err
}

type errorString string

func (e errorString) Error() string { return string(e) }

// FrontCover fetches the 500px front cover of a "release" or
// "release-group" (kind) by MBID. ErrNotFound means there is none.
func (c *Client) FrontCover(ctx context.Context, kind, id string) (io.ReadCloser, string, error) {
	resp, err := c.get(ctx, "coverart", c.CoverArtURL+"/"+kind+"/"+url.PathEscape(id)+"/front-500")
	if err != nil {
		return nil, "", err
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}
