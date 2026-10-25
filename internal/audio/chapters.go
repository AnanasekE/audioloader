package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

// Chapter is a named point in a long track, e.g. a podcast episode.
type Chapter struct {
	Start time.Duration
	Title string
}

// maxChapterTag caps how much of an ID3 tag is read for chapters; cover
// art can make tags large.
const maxChapterTag = 32 << 20

// ReadChapters returns a file's chapters, in order: ID3v2 CHAP frames (MP3)
// or the Nero "chpl" box (MP4/M4A). A file without chapters gives none and
// no error.
func ReadChapters(r io.ReadSeeker) ([]Chapter, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var head [12]byte
	n, err := io.ReadFull(r, head[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	var chapters []Chapter
	switch {
	case n >= 10 && string(head[:3]) == "ID3":
		chapters, err = id3Chapters(r, head[:10])
	case n >= 8 && string(head[4:8]) == "ftyp":
		chapters, err = mp4Chapters(r)
	}
	if err != nil {
		return nil, err
	}
	sort.SliceStable(chapters, func(i, j int) bool { return chapters[i].Start < chapters[j].Start })
	return chapters, nil
}

func synchsafe(b []byte) int {
	return int(b[0])<<21 | int(b[1])<<14 | int(b[2])<<7 | int(b[3])
}

func id3Chapters(r io.ReadSeeker, header []byte) ([]Chapter, error) {
	version := header[3]
	if version < 3 {
		return nil, nil // v2.2 has no CHAP frames
	}
	size := synchsafe(header[6:10])
	if size > maxChapterTag {
		return nil, nil
	}
	if _, err := r.Seek(10, io.SeekStart); err != nil {
		return nil, err
	}
	tag := make([]byte, size)
	if _, err := io.ReadFull(r, tag); err != nil {
		return nil, err
	}
	var chapters []Chapter
	for _, f := range id3Frames(tag, version) {
		if f.id != "CHAP" {
			continue
		}
		// element ID (NUL-terminated), start ms, end ms, start/end offsets
		nul := bytes.IndexByte(f.data, 0)
		if nul < 0 || len(f.data) < nul+1+16 {
			continue
		}
		p := f.data[nul+1:]
		c := Chapter{Start: time.Duration(binary.BigEndian.Uint32(p[0:4])) * time.Millisecond}
		// the title is a TIT2 frame embedded in the CHAP frame
		for _, sub := range id3Frames(p[16:], version) {
			if sub.id == "TIT2" && len(sub.data) > 0 {
				c.Title = id3Text(sub.data[0], sub.data[1:])
			}
		}
		chapters = append(chapters, c)
	}
	return chapters, nil
}

type id3Frame struct {
	id   string
	data []byte
}

// id3Frames splits b into ID3v2.3/2.4 frames, stopping at padding.
func id3Frames(b []byte, version byte) []id3Frame {
	var frames []id3Frame
	for len(b) >= 10 {
		id := string(b[0:4])
		if b[0] == 0 {
			break
		}
		var size int
		if version == 4 {
			size = synchsafe(b[4:8])
		} else {
			size = int(binary.BigEndian.Uint32(b[4:8]))
		}
		if size < 0 || 10+size > len(b) {
			break
		}
		frames = append(frames, id3Frame{id: id, data: b[10 : 10+size]})
		b = b[10+size:]
	}
	return frames
}

// id3Text decodes an ID3 text value in the given encoding.
func id3Text(enc byte, b []byte) string {
	var s string
	switch enc {
	case 0: // ISO-8859-1
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		s = string(r)
	case 1, 2: // UTF-16 with BOM, or big-endian without
		bigEndian := enc == 2
		if len(b) >= 2 && (b[0] == 0xfe && b[1] == 0xff || b[0] == 0xff && b[1] == 0xfe) {
			bigEndian = b[0] == 0xfe
			b = b[2:]
		}
		u := make([]uint16, len(b)/2)
		for i := range u {
			if bigEndian {
				u[i] = binary.BigEndian.Uint16(b[2*i:])
			} else {
				u[i] = binary.LittleEndian.Uint16(b[2*i:])
			}
		}
		s = string(utf16.Decode(u))
	default: // UTF-8
		s = string(b)
	}
	return strings.TrimSpace(strings.TrimRight(s, "\x00"))
}

// mp4Chapters reads Nero chapters from moov/udta/chpl: a full box header,
// four reserved bytes, a count, then per chapter a start in 100 ns units
// and a length-prefixed title.
func mp4Chapters(r io.ReadSeeker) ([]Chapter, error) {
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	moov, moovSize, err := findBox(r, 0, end, "moov")
	if err != nil {
		return nil, nil
	}
	udta, udtaSize, err := findBox(r, moov, moov+moovSize, "udta")
	if err != nil {
		return nil, nil
	}
	chpl, chplSize, err := findBox(r, udta, udta+udtaSize, "chpl")
	if err != nil || chplSize > maxChapterTag {
		return nil, nil
	}
	if _, err := r.Seek(chpl, io.SeekStart); err != nil {
		return nil, err
	}
	b := make([]byte, chplSize)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	if len(b) < 9 {
		return nil, nil
	}
	p := b[8:] // version, flags, reserved
	count := int(p[0])
	p = p[1:]
	var chapters []Chapter
	for i := 0; i < count && len(p) >= 9; i++ {
		start := binary.BigEndian.Uint64(p[0:8])
		n := int(p[8])
		if len(p) < 9+n {
			break
		}
		chapters = append(chapters, Chapter{
			Start: time.Duration(start * 100),
			Title: strings.TrimSpace(string(p[9 : 9+n])),
		})
		p = p[9+n:]
	}
	return chapters, nil
}
