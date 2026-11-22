// Package icns makes the app icon: every size an .icns file holds, scaled
// down from one PNG in linear light.
//
// Every image is a PNG, which macOS has read since 10.7, so the file is
// made in pure Go, the same on any host, with no need for iconutil or
// sips; cross-building a Mac app from Linux gets the same icon. The file
// itself, a container of typed images, is written by zapp's icns package.
package icns

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"

	"github.com/ironpark/zapp/pkg/icns"
)

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
	resizer := NewResizer(src)
	scaled := map[int][]byte{}
	var family icns.ICNS
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
			if err := enc.Encode(&buf, resizer.Resize(s.pixels)); err != nil {
				return nil, err
			}
			data = buf.Bytes()
			scaled[s.pixels] = data
		}
		// zapp's encoder would write 16 and 32 points as run-length
		// RGB, and would recompress at its own level; the element is the
		// PNG made here.
		family.Elements = append(family.Elements, icns.Element{Type: s.typ, Data: data})
	}
	return skipped, icns.Encode(w, &family)
}
