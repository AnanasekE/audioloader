package db

import (
	"strings"
	"testing"

	"audioloader/internal/audio"
)

func TestNewestReleaseCover(t *testing.T) {
	newTestDB(t)
	a, _ := AddArtist(Normal, "Cov")
	if id, err := NewestReleaseCover(Normal, a.ID.String()); err != nil || id != "" {
		t.Fatalf("no releases = %q, %v", id, err)
	}
	mk := func(title, date string, cover bool) string {
		r, err := CreateRelease(title, audio.ReleaseAlbum, date, []string{a.ID.String()})
		if err != nil {
			t.Fatal(err)
		}
		if cover {
			if err := SetReleaseCover(r.ID, strings.NewReader("img"), ".png"); err != nil {
				t.Fatal(err)
			}
		}
		return r.ID.String()
	}
	old := mk("Old", "1999", true)
	mk("Newest without cover", "2024", false)
	if id, _ := NewestReleaseCover(Normal, a.ID.String()); id != old {
		t.Fatalf("want the newest with a cover, got %q", id)
	}
	newer := mk("Newer", "2020", true)
	if id, _ := NewestReleaseCover(Normal, a.ID.String()); id != newer {
		t.Fatalf("want %q, got %q", newer, id)
	}
	if id, _ := NewestReleaseCover(Secret, a.ID.String()); id != "" {
		t.Fatalf("secret mode got %q", id)
	}
}
