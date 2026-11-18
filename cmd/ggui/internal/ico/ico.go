// Package ico makes the Windows icon: the sizes Windows shows a program's
// icon at, scaled down from one PNG in linear light by the same resizer
// as the macOS icon, as an .ico file or as the icon resources a program
// links in.
//
// It is written in pure Go, like the .icns, so that a Windows program
// cross-built on macOS or Linux has its icon too, with no rc.exe,
// windres or ImageMagick.
package ico

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"

	"github.com/ironpark/ggui/cmd/ggui/internal/icns"
)

// Sizes are the pixel sizes an icon holds: those Explorer, the taskbar,
// the Start menu and the title bar ask for at 100% to 300% scaling, as
// Microsoft's icon guidelines list them. Windows scales the nearest
// larger one for any other.
var Sizes = []int{16, 20, 24, 32, 40, 48, 64, 96, 256}

// pngFrom is the smallest size stored as PNG. Windows has read
// PNG-compressed icon images since Vista, but some readers of icons, old
// .NET and older resource editors among them, read PNG only at 256, as
// Visual Studio writes it; below, an uncompressed bitmap costs a few
// kilobytes and reads everywhere.
const pngFrom = 256

// MinSize is the smallest source accepted, the macOS icon's minimum, so
// that one PNG serves both.
const MinSize = icns.MinSize

// Image is one image of an icon.
type Image struct {
	Size int // width and height in pixels
	// Data is a PNG file, for sizes from pngFrom, or else a device
	// independent bitmap as icons hold it: a BITMAPINFOHEADER of twice
	// the height, the 32-bit BGRA rows bottom up, and a 1-bit mask below
	// them.
	Data []byte
}

// PNG reports whether the image is stored as PNG.
func (im Image) PNG() bool { return im.Size >= pngFrom }

// Images makes every size up to the source's own from src, which must be
// square. Larger sizes are left out rather than scaled up, since Windows
// scales the largest present instead; it returns those it left out, so
// the caller can say so.
func Images(src image.Image) (imgs []Image, skipped []int, err error) {
	b := src.Bounds()
	if b.Dx() != b.Dy() {
		return nil, nil, fmt.Errorf("icon is %d×%d; it must be square, ideally 1024×1024", b.Dx(), b.Dy())
	}
	if b.Dx() < MinSize {
		return nil, nil, fmt.Errorf("icon is %d×%d; it must be at least %d×%d, ideally 1024×1024", b.Dx(), b.Dy(), MinSize, MinSize)
	}
	for _, size := range Sizes {
		if size > b.Dx() {
			skipped = append(skipped, size)
			continue
		}
		px := icns.Resize(src, size)
		var data []byte
		if size >= pngFrom {
			var buf bytes.Buffer
			enc := png.Encoder{CompressionLevel: png.BestCompression}
			if err := enc.Encode(&buf, px); err != nil {
				return nil, nil, err
			}
			data = buf.Bytes()
		} else {
			data = dib(px)
		}
		imgs = append(imgs, Image{Size: size, Data: data})
	}
	return imgs, skipped, nil
}

// dib returns img as an icon's bitmap. The colors are straight alpha, as
// a 32-bit icon's are; the mask, which Windows ignores beside an alpha
// channel but older readers draw with, marks the fully transparent
// pixels.
func dib(img *image.NRGBA) []byte {
	size := img.Bounds().Dx()
	maskStride := (size + 31) / 32 * 4
	colors, mask := size*size*4, maskStride*size
	out := make([]byte, 40, 40+colors+mask)
	le := binary.LittleEndian
	le.PutUint32(out[0:], 40)                   // biSize
	le.PutUint32(out[4:], uint32(size))         // biWidth
	le.PutUint32(out[8:], uint32(2*size))       // biHeight: the colors and the mask
	le.PutUint16(out[12:], 1)                   // biPlanes
	le.PutUint16(out[14:], 32)                  // biBitCount
	le.PutUint32(out[20:], uint32(colors+mask)) // biSizeImage; biCompression is BI_RGB, 0
	px := make([]byte, colors)
	mk := make([]byte, mask)
	for y := range size {
		row := size - 1 - y // bottom up
		for x := range size {
			c := img.NRGBAAt(x, y)
			o := (row*size + x) * 4
			px[o], px[o+1], px[o+2], px[o+3] = c.B, c.G, c.R, c.A
			if c.A == 0 {
				mk[row*maskStride+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	out = append(out, px...)
	return append(out, mk...)
}

// entry is the part of a directory entry an .ico file and an icon group
// resource share: everything but where the image is.
func entry(im Image) []byte {
	e := make([]byte, 12)
	dim := byte(im.Size) // 256 is written as 0
	e[0], e[1] = dim, dim
	// bColorCount and bReserved are 0.
	binary.LittleEndian.PutUint16(e[4:], 1)  // wPlanes
	binary.LittleEndian.PutUint16(e[6:], 32) // wBitCount
	binary.LittleEndian.PutUint32(e[8:], uint32(len(im.Data)))
	return e
}

// header is ICONDIR: reserved, type 1 for an icon, and the count.
func header(n int) []byte {
	h := make([]byte, 6)
	binary.LittleEndian.PutUint16(h[2:], 1)
	binary.LittleEndian.PutUint16(h[4:], uint16(n))
	return h
}

// Encode writes imgs as an .ico file.
func Encode(w io.Writer, imgs []Image) error {
	if len(imgs) == 0 {
		return errors.New("an icon needs at least one image")
	}
	var buf bytes.Buffer
	buf.Write(header(len(imgs)))
	off := 6 + 16*len(imgs)
	for _, im := range imgs {
		buf.Write(entry(im))
		binary.Write(&buf, binary.LittleEndian, uint32(off))
		off += len(im.Data)
	}
	for _, im := range imgs {
		buf.Write(im.Data)
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// Group returns the RT_GROUP_ICON resource for imgs linked as RT_ICON
// resources numbered from firstID: the .ico file's directory with each
// image's resource ID where the file has its offset.
func Group(imgs []Image, firstID uint16) []byte {
	var buf bytes.Buffer
	buf.Write(header(len(imgs)))
	for i, im := range imgs {
		buf.Write(entry(im))
		binary.Write(&buf, binary.LittleEndian, firstID+uint16(i))
	}
	return buf.Bytes()
}
