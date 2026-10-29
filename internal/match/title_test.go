package match

import "testing"

func TestCleanTitle(t *testing.T) {
	cases := map[string]string{
		"Massive Attack - Teardrop (Official Video)": "Teardrop",
		"Teardrop [Official Audio]":                  "Teardrop",
		"Massive Attack – Angel (Lyrics) (HD)":       "Angel",
		"Someone Else - Teardrop":                    "Someone Else - Teardrop",
		"Teardrop (Remastered 2019)":                 "Teardrop (Remastered 2019)",
	}
	for in, want := range cases {
		if got := CleanTitle(in, []string{"Massive Attack"}); got != want {
			t.Errorf("CleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTitlePrefix(t *testing.T) {
	cases := map[string]string{
		"Massive Attack - Teardrop (Official Video)": "Massive Attack",
		"Teardrop":                                   "",
		"Teardrop (Official Audio)":                  "",
	}
	for in, want := range cases {
		if got := TitlePrefix(in); got != want {
			t.Errorf("TitlePrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTitleScore(t *testing.T) {
	artists := []string{"Massive Attack"}
	cases := []struct {
		candidate, title string
		min, max         float64
	}{
		{"Teardrop", "Teardrop", 1, 1},
		{"Massive Attack - Teardrop (Official Video)", "Teardrop", 1, 1},
		{"Teardrop (Remastered 2019)", "Teardrop", 0.9, 1},
		{"Teardrop (feat. Elizabeth Fraser)", "Teardrop", 0.9, 1},
		{"Angel", "Teardrop", 0, 0.6},
	}
	for _, c := range cases {
		if got := TitleScore(c.candidate, c.title, artists); got < c.min || got > c.max {
			t.Errorf("TitleScore(%q, %q) = %.2f, want %.2f..%.2f", c.candidate, c.title, got, c.min, c.max)
		}
	}
}
