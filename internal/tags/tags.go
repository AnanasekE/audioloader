// Package tags reads the embedded tags of stored audio files on the server:
// the basic fields and the MusicBrainz IDs that Picard writes. Uploads read
// tags in the browser (assets/js/tags.js); this is for files already stored.
package tags

import (
	"io"
	"regexp"
	"strings"

	"github.com/dhowden/tag"
)

// Info is what a file's tags say about it. Missing values are empty.
type Info struct {
	Title  string
	Artist string
	Album  string

	RecordingID    string // MusicBrainz recording ID
	ReleaseID      string
	ReleaseGroupID string
	ArtistIDs      []string
	ISRC           string
}

var (
	mbidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	isrcPattern = regexp.MustCompile(`[A-Z]{2}-?[A-Z0-9]{3}-?[0-9]{2}-?[0-9]{5}`)
)

// Read parses r's tags. A file without tags gives an empty Info and no error.
func Read(r io.ReadSeeker) (Info, error) {
	m, err := tag.ReadFrom(r)
	if err == tag.ErrNoTagsFound {
		return Info{}, nil
	}
	if err != nil {
		return Info{}, err
	}
	info := Info{Title: m.Title(), Artist: m.Artist(), Album: m.Album()}
	for key, v := range m.Raw() {
		switch v := v.(type) {
		case *tag.UFID:
			// ID3 keeps the recording ID in a UFID frame owned by MusicBrainz
			if v.Provider == "http://musicbrainz.org" {
				set(&info.RecordingID, firstMBID(string(v.Identifier)))
			}
		case *tag.Comm:
			// ID3 TXXX frames are keyed TXXX, TXXX_0, ...; the name is the description
			if strings.HasPrefix(key, "TXXX") {
				info.set(v.Description, v.Text)
			}
		case string:
			if key == "TSRC" {
				info.set("isrc", v)
			} else {
				info.set(key, v)
			}
		}
	}
	return info, nil
}

// set stores value if name is one of the MusicBrainz or ISRC tags. Names
// come in the formats' spellings: "MusicBrainz Album Id" (ID3, MP4) or
// "musicbrainz_albumid" (Vorbis).
func (info *Info) set(name, value string) {
	n := strings.ToLower(strings.ReplaceAll(name, "_", " "))
	switch n {
	// Vorbis and MP4 call the recording ID "track id"
	case "musicbrainz track id", "musicbrainz trackid":
		set(&info.RecordingID, firstMBID(value))
	case "musicbrainz album id", "musicbrainz albumid":
		set(&info.ReleaseID, firstMBID(value))
	case "musicbrainz release group id", "musicbrainz releasegroupid":
		set(&info.ReleaseGroupID, firstMBID(value))
	case "musicbrainz artist id", "musicbrainz artistid":
		if len(info.ArtistIDs) == 0 {
			for _, id := range mbidPattern.FindAllString(value, -1) {
				info.ArtistIDs = append(info.ArtistIDs, strings.ToLower(id))
			}
		}
	case "isrc":
		set(&info.ISRC, strings.ReplaceAll(isrcPattern.FindString(strings.ToUpper(value)), "-", ""))
	}
}

func set(dst *string, v string) {
	if *dst == "" {
		*dst = v
	}
}

func firstMBID(s string) string {
	return strings.ToLower(mbidPattern.FindString(s))
}
