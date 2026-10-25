// Package audio provides helpers for inspecting audio files.
// NOTE: VIBED
package audio

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// ErrUnknownFormat is returned when the input is not a supported audio format.
var ErrUnknownFormat = errors.New("audio: unknown format")

// ProbeDuration reads the file's headers and returns the playback duration.
// Supports WAV (RIFF/WAVE), MP3 (MPEG-1/2/2.5 Layer III), FLAC, Ogg Opus
// and MP4/M4A.
//
// The format is detected from the leading magic bytes. r is rewound to the
// start before probing; its final position is unspecified. If the format is
// not recognised, ErrUnknownFormat is returned.
func ProbeDuration(r io.ReadSeeker) (time.Duration, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	var magic [12]byte
	n, err := io.ReadFull(r, magic[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return 0, err
	}

	// FLAC files may also start with an ID3v2 tag, so peek past it
	if n >= 10 && string(magic[0:3]) == "ID3" {
		off := int64(10 + id3Size(magic[:10]))
		if _, err := r.Seek(off, io.SeekStart); err != nil {
			return 0, err
		}
		var m [4]byte
		if _, err := io.ReadFull(r, m[:]); err == nil && string(m[:]) == "fLaC" {
			return probeFLAC(r, off)
		}
	}

	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	switch {
	case n >= 12 && string(magic[0:4]) == "RIFF" && string(magic[8:12]) == "WAVE":
		return probeWAV(r)
	case n >= 4 && string(magic[0:4]) == "fLaC":
		return probeFLAC(r, 0)
	case n >= 4 && string(magic[0:4]) == "OggS":
		return probeOpus(r)
	case n >= 8 && string(magic[4:8]) == "ftyp":
		return probeMP4(r)
	case n >= 3 && string(magic[0:3]) == "ID3",
		n >= 2 && magic[0] == 0xFF && magic[1]&0xE0 == 0xE0:
		return probeMP3(r)
	}
	return 0, ErrUnknownFormat
}

// id3Size returns the ID3v2 tag size (excluding the 10-byte header) from its header.
// head must hold at least the 10-byte ID3v2 header. The size is stored as a
// 28-bit synchsafe integer (7 bits per byte); a footer adds another 10 bytes.
func id3Size(head []byte) int {
	size := int(head[6]&0x7F)<<21 | int(head[7]&0x7F)<<14 | int(head[8]&0x7F)<<7 | int(head[9]&0x7F)
	if head[5]&0x10 != 0 {
		size += 10
	}
	return size
}

// probeWAV walks the RIFF chunks after the 12-byte RIFF/WAVE header and
// returns the duration as the "data" chunk size divided by the byte rate
// from the "fmt " chunk. Unknown chunks are skipped, honouring the RIFF
// rule that odd-sized chunks are followed by a pad byte.
func probeWAV(r io.ReadSeeker) (time.Duration, error) {
	if _, err := r.Seek(12, io.SeekStart); err != nil {
		return 0, err
	}

	var byteRate uint32
	var hdr [8]byte
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return 0, fmt.Errorf("wav: missing data chunk: %w", err)
		}
		id := string(hdr[0:4])
		size := binary.LittleEndian.Uint32(hdr[4:8])

		switch id {
		case "fmt ":
			var f [16]byte
			if size < 16 {
				return 0, errors.New("wav: fmt chunk too small")
			}
			if _, err := io.ReadFull(r, f[:]); err != nil {
				return 0, err
			}
			byteRate = binary.LittleEndian.Uint32(f[8:12])
			if _, err := r.Seek(int64(size-16)+int64(size&1), io.SeekCurrent); err != nil {
				return 0, err
			}
		case "data":
			if byteRate == 0 {
				return 0, errors.New("wav: data before fmt or zero byte rate")
			}
			return time.Duration(float64(size) / float64(byteRate) * float64(time.Second)), nil
		default:
			if _, err := r.Seek(int64(size)+int64(size&1), io.SeekCurrent); err != nil {
				return 0, err
			}
		}
	}
}

// probeFLAC reads the STREAMINFO block; off is where the "fLaC" marker starts.
// The duration is total samples divided by sample rate. Streams that do not
// record a total sample count (value 0) return an error.
func probeFLAC(r io.ReadSeeker, off int64) (time.Duration, error) {
	if _, err := r.Seek(off+4, io.SeekStart); err != nil {
		return 0, err
	}

	// STREAMINFO is always the first metadata block
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, fmt.Errorf("flac: missing STREAMINFO: %w", err)
	}
	if hdr[0]&0x7F != 0 {
		return 0, errors.New("flac: first block is not STREAMINFO")
	}
	length := int(hdr[1])<<16 | int(hdr[2])<<8 | int(hdr[3])
	if length < 34 {
		return 0, errors.New("flac: STREAMINFO too small")
	}

	var si [34]byte
	if _, err := io.ReadFull(r, si[:]); err != nil {
		return 0, err
	}
	// bytes 10..17: sample rate (20 bits), channels (3), bps (5), total samples (36)
	sampleRate := uint64(si[10])<<12 | uint64(si[11])<<4 | uint64(si[12])>>4
	totalSamples := uint64(si[13]&0x0F)<<32 | uint64(binary.BigEndian.Uint32(si[14:18]))

	if sampleRate == 0 {
		return 0, errors.New("flac: zero sample rate")
	}
	if totalSamples == 0 {
		return 0, errors.New("flac: unknown total samples")
	}
	return time.Duration(totalSamples * uint64(time.Second) / sampleRate), nil
}

// opusRate is the fixed granule rate of Ogg Opus, regardless of input sample rate.
const opusRate = 48000

// probeOpus reads the OpusHead packet from the first Ogg page to get the
// stream serial and pre-skip, then scans backwards from the end of the file
// for the last page of that stream. The duration is its granule position
// minus pre-skip, at the fixed 48 kHz opusRate. Non-Opus Ogg streams
// (Vorbis, FLAC-in-Ogg, ...) return ErrUnknownFormat.
func probeOpus(r io.ReadSeeker) (time.Duration, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	// first page: 27-byte header, segment table, then the OpusHead packet
	var page [27]byte
	if _, err := io.ReadFull(r, page[:]); err != nil {
		return 0, fmt.Errorf("ogg: short first page: %w", err)
	}
	serial := binary.LittleEndian.Uint32(page[14:18])
	if _, err := r.Seek(int64(page[26]), io.SeekCurrent); err != nil {
		return 0, err
	}
	var head [19]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, fmt.Errorf("ogg: short OpusHead: %w", err)
	}
	if string(head[0:8]) != "OpusHead" {
		return 0, ErrUnknownFormat
	}
	preSkip := int64(binary.LittleEndian.Uint16(head[10:12]))

	// last page holds the final granule position; a page is at most 65307 bytes
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	tailLen := int64(65307)
	if tailLen > end {
		tailLen = end
	}
	if _, err := r.Seek(end-tailLen, io.SeekStart); err != nil {
		return 0, err
	}
	tail := make([]byte, tailLen)
	if _, err := io.ReadFull(r, tail); err != nil {
		return 0, err
	}

	for i := bytes.LastIndex(tail, []byte("OggS")); i >= 0; i = bytes.LastIndex(tail[:i], []byte("OggS")) {
		if i+27 > len(tail) {
			continue
		}
		granule := int64(binary.LittleEndian.Uint64(tail[i+6 : i+14]))
		if binary.LittleEndian.Uint32(tail[i+14:i+18]) != serial || granule < 0 {
			continue
		}
		samples := granule - preSkip
		if samples < 0 {
			samples = 0
		}
		return time.Duration(samples * int64(time.Second) / opusRate), nil
	}
	return 0, errors.New("ogg: no final page found")
}

// Lookup tables for decoding MPEG audio frame headers.
var (
	// bitrates in kbps, [lsf][index]; lsf=0 MPEG-1, lsf=1 MPEG-2/2.5 (Layer III)
	mp3Bitrates = [2][16]int{
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
	}
	// sample rates, [version bits][index]; version bits: 0=2.5, 2=2, 3=1
	mp3SampleRates = [4][3]int{
		{11025, 12000, 8000},
		{0, 0, 0},
		{22050, 24000, 16000},
		{44100, 48000, 32000},
	}
)

// probeMP3 skips any leading ID3v2 tag, then walks every MPEG Layer III
// frame in the stream, summing samples per frame. Invalid headers and
// trailing junk (e.g. an ID3v1 tag) are skipped by resyncing one byte at a
// time. This handles both CBR and VBR files, at the cost of reading the
// whole stream.
func probeMP3(r io.ReadSeeker) (time.Duration, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	br := bufio.NewReader(r)

	if head, err := br.Peek(10); err == nil && bytes.Equal(head[0:3], []byte("ID3")) {
		if _, err := br.Discard(10 + id3Size(head)); err != nil {
			return 0, err
		}
	}

	var totalSamples, frames int64
	var sampleRate int
	for {
		h, err := br.Peek(4)
		if err != nil {
			break
		}
		if h[0] != 0xFF || h[1]&0xE0 != 0xE0 {
			// not a frame sync; resync byte by byte (handles junk / ID3v1 "TAG")
			br.Discard(1)
			continue
		}

		version := (h[1] >> 3) & 0x03
		layer := (h[1] >> 1) & 0x03
		bitrateIdx := h[2] >> 4
		srIdx := (h[2] >> 2) & 0x03
		padding := int((h[2] >> 1) & 0x01)

		if version == 1 || layer != 1 || bitrateIdx == 0 || bitrateIdx == 15 || srIdx == 3 {
			br.Discard(1)
			continue
		}

		lsf := 0
		samplesPerFrame := 1152
		if version != 3 {
			lsf = 1
			samplesPerFrame = 576
		}
		sr := mp3SampleRates[version][srIdx]
		bitrate := mp3Bitrates[lsf][bitrateIdx] * 1000
		frameLen := (samplesPerFrame/8)*bitrate/sr + padding
		if frameLen < 4 {
			br.Discard(1)
			continue
		}

		sampleRate = sr
		totalSamples += int64(samplesPerFrame)
		frames++
		if _, err := br.Discard(frameLen); err != nil {
			break
		}
	}

	if frames == 0 || sampleRate == 0 {
		return 0, errors.New("mp3: no frames found")
	}
	return time.Duration(totalSamples * int64(time.Second) / int64(sampleRate)), nil
}

// probeMP4 walks the top-level ISO BMFF boxes (MP4/M4A) to "moov", then
// reads its "mvhd" box. The duration is the movie duration divided by its
// timescale. "moov" may sit before or after the media data, so boxes are
// skipped by seeking rather than read.
func probeMP4(r io.ReadSeeker) (time.Duration, error) {
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}

	moov, moovSize, err := findBox(r, 0, end, "moov")
	if err != nil {
		return 0, fmt.Errorf("mp4: %w", err)
	}
	mvhd, _, err := findBox(r, moov, moov+moovSize, "mvhd")
	if err != nil {
		return 0, fmt.Errorf("mp4: %w", err)
	}

	if _, err := r.Seek(mvhd, io.SeekStart); err != nil {
		return 0, err
	}
	// version (1) + flags (3), then version-dependent field sizes
	var vf [4]byte
	if _, err := io.ReadFull(r, vf[:]); err != nil {
		return 0, err
	}
	var timescale, duration uint64
	switch vf[0] {
	case 0:
		// creation (4), modification (4), timescale (4), duration (4)
		var b [16]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		timescale = uint64(binary.BigEndian.Uint32(b[8:12]))
		duration = uint64(binary.BigEndian.Uint32(b[12:16]))
	case 1:
		// creation (8), modification (8), timescale (4), duration (8)
		var b [28]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		timescale = uint64(binary.BigEndian.Uint32(b[16:20]))
		duration = binary.BigEndian.Uint64(b[20:28])
	default:
		return 0, fmt.Errorf("mp4: unsupported mvhd version %d", vf[0])
	}

	if timescale == 0 {
		return 0, errors.New("mp4: zero timescale")
	}
	// split into whole seconds and remainder so duration*time.Second can't overflow
	secs, rem := duration/timescale, duration%timescale
	return time.Duration(secs)*time.Second + time.Duration(rem*uint64(time.Second)/timescale), nil
}

// findBox scans the boxes between start and end for one of type typ and
// returns the offset of its payload (just past the header) and the payload size.
func findBox(r io.ReadSeeker, start, end int64, typ string) (int64, int64, error) {
	for off := start; off+8 <= end; {
		if _, err := r.Seek(off, io.SeekStart); err != nil {
			return 0, 0, err
		}
		var hdr [8]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return 0, 0, err
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		headerLen := int64(8)
		switch size {
		case 0: // box extends to the end of its parent
			size = end - off
		case 1: // 64-bit size follows the type
			var ext [8]byte
			if _, err := io.ReadFull(r, ext[:]); err != nil {
				return 0, 0, err
			}
			size = int64(binary.BigEndian.Uint64(ext[:]))
			headerLen = 16
		}
		if size < headerLen || off+size > end {
			return 0, 0, fmt.Errorf("corrupt %q box at %d", hdr[4:8], off)
		}
		if string(hdr[4:8]) == typ {
			return off + headerLen, size - headerLen, nil
		}
		off += size
	}
	return 0, 0, fmt.Errorf("no %q box", typ)
}
