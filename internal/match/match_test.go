package match

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Björk":                   "bjork",
		"  Song (feat. Someone) ": "song",
		"Song ft. X":              "song",
		"Rock & Roll!":            "rock and roll",
		"Don't Stop":              "don t stop",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	if s := Similarity("Teardrop", "teardrop"); s != 1 {
		t.Fatalf("equal = %v", s)
	}
	if s := Similarity("Teardrop", "Teardrops"); s < 0.85 {
		t.Fatalf("close = %v", s)
	}
	if s := Similarity("Teardrop", "Angel"); s > 0.3 {
		t.Fatalf("different = %v", s)
	}
}

func TestClose(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Massive Attack", "massive  attack", true},
		{"Massive Attack", "Massive Attak", true},
		{"Beyonce", "Beyoncé", true},
		{"Massive Attack", "Massive", false},
		{"Eve", "Eva", false},
		{"Blur", "Bush", false},
		{"", "", false},
		{"!!!", "???", false},
	} {
		if got := Close(c.a, c.b); got != c.want {
			t.Errorf("Close(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
