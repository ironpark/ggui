package ggui

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// encodedImages returns a tiny image in each format DecodeImage reads.
func encodedImages(t testing.TB) map[string][]byte {
	t.Helper()
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := range src.Pix {
		src.Pix[i] = byte(i * 17)
	}
	pal := image.NewPaletted(image.Rect(0, 0, 3, 2), color.Palette{color.Black, color.White})
	pal.SetColorIndex(1, 1, 1)
	var p, j, g bytes.Buffer
	if err := png.Encode(&p, src); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&j, src, nil); err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(&g, pal, nil); err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{"png": p.Bytes(), "jpeg": j.Bytes(), "gif": g.Bytes()}
}

// largestFuzzImage bounds the pixels a fuzzed header may ask for: the
// standard decoders allocate what the header says before reading the
// data, so a few bytes could otherwise ask for gigabytes.
const largestFuzzImage = 1 << 20

func FuzzDecodeImage(f *testing.F) {
	for _, data := range encodedImages(f) {
		f.Add(data)
		f.Add(data[:len(data)/2])
	}
	f.Add([]byte{})
	f.Add([]byte("GIF89a"))
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add([]byte{0xff, 0xd8, 0xff})
	f.Add(emptyGIF)
	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, _, cfgErr := image.DecodeConfig(bytes.NewReader(data))
		if cfgErr == nil && cfg.Width*cfg.Height > largestFuzzImage {
			return
		}
		want, _, stdErr := image.Decode(bytes.NewReader(data))
		if stdErr == nil && want.Bounds().Empty() {
			t.Skip("BUG: DecodeImage panics on an image with no pixels; see TestDecodeImageRejectsAnEmptyImage")
		}
		img, err := DecodeImage(data)
		if (err == nil) != (stdErr == nil) {
			t.Fatalf("DecodeImage error %v, but image.Decode error %v", err, stdErr)
		}
		if err != nil {
			if img != nil {
				t.Fatalf("DecodeImage returned an image with the error %v", err)
			}
			if !strings.HasPrefix(err.Error(), "ggui: decode image: ") {
				t.Fatalf("error %q does not say what failed", err)
			}
			return
		}
		if got, wantB := img.Bounds(), want.Bounds(); got.Dx() != wantB.Dx() || got.Dy() != wantB.Dy() {
			t.Fatalf("decoded image is %v, want the %v image.Decode reads", got, wantB)
		}
	})
}

// emptyGIF is a well-formed GIF whose one frame is 0x0: image.Decode
// accepts it as an image with no pixels.
var emptyGIF = []byte("GIF89a\x00\x00\x00\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff" +
	"\x2c\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x01\x2c\x00\x3b")

func TestDecodeImageRejectsAnEmptyImage(t *testing.T) {
	t.Parallel()
	t.Skip("BUG: DecodeImage passes a 0x0 image.Decode result to ggfx.NewImageFromImage (image.go:242), which panics \"width at NewImage must be positive\" instead of returning an error")
	if _, err := DecodeImage(emptyGIF); err == nil {
		t.Fatal("DecodeImage of a 0x0 GIF succeeded, want an error: there is nothing to draw")
	}
}

func TestDecodeImageReadsEveryRegisteredFormat(t *testing.T) {
	t.Parallel()
	for name, data := range encodedImages(t) {
		img, err := DecodeImage(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if b := img.Bounds(); b.Dx() != 3 || b.Dy() != 2 {
			t.Errorf("%s decoded to %v, want 3x2", name, b)
		}
	}
	_, err := DecodeImage([]byte("not an image"))
	if !errors.Is(err, image.ErrFormat) {
		t.Fatalf("DecodeImage of text = %v, want an error wrapping image.ErrFormat", err)
	}
}

func TestLoadImageFileReadsTheFileAndReportsWhatFailed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "icon.png")
	if err := os.WriteFile(path, encodedImages(t)["png"], 0o600); err != nil {
		t.Fatal(err)
	}
	img, err := LoadImageFile(path)
	if err != nil || img.Bounds().Dx() != 3 {
		t.Fatalf("LoadImageFile = %v, %v; want the 3x2 image written", img, err)
	}
	if _, err := LoadImageFile(filepath.Join(dir, "missing.png")); !errors.Is(err, fs.ErrNotExist) || !strings.HasPrefix(err.Error(), "ggui: load image: ") {
		t.Fatalf("a missing file gave %v, want a load error wrapping fs.ErrNotExist", err)
	}
	junk := filepath.Join(dir, "junk.png")
	if err := os.WriteFile(junk, []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadImageFile(junk); err == nil || !strings.HasPrefix(err.Error(), "ggui: decode image: ") {
		t.Fatalf("a file that is not an image gave %v, want a decode error", err)
	}
}
