// Package icns writes and reads Apple icon files.
//
// An .icns file is a container: the magic "icns" and the file's length,
// then entries, each a four-character type, its length including the
// eight-byte header, and the image. Every type written here holds a PNG,
// which macOS has read since 10.7, so the file is made in pure Go, the
// same on any host, with no need for iconutil or sips; cross-building a
// Mac app from Linux gets the same icon.
package icns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
)

// An Entry is one image in the file.
type Entry struct {
	Type string // the four-character code, such as ic10
	Data []byte // the PNG
}

// size is one image the file holds: a point size at a scale.
type size struct {
	typ    string
	pixels int
}

// sizes are what iconutil writes from a full .iconset, from 16 points at
// 1x to 512 points at 2x. 16 and 32 points use the PNG codes icp4 and icp5
// rather than iconutil's ARGB ic04 and ic05, which hold the same pixels in
// a run-length format of their own.
var sizes = []size{
	{"icp4", 16},   // 16pt
	{"ic11", 32},   // 16pt @2x
	{"icp5", 32},   // 32pt
	{"ic12", 64},   // 32pt @2x
	{"ic07", 128},  // 128pt
	{"ic13", 256},  // 128pt @2x
	{"ic08", 256},  // 256pt
	{"ic14", 512},  // 256pt @2x
	{"ic09", 512},  // 512pt
	{"ic10", 1024}, // 512pt @2x
}

// MinSize is the smallest source accepted: smaller than the 32-point
// Retina image, an icon would only be upscaled into blur.
const MinSize = 64

// Encode writes an icon file made from src, which must be square. Each
// size up to the source's own is scaled down from it; larger sizes are
// left out rather than scaled up, and macOS scales the largest one
// present instead, so a 512-pixel source still makes a working icon that
// lacks only the Retina 512-point image. It returns the pixel sizes it
// left out, so the caller can say so.
func Encode(w io.Writer, src image.Image) (skipped []int, err error) {
	b := src.Bounds()
	if b.Dx() != b.Dy() {
		return nil, fmt.Errorf("icon is %d×%d; it must be square, ideally 1024×1024", b.Dx(), b.Dy())
	}
	if b.Dx() < MinSize {
		return nil, fmt.Errorf("icon is %d×%d; it must be at least %d×%d, ideally 1024×1024", b.Dx(), b.Dy(), MinSize, MinSize)
	}
	scaled := map[int][]byte{}
	var entries []Entry
	for _, s := range sizes {
		if s.pixels > b.Dx() {
			if len(skipped) == 0 || skipped[len(skipped)-1] != s.pixels {
				skipped = append(skipped, s.pixels)
			}
			continue
		}
		data, ok := scaled[s.pixels]
		if !ok {
			var buf bytes.Buffer
			enc := png.Encoder{CompressionLevel: png.BestCompression}
			if err := enc.Encode(&buf, Resize(src, s.pixels)); err != nil {
				return nil, err
			}
			data = buf.Bytes()
			scaled[s.pixels] = data
		}
		entries = append(entries, Entry{Type: s.typ, Data: data})
	}
	return skipped, Write(w, entries)
}

// Write writes entries as an icon file.
func Write(w io.Writer, entries []Entry) error {
	total := 8
	for _, e := range entries {
		if len(e.Type) != 4 {
			return fmt.Errorf("icns: entry type %q is not four characters", e.Type)
		}
		total += 8 + len(e.Data)
	}
	var buf bytes.Buffer
	buf.Grow(total)
	buf.WriteString("icns")
	binary.Write(&buf, binary.BigEndian, uint32(total))
	for _, e := range entries {
		buf.WriteString(e.Type)
		binary.Write(&buf, binary.BigEndian, uint32(8+len(e.Data)))
		buf.Write(e.Data)
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// Read reads an icon file's entries, as written by Write or by iconutil.
func Read(r io.Reader) ([]Entry, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 || string(data[:4]) != "icns" {
		return nil, errors.New("icns: not an icon file")
	}
	if n := binary.BigEndian.Uint32(data[4:8]); int(n) != len(data) {
		return nil, fmt.Errorf("icns: header says %d bytes, file has %d", n, len(data))
	}
	var entries []Entry
	for p := 8; p < len(data); {
		if len(data)-p < 8 {
			return nil, fmt.Errorf("icns: truncated entry header at %d", p)
		}
		typ := string(data[p : p+4])
		n := int(binary.BigEndian.Uint32(data[p+4 : p+8]))
		if n < 8 || p+n > len(data) {
			return nil, fmt.Errorf("icns: entry %s at %d has bad length %d", typ, p, n)
		}
		entries = append(entries, Entry{Type: typ, Data: data[p+8 : p+n]})
		p += n
	}
	return entries, nil
}
