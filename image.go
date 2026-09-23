package ggui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // registered for DecodeImage
	_ "image/jpeg" // registered for DecodeImage
	_ "image/png"  // registered for DecodeImage
	"math"
	"os"

	"github.com/ironpark/ggfx"

	"github.com/ironpark/ggui/internal/property"
)

// ImageFit says how an image is placed in a box of another shape.
type ImageFit int

const (
	FitContain ImageFit = iota // scaled to fit inside, keeping its aspect ratio
	FitCover                   // scaled to cover the box, keeping its aspect ratio, and clipped
	FitFill                    // stretched to the box
	FitNone                    // drawn at its natural size, centered, and clipped
)

// ImageWidget draws an ggfx.Image. Build one with Image.
type ImageWidget struct {
	props     property.Owner
	img       *ggfx.Image
	fit       ImageFit
	width     float64
	height    float64
	pixelated bool
	alt       string
}

// Image draws img. With no Size it asks for the image's natural size in
// logical pixels, and scales down, keeping its aspect ratio, when the
// parent gives it less room.
func Image(img *ggfx.Image) *ImageWidget {
	return &ImageWidget{img: img}
}

// Fit sets how the image is placed when the box has another shape; the
// default is FitContain.
func (w *ImageWidget) Fit(f ImageFit) *ImageWidget {
	defer property.Watch(&w.props, &w.fit)()
	w.fit = f
	return w
}

// Size fixes both dimensions. A zero dimension follows the other, keeping
// the aspect ratio, or the image when both are zero.
func (w *ImageWidget) Size(width, height float64) *ImageWidget {
	defer property.Watch(&w.props, &w.height)()
	defer property.Watch(&w.props, &w.width)()
	w.width, w.height = width, height
	return w
}

// Width fixes the width; the height follows the aspect ratio.
func (w *ImageWidget) Width(v float64) *ImageWidget {
	defer property.Watch(&w.props, &w.width)()
	w.width = v
	return w
}

// Height fixes the height; the width follows the aspect ratio.
func (w *ImageWidget) Height(v float64) *ImageWidget {
	defer property.Watch(&w.props, &w.height)()
	w.height = v
	return w
}

// Pixelated scales the image by its nearest pixel instead of smoothing it,
// for pixel art.
func (w *ImageWidget) Pixelated(v bool) *ImageWidget {
	defer property.Watch(&w.props, &w.pixelated)()
	w.pixelated = v
	return w
}

// Alt describes the image for a screen reader. Without one the image is
// decorative and stays out of the accessibility tree entirely, which is
// what an icon beside a label that already says the same thing wants.
func (w *ImageWidget) Alt(s string) *ImageWidget {
	defer property.Watch(&w.props, &w.alt)()
	w.alt = s
	return w
}

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
	defer w.props.Layout()()
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

// Place returns the Rect an image of the natural size n is drawn in to fit
// the box r this way. An image with no size fills r.
func (f ImageFit) Place(n Size, r Rect) Rect {
	if n.W == 0 || n.H == 0 {
		return r
	}
	var s Size
	switch f {
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
	if w.alt != "" {
		dst.Leaf(r, Node{Role: RoleImage, Name: w.alt})
	}
	at := w.fit.Place(w.natural(), r)
	target := dst
	if at != r {
		target = dst.Clip(r)
	}
	target.DrawImage(w.img, at, ImageOptions{Pixelated: w.pixelated})
}

// ImageOptions says how DrawImage draws an image. The zero value draws it
// opaque, upright and smoothed.
type ImageOptions struct {
	// Fade is how far the image fades out: 0 draws it opaque, 1 not at all.
	Fade float64
	// Tint multiplies every pixel, which is how a white icon takes a
	// colour; nil leaves them.
	Tint color.Color
	// Rotation turns the image clockwise about the centre of where it is
	// drawn, in radians.
	Rotation float64
	// Pixelated scales by the nearest pixel instead of smoothing.
	Pixelated bool
}

// DrawImage draws img stretched over the logical Rect r, as o says.
func (c *Canvas) DrawImage(img *ggfx.Image, r Rect, o ImageOptions) {
	if c == nil || c.Image == nil || img == nil || o.Fade >= 1 {
		return
	}
	b := img.Bounds()
	if b.Empty() {
		return
	}
	// A turned image reaches as far as its corners do.
	reach := 0.0
	if o.Rotation != 0 {
		reach = math.Hypot(r.Size.W, r.Size.H) / 2
	}
	if !c.visiblePaintBounds(r, reach) {
		return
	}
	op := &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear}
	if o.Pixelated {
		op.Filter = ggfx.FilterNearest
	}
	w, h := float64(b.Dx()), float64(b.Dy())
	if o.Rotation != 0 {
		op.GeoM.Translate(-w/2, -h/2)
		op.GeoM.Scale(r.Size.W/w, r.Size.H/h)
		op.GeoM.Rotate(o.Rotation)
		op.GeoM.Concat(c.Geo(r.Center()))
	} else {
		op.GeoM.Scale(r.Size.W/w, r.Size.H/h)
		op.GeoM.Concat(c.Geo(r.Origin))
	}
	if o.Tint != nil {
		op.ColorScale.ScaleWithColor(o.Tint)
	}
	if o.Fade != 0 {
		op.ColorScale.ScaleAlpha(float32(1 - o.Fade))
	}
	c.Image.DrawImage(img, op)
}

// DecodeImage decodes PNG, JPEG or GIF bytes into an ggfx.Image.
func DecodeImage(data []byte) (*ggfx.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ggui: decode image: %w", err)
	}
	return ggfx.NewImageFromImage(img), nil
}

// LoadImageFile decodes the PNG, JPEG or GIF file at path.
func LoadImageFile(path string) (*ggfx.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ggui: load image: %w", err)
	}
	return DecodeImage(data)
}
