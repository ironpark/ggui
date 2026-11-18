package ico

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"
)

// disc is a size×size opaque red disc on transparency, the shape of an
// icon: hard edges to scale down and a transparent surround whose color
// must not leak in.
func disc(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	r := size * 2 / 5
	for y := range size {
		for x := range size {
			dx, dy := x-size/2, y-size/2
			if dx*dx+dy*dy < r*r {
				img.SetNRGBA(x, y, color.NRGBA{220, 30, 30, 255})
			} else {
				img.SetNRGBA(x, y, color.NRGBA{0, 255, 0, 0})
			}
		}
	}
	return img
}

// entry is a directory entry read back from an .ico file.
type dirEntry struct {
	width, height, colors, reserved byte
	planes, bitCount                uint16
	size, offset                    uint32
}

// readICO parses an .ico file the way Windows does: the ICONDIR, then
// each entry, then each image where its entry says.
func readICO(t *testing.T, data []byte) ([]dirEntry, [][]byte) {
	t.Helper()
	le := binary.LittleEndian
	if len(data) < 6 || le.Uint16(data[0:]) != 0 || le.Uint16(data[2:]) != 1 {
		t.Fatalf("not an icon file: % x", data[:min(len(data), 6)])
	}
	n := int(le.Uint16(data[4:]))
	var es []dirEntry
	var imgs [][]byte
	for i := range n {
		b := data[6+16*i:]
		e := dirEntry{b[0], b[1], b[2], b[3], le.Uint16(b[4:]), le.Uint16(b[6:]), le.Uint32(b[8:]), le.Uint32(b[12:])}
		if int(e.offset)+int(e.size) > len(data) {
			t.Fatalf("entry %d runs past the file: %+v", i, e)
		}
		es = append(es, e)
		imgs = append(imgs, data[e.offset:e.offset+e.size])
	}
	return es, imgs
}

// decodeDIB reads an icon's bitmap back into an image, checking its
// header, and returns the mask as a bitmap of transparent pixels.
func decodeDIB(t *testing.T, data []byte, size int) (*image.NRGBA, [][]bool) {
	t.Helper()
	le := binary.LittleEndian
	if le.Uint32(data[0:]) != 40 || int(le.Uint32(data[4:])) != size || int(le.Uint32(data[8:])) != 2*size ||
		le.Uint16(data[12:]) != 1 || le.Uint16(data[14:]) != 32 || le.Uint32(data[16:]) != 0 {
		t.Fatalf("%d: bad BITMAPINFOHEADER % x", size, data[:40])
	}
	stride := (size + 31) / 32 * 4
	if want := 40 + size*size*4 + stride*size; len(data) != want || int(le.Uint32(data[20:])) != want-40 {
		t.Fatalf("%d: bitmap is %d bytes, biSizeImage %d; want %d", size, len(data), le.Uint32(data[20:]), want)
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	mask := make([][]bool, size)
	px, mk := data[40:], data[40+size*size*4:]
	for y := range size {
		row := size - 1 - y
		mask[y] = make([]bool, size)
		for x := range size {
			o := (row*size + x) * 4
			img.SetNRGBA(x, y, color.NRGBA{px[o+2], px[o+1], px[o], px[o+3]})
			mask[y][x] = mk[row*stride+x/8]&(0x80>>(x%8)) != 0
		}
	}
	return img, mask
}

func TestEncodeReadBack(t *testing.T) {
	imgs, skipped, err := Images(disc(1024))
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped %v from a 1024 source", skipped)
	}
	var buf bytes.Buffer
	if err := Encode(&buf, imgs); err != nil {
		t.Fatal(err)
	}
	es, data := readICO(t, buf.Bytes())
	if len(es) != len(Sizes) {
		t.Fatalf("%d images, want %d", len(es), len(Sizes))
	}
	for i, e := range es {
		size := Sizes[i]
		dim := byte(size)
		if e.width != dim || e.height != dim || e.colors != 0 || e.reserved != 0 || e.planes != 1 || e.bitCount != 32 {
			t.Errorf("%d: entry %+v", size, e)
		}
		var img image.Image
		var mask [][]bool
		if size >= 256 {
			if !bytes.HasPrefix(data[i], []byte("\x89PNG")) {
				t.Fatalf("%d is not stored as PNG", size)
			}
			if img, err = png.Decode(bytes.NewReader(data[i])); err != nil {
				t.Fatalf("%d: %v", size, err)
			}
		} else {
			img, mask = decodeDIB(t, data[i], size)
		}
		if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
			t.Errorf("%d is %v", size, b)
		}
		// The centre keeps the disc's color, the corner its transparency,
		// and the mask marks the corner and not the centre: the rows are
		// the right way up, and the channels in BGRA order.
		if c := color.NRGBAModel.Convert(img.At(size/2, size/2)).(color.NRGBA); c.R < 200 || c.G > 50 || c.B > 50 || c.A != 255 {
			t.Errorf("%d: centre = %v", size, c)
		}
		if _, _, _, a := img.At(0, size-1).RGBA(); a != 0 {
			t.Errorf("%d: corner alpha = %d", size, a)
		}
		if mask != nil && (!mask[size-1][0] || mask[size/2][size/2]) {
			t.Errorf("%d: mask wrong at the corner or centre", size)
		}
	}
}

func TestSmallSourceSkipsLargerSizes(t *testing.T) {
	imgs, skipped, err := Images(disc(64))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(skipped, []int{96, 256}) || len(imgs) != len(Sizes)-2 {
		t.Errorf("skipped = %v, %d images", skipped, len(imgs))
	}
	if _, _, err := Images(image.NewNRGBA(image.Rect(0, 0, 100, 80))); err == nil {
		t.Error("a non-square source was accepted")
	}
	if _, _, err := Images(image.NewNRGBA(image.Rect(0, 0, 32, 32))); err == nil {
		t.Error("a 32-pixel source was accepted")
	}
}

// The icon group resource is the file's directory with resource IDs in
// place of offsets: 14-byte entries.
func TestGroup(t *testing.T) {
	imgs, _, err := Images(disc(256))
	if err != nil {
		t.Fatal(err)
	}
	g := Group(imgs, 7)
	le := binary.LittleEndian
	if len(g) != 6+14*len(imgs) || le.Uint16(g[2:]) != 1 || int(le.Uint16(g[4:])) != len(imgs) {
		t.Fatalf("group header % x, %d bytes", g[:6], len(g))
	}
	for i, im := range imgs {
		e := g[6+14*i:]
		if e[0] != byte(im.Size) || le.Uint16(e[6:]) != 32 || int(le.Uint32(e[8:])) != len(im.Data) || le.Uint16(e[12:]) != uint16(7+i) {
			t.Errorf("entry %d = % x", i, e[:14])
		}
	}
}
