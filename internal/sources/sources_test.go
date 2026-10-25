package sources

import "testing"

func TestRank(t *testing.T) {
	cands := []Candidate{
		{Source: "yt-dlp", Ref: "yt", Bitrate: 160},
		{Source: "folder", Ref: "mp3", Bitrate: 320},
		{Source: "folder", Ref: "flac", Lossless: true, Bitrate: 900},
		{Source: "yt-dlp", Ref: "yt2", Bitrate: 320},
	}
	Rank(cands, []string{"folder", "yt-dlp"})
	got := []string{cands[0].Ref, cands[1].Ref, cands[2].Ref, cands[3].Ref}
	want := []string{"flac", "mp3", "yt2", "yt"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
