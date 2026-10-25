package tags

import (
	"os"
	"slices"
	"testing"
)

func TestReadMusicBrainzIDs(t *testing.T) {
	want := Info{
		Title:          "Teardrop",
		RecordingID:    "f3bba4cd-8018-468b-902e-bc8f029593e5",
		ReleaseID:      "0d33ef7a-1f5d-4365-b807-b412271b99c3",
		ReleaseGroupID: "6f9f6899-c0d3-311d-ae87-a10ae6bc53a9",
		ArtistIDs:      []string{"10adbe5e-a2c0-4bf3-8249-2b4cbf6e6ca8"},
		ISRC:           "GBAAA9800322",
	}
	for _, name := range []string{"tagged.mp3", "tagged.flac", "tagged.m4a"} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			got, err := Read(f)
			if err != nil {
				t.Fatal(err)
			}
			if got.Title != want.Title || got.RecordingID != want.RecordingID || got.ReleaseID != want.ReleaseID ||
				got.ReleaseGroupID != want.ReleaseGroupID || !slices.Equal(got.ArtistIDs, want.ArtistIDs) || got.ISRC != want.ISRC {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
