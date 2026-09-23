package icons

import (
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"math"
	"strings"
	"sync"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// SVG is a parsed monochrome SVG. Its alpha coverage is tinted at draw time,
// including currentColor strokes and fills. Gradients and multicolor artwork
// should use ggui.Image instead. SVGs are intended to be trusted local assets.
// A shared SVG retains at most eight device-resolution rasterizations.
type SVG struct {
	mu    sync.Mutex
	icon  *oksvg.SvgIcon
	cache []raster
}
type raster struct {
	size  int
	image *ggfx.Image
}

// Parse compiles SVG paths once without creating GPU resources.
func Parse(data []byte) (*SVG, error) {
	source := strings.ReplaceAll(string(data), "currentColor", "#ffffff")
	icon, err := oksvg.ReadIconStream(strings.NewReader(source), oksvg.StrictErrorMode)
	if err != nil {
		return nil, fmt.Errorf("icons: parse SVG: %w", err)
	}
	b := icon.ViewBox
	if b.W <= 0 || b.H <= 0 || math.IsNaN(b.W+b.H+b.X+b.Y) || math.IsInf(b.W+b.H+b.X+b.Y, 0) {
		return nil, fmt.Errorf("icons: SVG needs a finite positive viewBox")
	}
	return &SVG{icon: icon}, nil
}

// Load reads an SVG from an embedded or other filesystem.
func Load(files fs.FS, path string) (*SVG, error) {
	data, err := fs.ReadFile(files, path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func (s *SVG) rasterize(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scanner := rasterx.NewScannerGV(size, size, img, img.Bounds())
	painter := rasterx.NewDasher(size, size, scanner)
	b := s.icon.ViewBox
	scale := float64(size) / max(b.W, b.H)
	s.icon.Transform = rasterx.Matrix2D{A: scale, D: scale, E: (float64(size)-b.W*scale)/2 - b.X*scale, F: (float64(size)-b.H*scale)/2 - b.Y*scale}
	s.icon.Draw(painter, 1)
	// Store a white coverage mask so recoloring never requires rasterization.
	for i := 0; i < len(img.Pix); i += 4 {
		a := img.Pix[i+3]
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = a, a, a
	}
	return img
}

// Draw paints the icon centered in r, preserving its aspect ratio. Rotation is
// clockwise in radians. Call on the UI thread, as with other Canvas methods.
func (s *SVG) Draw(dst *ggui.Canvas, r ggui.Rect, col color.Color, rotation float64) {
	if s == nil || dst == nil || dst.Image == nil || col == nil {
		return
	}
	side := min(r.Size.W, r.Size.H)
	if side <= 0 || math.IsNaN(side) || math.IsInf(side, 0) {
		return
	}
	size := int(min(1024, math.Ceil(side*dst.Scale())))
	if size < 1 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var img *ggfx.Image
	for i, c := range s.cache {
		if c.size == size {
			img = c.image
			copy(s.cache[i:], s.cache[i+1:])
			s.cache[len(s.cache)-1] = c
			break
		}
	}
	if img == nil {
		img = ggfx.NewImageFromImage(s.rasterize(size))
		if len(s.cache) == 8 {
			s.cache[0].image.Deallocate()
			s.cache = s.cache[1:]
		}
		s.cache = append(s.cache, raster{size, img})
	}
	at := ggui.FitContain.Place(ggui.Sz(1, 1), r)
	dst.DrawImage(img, at, ggui.ImageOptions{Fit: ggui.FitFill, Tint: col, Rotation: rotation})
}
