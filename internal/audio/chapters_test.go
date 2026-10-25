package audio

import (
	"os"
	"testing"
	"time"
)

func TestReadChapters(t *testing.T) {
	want := []Chapter{{0, "Intro"}, {1500 * time.Millisecond, "Main topic"}, {3 * time.Second, "Outro"}}
	for _, name := range []string{"chapters.mp3", "chapters.m4a"} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			got, err := ReadChapters(f)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatalf("got %+v", got)
			}
			for i := range want {
				// MP4 stores 100 ns units of the AAC timeline; allow a little drift
				if got[i].Title != want[i].Title || (got[i].Start-want[i].Start).Abs() > 50*time.Millisecond {
					t.Fatalf("chapter %d = %+v, want %+v", i, got[i], want[i])
				}
			}
		})
	}
	f, _ := os.Open("../tags/testdata/tagged.flac")
	defer f.Close()
	if got, err := ReadChapters(f); err != nil || len(got) != 0 {
		t.Fatalf("flac: %v, %v", got, err)
	}
}
