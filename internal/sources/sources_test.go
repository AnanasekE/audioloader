package sources

import "testing"

func TestRank(t *testing.T) {
	cands := []Candidate{
		{Source: "yt-dlp", Ref: "yt", Bitrate: 160},
		{Source: "ytmusic", Ref: "ytm", Bitrate: 128},
		{Source: "spotdl", Ref: "sp", Bitrate: 160},
		{Source: "folder", Ref: "mp3", Bitrate: 128},
		{Source: "folder", Ref: "flac", Lossless: true, Bitrate: 900},
		{Source: "yt-dlp", Ref: "yt2", Bitrate: 320},
		{Source: "folder", Ref: "mp3hi", Bitrate: 320},
		{Source: "other", Ref: "x", Lossless: true},
	}
	Rank(cands)
	var got []string
	for _, c := range cands {
		got = append(got, c.Ref)
	}
	// source order first, quality only within a source
	want := []string{"flac", "mp3hi", "mp3", "ytm", "sp", "yt2", "yt", "x"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
