package ggui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // registered for DecodeImage
	_ "image/jpeg" // registered for DecodeImage
	_ "image/png"  // registered for DecodeImage
	"math"
	"os"
	"runtime"
	"weak"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/imgcache"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
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
	defer w.props.Layout().EndLayout()
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
	dst.DrawImage(w.img, r, ImageOptions{Fit: w.fit, Pixelated: w.pixelated})
}

// ImageOptions says how DrawImage draws an image. The zero value fits it
// inside the Rect, opaque, upright and smoothed.
type ImageOptions struct {
	// Fit places the image in the Rect; what falls outside is clipped.
	Fit ImageFit
	// Radius rounds the Rect's corners, antialiased, and the image is cut
	// to them: at half the side of a square, a round photo.
	Radius float64
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

// DrawImage draws img in the logical Rect r, as o says.
func (c *Canvas) DrawImage(img *ggfx.Image, r Rect, o ImageOptions) {
	if c == nil || c.Image == nil || img == nil || o.Fade >= 1 {
		return
	}
	b := img.Bounds()
	if b.Empty() {
		return
	}
	at := o.Fit.Place(Sz(b.Dx(), b.Dy()), r)
	if o.Radius > 0 {
		c.shadeImage(img, r, at, o)
		return
	}
	if at != r {
		c = c.Clip(r)
	}
	c.drawImage(img, at, o)
}

// drawImage draws img stretched over the logical Rect r.
func (c *Canvas) drawImage(img *ggfx.Image, r Rect, o ImageOptions) {
	b := img.Bounds()
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
	img, err := decodeImage(data)
	if err != nil {
		return nil, err
	}
	return newTexture(img), nil
}

// largeImage is the side past which an image is a texture of its own: the
// atlas would round it up to a page of powers of two, 2048×2048 and 16 MB
// for a 1254-pixel square, and it packs badly with the small images the
// atlas batches.
const largeImage = 512

// newTexture uploads img, a texture of exactly its size when it is large.
func newTexture(img image.Image) *ggfx.Image {
	if b := img.Bounds(); b.Dx() > largeImage || b.Dy() > largeImage {
		return ggfx.NewImageFromImageWithOptions(img, &ggfx.NewImageFromImageOptions{Unmanaged: true})
	}
	return ggfx.NewImageFromImage(img)
}

// decodeImage decodes PNG, JPEG or GIF bytes, refusing an image with no
// pixels.
func decodeImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ggui: decode image: %w", err)
	}
	if img.Bounds().Empty() {
		return nil, errors.New("ggui: decode image: the image has no pixels")
	}
	return img, nil
}

// LoadImageFile decodes the PNG, JPEG or GIF file at path.
func LoadImageFile(path string) (*ggfx.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ggui: load image: %w", err)
	}
	return DecodeImage(data)
}

// ImageLoader produces the image for src. It runs on a goroutine of its
// own, never the UI thread, and should give up when ctx is cancelled: the
// widget asking for it moved on to another src, or went away.
type ImageLoader func(ctx context.Context, src string) (image.Image, error)

// LoadImage is the default ImageLoader: it reads the PNG, JPEG or GIF file
// at the path src.
func LoadImage(ctx context.Context, src string) (image.Image, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("ggui: load image: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return decodeImage(data)
}

// AsyncImageWidget shows an image that is loaded and decoded off the UI
// thread. Build one with AsyncImage.
type AsyncImageWidget struct {
	props       property.Owner
	src         property.Value[string]
	loader      ImageLoader
	maxSize     int
	placeholder Widget
	fallback    Widget
	inner       *ImageWidget
	cache       *imageCache
	h           *imgcache.Handle[*ggfx.Image]
	status      AsyncStatus
	err         error
}

// AsyncImage shows the image at src, which the Loader turns into pixels --
// by default src is a file path. The file is read and decoded on a
// goroutine, the Placeholder shows until it is ready, and the decoded
// image is kept in a process-wide cache bounded in bytes (see
// SetImageCacheLimit), so the same src shown twice, or again later, is
// decoded once. The cache is keyed by src and MaxSize, not by loader: give
// each loader srcs of its own.
//
// Once loaded it lays out and paints as Image does, with the same Fit,
// Size, Width, Height, Pixelated and Alt.
//
//	ggui.AsyncImage(photo.Path).MaxSize(512).Size(96, 96).Fit(ggui.FitCover).
//		Placeholder(ui.Skeleton(96, 96))
func AsyncImage(src string) *AsyncImageWidget {
	a := &AsyncImageWidget{loader: LoadImage, inner: Image(nil), cache: defaultImageCache}
	a.src.Set(src)
	a.h = &imgcache.Handle[*ggfx.Image]{}
	// A widget dropped by a rebuild lets go of its image, so the cache may
	// evict it: through its owner when it was built under one, else when
	// it is collected.
	h := a.h
	if reactive.CurrentOwner() != nil {
		OnCleanup(h.Release)
	}
	runtime.AddCleanup(a, func(h *imgcache.Handle[*ggfx.Image]) { h.Release() }, h)
	return a
}

// Src changes the image shown; the load of the previous one is cancelled
// if nothing else is showing it.
func (a *AsyncImageWidget) Src(src string) *AsyncImageWidget {
	if a.src.Set(src) {
		a.props.Changed()
	}
	return a
}

// BindSrc follows r for Src without a rebuild.
func (a *AsyncImageWidget) BindSrc(r Readable[string]) *AsyncImageWidget {
	if a.src.Bind(r, "BindSrc") {
		a.props.Changed()
	}
	return a
}

// Loader replaces how a src becomes an image: from an archive, the
// network, a thumbnailer. nil restores LoadImage.
func (a *AsyncImageWidget) Loader(fn ImageLoader) *AsyncImageWidget {
	if fn == nil {
		fn = LoadImage
	}
	a.loader = fn
	return a
}

// MaxSize downscales an image whose larger side exceeds px pixels to that
// size before it is uploaded, keeping its aspect ratio: a thumbnail of a
// photo costs a thumbnail's memory. 0, the default, keeps the full size.
func (a *AsyncImageWidget) MaxSize(px int) *AsyncImageWidget {
	defer property.Watch(&a.props, &a.maxSize)()
	a.maxSize = max(px, 0)
	return a
}

// Placeholder shows w until the image is ready, and when it fails without
// a Fallback. With a fixed Size the placeholder is given that size.
func (a *AsyncImageWidget) Placeholder(w Widget) *AsyncImageWidget {
	defer property.Watch(&a.props, &a.placeholder)()
	a.placeholder = w
	return a
}

// Fallback shows w in place of an image that failed to load.
func (a *AsyncImageWidget) Fallback(w Widget) *AsyncImageWidget {
	defer property.Watch(&a.props, &a.fallback)()
	a.fallback = w
	return a
}

// Fit sets how the image is placed when the box has another shape.
func (a *AsyncImageWidget) Fit(f ImageFit) *AsyncImageWidget { a.inner.Fit(f); return a }

// Size fixes both dimensions; see ImageWidget.Size.
func (a *AsyncImageWidget) Size(width, height float64) *AsyncImageWidget {
	a.inner.Size(width, height)
	return a
}

// Width fixes the width; the height follows the aspect ratio.
func (a *AsyncImageWidget) Width(v float64) *AsyncImageWidget { a.inner.Width(v); return a }

// Height fixes the height; the width follows the aspect ratio.
func (a *AsyncImageWidget) Height(v float64) *AsyncImageWidget { a.inner.Height(v); return a }

// Pixelated scales the image by its nearest pixel instead of smoothing it.
func (a *AsyncImageWidget) Pixelated(v bool) *AsyncImageWidget { a.inner.Pixelated(v); return a }

// Alt describes the image for a screen reader; see ImageWidget.Alt.
func (a *AsyncImageWidget) Alt(s string) *AsyncImageWidget { a.inner.Alt(s); return a }

// Status reports whether the image is still loading, shown, or failed, as
// of the last layout.
func (a *AsyncImageWidget) Status() AsyncStatus { return a.status }

// Err is why the image failed to load, when Status is Failed.
func (a *AsyncImageWidget) Err() error { return a.err }

// sync makes the handle hold the entry for the current src and reads its
// state. It runs on the UI thread, in Layout.
func (a *AsyncImageWidget) sync() {
	key := imgcache.Key{Src: a.src.Get(), Max: a.maxSize}
	h := a.h
	if key.Src == "" {
		h.Release()
		a.status, a.err, a.inner.img = Failed, errNoSrc, nil
		return
	}
	if !h.Holds(a.cache, key) {
		post := loopPost(reactive.CurrentOwner())
		if l := runningLoop(); post == nil && l != nil {
			post = l.post
		}
		// The cache keeps notify until the load ends; holding the widget
		// weakly lets a widget dropped meanwhile be collected, and its
		// cleanup cancel the load.
		wa := weak.Make(a)
		notify := func() {
			if a := wa.Value(); a != nil {
				a.props.Changed()
			}
		}
		h.Acquire(a.cache, key, imgcache.Loader(a.loader), imgcache.Waiter{Post: post, Notify: notify})
	}
	var st imgcache.Status
	st, a.inner.img, a.err = h.State()
	a.status = asyncStatus(st)
}

// Layout implements Widget.
func (a *AsyncImageWidget) Layout(c Constraints, env Env) Size {
	defer a.props.Layout().EndLayout()
	a.inner.props.Read()
	a.sync()
	if a.status == Ready {
		return a.inner.Layout(c, env)
	}
	w, h := a.inner.width, a.inner.height
	if p := a.shown(); p != nil {
		if w > 0 && h > 0 {
			c = Tight(c.Constrain(Sz(w, h)))
		}
		return p.Layout(c, env)
	}
	return c.Constrain(Sz(w, h))
}

// shown is the widget painted in place of the image, or nil.
func (a *AsyncImageWidget) shown() Widget {
	if a.status == Failed && a.fallback != nil {
		return a.fallback
	}
	return a.placeholder
}

// Paint implements Widget.
func (a *AsyncImageWidget) Paint(dst *Canvas, r Rect) {
	if a.status == Ready {
		a.inner.Paint(dst, r)
		return
	}
	if a.inner.alt != "" {
		dst.Leaf(r, Node{Role: RoleImage, Name: a.inner.alt})
	}
	if p := a.shown(); p != nil {
		dst.Paint(p, r)
	}
}

// SetImageCacheLimit bounds the memory the AsyncImage cache keeps for
// images nothing shows, in bytes of decoded RGBA pixels; the least
// recently shown go first, and are deallocated. Images on screen are never
// evicted, whatever they cost. The default is 256 MiB; 0 keeps nothing that
// is not shown. Call it on the UI thread.
func SetImageCacheLimit(bytes int64) { defaultImageCache.SetLimit(bytes) }

var defaultImageCache = newImageCache(256 << 20)

// errNoSrc is the error of an AsyncImage given no src.
var errNoSrc = errors.New("ggui: AsyncImage has no src")

// imageCache is the AsyncImage cache, of textures on the GPU.
type imageCache = imgcache.Cache[*ggfx.Image]

func newImageCache(limit int64) *imageCache {
	return imgcache.New(limit, uploadImage, (*ggfx.Image).Deallocate)
}

// uploadImage makes a texture of a decoded image, and says what it costs
// in bytes of RGBA pixels.
func uploadImage(src image.Image) (*ggfx.Image, int64) {
	img := newTexture(src)
	b := img.Bounds()
	return img, int64(b.Dx()) * int64(b.Dy()) * 4
}

// asyncStatus is the AsyncStatus of a cache entry's.
func asyncStatus(s imgcache.Status) AsyncStatus {
	switch s {
	case imgcache.Ready:
		return Ready
	case imgcache.Failed:
		return Failed
	}
	return Pending
}
