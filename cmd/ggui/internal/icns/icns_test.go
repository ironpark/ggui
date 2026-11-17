package icns

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/ironpark/zapp/pkg/icns"
)

// read decodes an icon file with zapp's reader.
func read(t *testing.T, data []byte) []icns.Element {
	t.Helper()
	f, err := icns.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return f.Elements
}

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
	entries := read(t, buf.Bytes())
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
	entries := read(t, buf.Bytes())
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

// iconutil, Apple's own tool, must read the file back into an iconset
// with every size.
func TestIconutilReadsIt(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("iconutil is part of macOS")
	}
	dir := t.TempDir()
	var buf bytes.Buffer
	if _, err := Encode(&buf, disc(1024)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "AppIcon.icns")
	os.WriteFile(path, buf.Bytes(), 0o644)
	set := filepath.Join(dir, "AppIcon.iconset")
	if out, err := exec.Command("iconutil", "-c", "iconset", "-o", set, path).CombinedOutput(); err != nil {
		t.Fatalf("iconutil: %s", out)
	}
	files, _ := filepath.Glob(filepath.Join(set, "*.png"))
	if len(files) != len(sizes) {
		t.Errorf("iconutil wrote %d images, want %d: %v", len(files), len(sizes), files)
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
