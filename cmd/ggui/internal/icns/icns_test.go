package icns

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"
)

// disc is a size×size opaque red disc on transparency, the shape of an
// icon: hard edges to downscale and a transparent surround whose color
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
				// Transparent, but green: a resizer that ignores alpha
				// would tint the edge green.
				img.SetNRGBA(x, y, color.NRGBA{0, 255, 0, 0})
			}
		}
	}
	return img
}

func TestEncodeReadBack(t *testing.T) {
	var buf bytes.Buffer
	skipped, err := Encode(&buf, disc(1024))
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped %v from a 1024 source", skipped)
	}
	entries, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, e := range entries {
		types = append(types, e.Type)
	}
	want := []string{"icp4", "ic11", "icp5", "ic12", "ic07", "ic13", "ic08", "ic14", "ic09", "ic10"}
	if !slices.Equal(types, want) {
		t.Fatalf("types = %v, want %v", types, want)
	}
	for i, e := range entries {
		img, err := png.Decode(bytes.NewReader(e.Data))
		if err != nil {
			t.Fatalf("%s: %v", e.Type, err)
		}
		size := sizes[i].pixels
		if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
			t.Errorf("%s is %v, want %d×%d", e.Type, b, size, size)
		}
		// The center stays the disc's color; a corner stays transparent.
		if c := color.NRGBAModel.Convert(img.At(size/2, size/2)).(color.NRGBA); c.R < 200 || c.G > 50 || c.A != 255 {
			t.Errorf("%s center = %v", e.Type, c)
		}
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Errorf("%s corner alpha = %d, want 0", e.Type, a)
		}
	}
}

func TestEncodeSmallSourceSkipsLargerSizes(t *testing.T) {
	var buf bytes.Buffer
	skipped, err := Encode(&buf, disc(256))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(skipped, []int{512, 1024}) {
		t.Errorf("skipped = %v, want [512 1024]", skipped)
	}
	entries, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 7 {
		t.Errorf("%d entries, want 7 (16 to 256 pixels)", len(entries))
	}
}

func TestEncodeRejects(t *testing.T) {
	if _, err := Encode(&bytes.Buffer{}, image.NewNRGBA(image.Rect(0, 0, 100, 80))); err == nil {
		t.Error("a non-square source was accepted")
	}
	if _, err := Encode(&bytes.Buffer{}, image.NewNRGBA(image.Rect(0, 0, 32, 32))); err == nil {
		t.Error("a 32-pixel source was accepted")
	}
}

func TestReadRejectsDamage(t *testing.T) {
	var buf bytes.Buffer
	Write(&buf, []Entry{{Type: "ic07", Data: []byte("data")}})
	good := buf.Bytes()
	if _, err := Read(bytes.NewReader(good)); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(bytes.NewReader(good[:len(good)-1])); err == nil {
		t.Error("a truncated file was read")
	}
	if _, err := Read(bytes.NewReader([]byte("nope"))); err == nil {
		t.Error("a file without the magic was read")
	}
}

// Averaging must keep the mean: an image half white and half black, by
// columns, comes out as the gray that is half the light, not half the
// sRGB value, which is what working in linear light buys.
func TestResizeAveragesLight(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			v := uint8(0)
			if x%2 == 0 {
				v = 255
			}
			src.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	got := Resize(src, 16).NRGBAAt(8, 8)
	// Half of linear white is sRGB 188.
	if got.R < 186 || got.R > 190 || got.A != 255 {
		t.Errorf("resized = %v, want sRGB 188 gray", got)
	}
}
