package avatar

import (
	"bytes"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestInitials(t *testing.T) {
	for name, want := range map[string]string{
		"Madonna":        "M",
		"Little V":       "LV",
		"CerberVT":       "C",
		"Hatsune Miku":   "HM",
		"the weeknd x":   "TW",
		"  spaced   out": "SO",
		"Édith Piaf":     "EP",
		"Björk":          "B",
		"Øystein Łukasz": "OL",
		"2Pac":           "2",
		"50 Cent":        "5C",
		"🎵":              "",
		"初音ミク":           "",
		"Hatsune ミク":     "H",
		"":               "",
	} {
		if got := Initials(name); got != want {
			t.Errorf("Initials(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTile(t *testing.T) {
	a, err := Tile("Little V")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Tile("Little V")
	if !bytes.Equal(a, b) {
		t.Error("same name gave different bytes")
	}
	if ColorIndex("Little V") != ColorIndex("  LITTLE V ") {
		t.Error("colour depends on case or spaces")
	}
	img, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 64 {
		t.Errorf("size = %v", b)
	}
	for _, name := range []string{"初音ミク", "", "A", "ZZ"} {
		if _, err := Tile(name); err != nil {
			t.Errorf("Tile(%q): %v", name, err)
		}
	}
}

func luminance(c color.RGBA) float64 {
	f := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

func TestPaletteContrast(t *testing.T) {
	li := luminance(Ink)
	for _, bg := range Palette {
		lb := luminance(bg)
		if r := (li + 0.05) / (lb + 0.05); r < 4.5 {
			t.Errorf("%v: contrast %.2f < 4.5", bg, r)
		}
	}
}

func TestGlyphs(t *testing.T) {
	for r, g := range glyphs {
		for _, row := range g {
			if len(row) != 5 {
				t.Errorf("%q: bad row %q", r, row)
			}
		}
	}
	for r := 'A'; r <= 'Z'; r++ {
		if _, ok := glyphs[r]; !ok {
			t.Errorf("missing %q", r)
		}
	}
}

// TestWriteSamples writes tiles to $AVATAR_SAMPLES for a look by eye.
func TestWriteSamples(t *testing.T) {
	dir := os.Getenv("AVATAR_SAMPLES")
	if dir == "" {
		t.Skip("AVATAR_SAMPLES not set")
	}
	for i, n := range []string{"Little V", "CerberVT", "Hatsune Miku", "初音ミク", "Édith Piaf", "2Pac", "Wu Tang Clan", "Björk"} {
		data, _ := Tile(n)
		os.WriteFile(filepath.Join(dir, "tile"+strconv.Itoa(i)+".png"), data, 0o644)
	}
}
