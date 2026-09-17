package ggui

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // registered for DecodeImage
	_ "image/jpeg" // registered for DecodeImage
	_ "image/png"  // registered for DecodeImage
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

// ImageFit says how an image is placed in a box of another shape.
type ImageFit int

const (
	FitContain ImageFit = iota // scaled to fit inside, keeping its aspect ratio
	FitCover                   // scaled to cover the box, keeping its aspect ratio, and clipped
	FitFill                    // stretched to the box
	FitNone                    // drawn at its natural size, centered, and clipped
)

// ImageWidget draws an ebiten.Image. Build one with Image.
type ImageWidget struct {
	img    *ebiten.Image
	fit    ImageFit
	width  float64
	height float64
	filter ebiten.Filter
}

// Image draws img. With no Size it asks for the image's natural size in
// logical pixels, and scales down, keeping its aspect ratio, when the
// parent gives it less room.
func Image(img *ebiten.Image) *ImageWidget {
	return &ImageWidget{img: img, filter: ebiten.FilterLinear}
}

// Fit sets how the image is placed when the box has another shape; the
// default is FitContain.
func (w *ImageWidget) Fit(f ImageFit) *ImageWidget { w.fit = f; return w }

// Size fixes both dimensions. A zero dimension follows the other, keeping
// the aspect ratio, or the image when both are zero.
func (w *ImageWidget) Size(width, height float64) *ImageWidget {
	w.width, w.height = width, height
	return w
}

// Width fixes the width; the height follows the aspect ratio.
func (w *ImageWidget) Width(v float64) *ImageWidget { w.width = v; return w }

// Height fixes the height; the width follows the aspect ratio.
func (w *ImageWidget) Height(v float64) *ImageWidget { w.height = v; return w }

// Filter sets how pixels are sampled when scaled; the default is linear.
func (w *ImageWidget) Filter(f ebiten.Filter) *ImageWidget { w.filter = f; return w }

// natural is the image's size in logical pixels.
func (w *ImageWidget) natural() Size {
	if w.img == nil {
		return Size{}
	}
	b := w.img.Bounds()
	return Sz(float64(b.Dx()), float64(b.Dy()))
}

// Layout implements Widget.
func (w *ImageWidget) Layout(c Constraints, env Env) Size {
	n := w.natural()
	want := n
	ratio := 1.0
	if n.W > 0 && n.H > 0 {
		ratio = n.W / n.H
	}
	switch {
	case w.width > 0 && w.height > 0:
		want = Sz(w.width, w.height)
	case w.width > 0:
		want = Sz(w.width, w.width/ratio)
	case w.height > 0:
		want = Sz(w.height*ratio, w.height)
	default:
		// Natural size, shrunk to what fits.
		if want.W > c.MaxW {
			want = Sz(c.MaxW, c.MaxW/ratio)
		}
		if want.H > c.MaxH {
			want = Sz(c.MaxH*ratio, c.MaxH)
		}
	}
	return c.Constrain(want)
}

// placement returns the Rect the image is drawn in for a box r.
func (w *ImageWidget) placement(r Rect) Rect {
	n := w.natural()
	if n.W == 0 || n.H == 0 {
		return r
	}
	var s Size
	switch w.fit {
	case FitFill:
		return r
	case FitNone:
		s = n
	case FitCover:
		k := max(r.Size.W/n.W, r.Size.H/n.H)
		s = Sz(n.W*k, n.H*k)
	default:
		k := min(r.Size.W/n.W, r.Size.H/n.H)
		s = Sz(n.W*k, n.H*k)
	}
	return Rct(Pt(r.Origin.X+(r.Size.W-s.W)/2, r.Origin.Y+(r.Size.H-s.H)/2), s)
}

// Paint implements Widget.
func (w *ImageWidget) Paint(dst *Canvas, r Rect) {
	if dst == nil || dst.Image == nil || w.img == nil {
		return
	}
	n := w.natural()
	if n.W == 0 || n.H == 0 {
		return
	}
	at := w.placement(r)
	target := dst
	if at != r {
		target = dst.Clip(r)
	}
	op := &ebiten.DrawImageOptions{Filter: w.filter}
	op.GeoM.Scale(at.Size.W/n.W, at.Size.H/n.H)
	op.GeoM.Concat(dst.Geo(at.Origin))
	target.Image.DrawImage(w.img, op)
}

// DecodeImage decodes PNG, JPEG or GIF bytes into an ebiten.Image.
func DecodeImage(data []byte) (*ebiten.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ggui: decode image: %w", err)
	}
	return ebiten.NewImageFromImage(img), nil
}

// LoadImageFile decodes the PNG, JPEG or GIF file at path.
func LoadImageFile(path string) (*ebiten.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ggui: load image: %w", err)
	}
	return DecodeImage(data)
}
