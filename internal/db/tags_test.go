package db

import (
	"bytes"
	"strings"
	"testing"

	"audioloader/internal/audio"
)

func seedTagged(t *testing.T) {
	t.Helper()
	newTestDB(t)
	a, _ := AddArtist(Secret, "Zed")
	b, _ := AddArtist(Secret, "Amy")
	for _, c := range []struct {
		title, artist string
		tags          []string
	}{
		{"One", a.ID.String(), []string{"ambient", "Rain"}},
		{"Two", b.ID.String(), []string{"ambient", "lofi"}},
		{"Three", a.ID.String(), []string{"rain"}},
		{"Four", b.ID.String(), nil},
	} {
		if _, err := AddTrack(Secret, []string{c.artist}, &audio.Track{Title: c.title, Tags: c.tags}, bytes.NewReader([]byte(c.title)), ".mp3", nil, ""); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTrackTagsQueries(t *testing.T) {
	seedTagged(t)
	titlesFor := func(q TagQuery) string {
		tr, err := TracksByTags(Secret, q)
		if err != nil {
			t.Fatal(err)
		}
		return titles(tr)
	}
	cases := []struct {
		name string
		q    TagQuery
		want []string
	}{
		{"empty is everything", TagQuery{}, []string{"One", "Two", "Three", "Four"}},
		{"all-of", TagQuery{Include: []string{"ambient", "rain"}}, []string{"One"}},
		{"any-of", TagQuery{Include: []string{"lofi", "rain"}, Any: true}, []string{"One", "Two", "Three"}},
		{"exclude", TagQuery{Include: []string{"ambient"}, Exclude: []string{"lofi"}}, []string{"One"}},
		{"exclude alone", TagQuery{Exclude: []string{"ambient"}}, []string{"Three", "Four"}},
		{"case-insensitive", TagQuery{Include: []string{"RAIN"}}, []string{"One", "Three"}},
		{"no match", TagQuery{Include: []string{"nothing"}}, nil},
	}
	for _, c := range cases {
		tr, _ := TracksByTags(Secret, c.q)
		if len(tr) != len(c.want) {
			t.Errorf("%s: got %s, want %v", c.name, titlesFor(c.q), c.want)
			continue
		}
		got := map[string]bool{}
		for _, x := range tr {
			got[x.Title] = true
		}
		for _, w := range c.want {
			if !got[w] {
				t.Errorf("%s: missing %s in %s", c.name, w, titlesFor(c.q))
			}
		}
	}
}

func TestTrackTagsCounts(t *testing.T) {
	seedTagged(t)
	counts, err := TrackTags(Secret, TagQuery{})
	if err != nil {
		t.Fatal(err)
	}
	lower := map[string]int{}
	for k, n := range counts {
		lower[strings.ToLower(k)] += n
	}
	if len(counts) != 3 || lower["ambient"] != 2 || lower["rain"] != 2 || lower["lofi"] != 1 {
		t.Errorf("all counts = %v", counts)
	}
	counts, _ = TrackTags(Secret, TagQuery{Include: []string{"ambient"}})
	lower = map[string]int{}
	for k, n := range counts {
		lower[strings.ToLower(k)] += n
	}
	if lower["ambient"] != 2 || lower["rain"] != 1 || lower["lofi"] != 1 {
		t.Errorf("counts within ambient = %v", counts)
	}
}

func TestTrackTagsSort(t *testing.T) {
	seedTagged(t)
	tr, _ := TracksByTags(Secret, TagQuery{Sort: "creator"})
	if len(tr) != 4 || tr[0].Artists[0].Name != "Amy" || tr[3].Artists[0].Name != "Zed" {
		t.Errorf("by creator: %s", titles(tr))
	}
	tr, _ = TracksByTags(Secret, TagQuery{Sort: "tag"})
	if len(tr) != 4 || tr[3].Title != "Four" {
		t.Errorf("by tag, untagged last: %s", titles(tr))
	}
}

func TestTrackTagsDoNotLeakAcrossModes(t *testing.T) {
	seedTagged(t)
	n := addTestTrack(t, "Public", "pub")
	// tags are secret-only in the app, but even a stray row mustn't cross over
	if _, err := DB.Exec(`INSERT INTO track_tags(track_id, tag) VALUES(?, 'ambient'), (?, 'public-only');`, n, n); err != nil {
		t.Fatal(err)
	}
	counts, _ := TrackTags(Secret, TagQuery{})
	if _, ok := counts["public-only"]; ok || counts["ambient"] != 2 {
		t.Errorf("secret counts = %v", counts)
	}
	tr, _ := TracksByTags(Secret, TagQuery{Include: []string{"ambient"}})
	for _, x := range tr {
		if x.Title == "Public" {
			t.Error("a normal track showed up in secret mode")
		}
	}
	if tr, _ := TracksByTags(Normal, TagQuery{Include: []string{"ambient"}}); len(tr) != 1 || tr[0].Title != "Public" {
		t.Errorf("normal sees %s", titles(tr))
	}
}
