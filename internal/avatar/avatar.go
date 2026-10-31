// Package avatar draws the pixel-art initials tile shown for artists
// without a picture.
package avatar

import (
	"bytes"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	// Size is the width and height of a tile in pixels.
	Size = 64
	// block is the size of one glyph pixel.
	block = 4
	note  = '♪'
)

// Ink is the colour of the letters.
var Ink = color.RGBA{0xf4, 0xf5, 0xf7, 0xff}

// Palette holds the muted backgrounds, hues from the Console palette in
// styles/input.css. Each one is dark enough for Ink to reach 4.5:1.
var Palette = []color.RGBA{
	{0x1b, 0x6e, 0x4d, 0xff}, // green
	{0x2a, 0x5f, 0x99, 0xff}, // blue
	{0x8a, 0x5a, 0x12, 0xff}, // amber
	{0xa3, 0x3a, 0x3a, 0xff}, // red
	{0x6b, 0x4a, 0xb0, 0xff}, // purple
	{0x1f, 0x6b, 0x70, 0xff}, // teal
	{0x9a, 0x3f, 0x7a, 0xff}, // pink
	{0x4d, 0x56, 0x63, 0xff}, // slate
}

// foldExtra maps letters that don't decompose into a base letter and mark.
var foldExtra = map[rune]rune{'ø': 'O', 'Ø': 'O', 'ł': 'L', 'Ł': 'L', 'đ': 'D', 'Đ': 'D', 'æ': 'A', 'Æ': 'A', 'ß': 'S', 'œ': 'O', 'Œ': 'O'}

// fold returns the uppercase ASCII letter or digit r stands for, or 0.
func fold(r rune) rune {
	if r > unicode.MaxASCII {
		if f, ok := foldExtra[r]; ok {
			return f
		}
		// é decomposes to e plus a combining mark
		for _, d := range norm.NFD.String(string(r)) {
			r = d
			break
		}
	}
	r = unicode.ToUpper(r)
	if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
		return r
	}
	return 0
}

// Initials returns the first usable character of each of the first two
// words of name, uppercased and without accents. It is "" when name has no
// letter or digit the font can draw.
func Initials(name string) string {
	var out []rune
	for _, word := range strings.Fields(name) {
		for _, r := range word {
			if f := fold(r); f != 0 {
				out = append(out, f)
				break
			}
		}
		if len(out) == 2 {
			break
		}
	}
	return string(out)
}

// ColorIndex picks the palette entry for name: same name, same colour.
func ColorIndex(name string) int {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	return int(h.Sum32() % uint32(len(Palette)))
}

// Tile returns a 64x64 PNG with the initials of name on a colour picked from it.
func Tile(name string) ([]byte, error) {
	text := []rune(Initials(name))
	if len(text) == 0 {
		text = []rune{note}
	}
	bg := Palette[ColorIndex(name)]
	img := image.NewRGBA(image.Rect(0, 0, Size, Size))
	fill(img, image.Rect(0, 0, Size, Size), bg)

	cols := len(text)*5 + len(text) - 1
	x0 := (Size - cols*block) / 2
	y0 := (Size - 7*block) / 2
	for i, r := range text {
		g := glyphs[r]
		for y, row := range g {
			for x, c := range row {
				if c != '#' {
					continue
				}
				px := x0 + (i*6+x)*block
				py := y0 + y*block
				fill(img, image.Rect(px, py, px+block, py+block), Ink)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}
