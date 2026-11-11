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
	"github.com/ironpark/ggui/internal/framestats"
	"github.com/ironpark/ggui/internal/imgcache"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/render"
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
	// source is the image's natural size when img is a smaller copy of
	// it, as an AsyncImage decoded for its display is; zero for img's own.
	source Size
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
	if w.source != (Size{}) {
		return w.source
	}
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
	dst.drawImageAs(w.img, w.natural(), r, ImageOptions{Fit: w.fit, Pixelated: w.pixelated})
}

// imageBox is the most room an image may be drawn in, in logical pixels,
// which an AsyncImage decodes it for. Without Cover the image is fitted
// inside W×H, and an infinite side sets no limit; with Cover it must cover
// W×H, and a side of 0 asks for nothing.
type imageBox struct {
	W, H  float64
	Cover bool
}

// drawBound is the box Layout under c can draw the image in at most.
//
// A side the image is sized to, or that c fixes, is known; another is at
// most what c allows. FitCover and FitFill must cover the known sides,
// and the least c asks of the others; with no side known, the box keeps
// the image's own shape, so fitting inside c's bounds is what covering it
// takes too. FitNone draws the image at its natural size, so it needs all
// of it, and no box limits it.
func (w *ImageWidget) drawBound(c Constraints) imageBox {
	if w.fit == FitNone {
		return imageBox{W: math.Inf(1), H: math.Inf(1)}
	}
	side := func(fixed, lo, hi float64) (float64, bool) {
		if fixed > 0 {
			return min(max(fixed, lo), hi), true
		}
		return hi, lo == hi
	}
	bw, wKnown := side(w.width, c.MinW, c.MaxW)
	bh, hKnown := side(w.height, c.MinH, c.MaxH)
	if (w.fit == FitCover || w.fit == FitFill) && (wKnown || hKnown) {
		if !wKnown {
			bw = c.MinW
		}
		if !hKnown {
			bh = c.MinH
		}
		return imageBox{W: bw, H: bh, Cover: true}
	}
	return imageBox{W: bw, H: bh}
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
	if img == nil {
		return
	}
	b := img.Bounds()
	c.drawImageAs(img, Sz(float64(b.Dx()), float64(b.Dy())), r, o)
}

// drawImageAs draws img as DrawImage does, placed as an image of natural
// size n: img may be a smaller copy of the image, which covers the same
// Rect.
func (c *Canvas) drawImageAs(img *ggfx.Image, n Size, r Rect, o ImageOptions) {
	if c == nil || c.Image == nil || img == nil || o.Fade >= 1 {
		return
	}
	if img.Bounds().Empty() {
		return
	}
	at := o.Fit.Place(n, r)
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
	// A turned image reaches as far as its corners do at this angle: no
	// further than r when hardly turned, so that a spinner turning from
	// upright stays within what the frame before it drew.
	reach := 0.0
	if o.Rotation != 0 {
		sin, cos := math.Abs(math.Sin(o.Rotation)), math.Abs(math.Cos(o.Rotation))
		w, h := r.Size.W, r.Size.H
		reach = max((w*cos+h*sin-w)/2, (w*sin+h*cos-h)/2, 0)
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
	return render.NewTexture(img), nil
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
	maxSize     int // a MaxSize, 0 to decode for the display, or fullSize
	placeholder Widget
	fallback    Widget
	inner       *ImageWidget
	cache       *imageCache
	hs          *asyncHandles
	box         imageBox // the room the last layout gave the image
	scale       float64  // device pixels per logical pixel where it was last painted
	waiter      imgcache.Waiter
	status      AsyncStatus
	err         error
}

// fullSize is the maxSize of an AsyncImage decoded at its own size.
const fullSize = -1

// asyncHandles are an AsyncImage's holds on the cache: cur, the image it
// shows, and next, a copy for another size that is loading meanwhile, with
// the Keys they hold. They are apart from the widget so that collecting it
// can release them.
type asyncHandles struct {
	cur, next       *imgcache.Handle[*ggfx.Image]
	curKey, nextKey imgcache.Key // nextKey is the zero Key for nothing
	source          Size         // the size cur's image was loaded at, once read
}

// release lets go of both entries; it may run on the collector's
// goroutine, so it leaves the Keys, which only the UI thread touches.
func (h *asyncHandles) release() {
	h.next.Release()
	h.cur.Release()
}

// clear lets go of both entries and forgets their Keys.
func (h *asyncHandles) clear() {
	h.release()
	h.setCur(imgcache.Key{})
	h.nextKey = imgcache.Key{}
}

// setCur records that cur now holds k.
func (h *asyncHandles) setCur(k imgcache.Key) { h.curKey, h.source = k, Size{} }

// dropNext lets go of the copy next was loading.
func (h *asyncHandles) dropNext() {
	h.next.Release()
	h.nextKey = imgcache.Key{}
}

// swap makes the copy next loaded the one shown, and lets go of the other.
func (h *asyncHandles) swap() {
	h.cur, h.next = h.next, h.cur
	h.setCur(h.nextKey)
	h.dropNext()
}

// AsyncImage shows the image at src, which the Loader turns into pixels --
// by default src is a file path. The file is read and decoded on a
// goroutine, the Placeholder shows until it is ready, and the decoded
// image is kept in a process-wide cache bounded in bytes (see
// SetImageCacheLimit), so the same src shown twice, or again later, is
// decoded once. The cache is keyed by src and the size it was decoded at,
// not by loader: give each loader srcs of its own.
//
// The image is decoded no larger than it is shown, in the screen's pixels:
// a photo of 6000×4000 shown at 200×200 on a screen of two pixels to the
// point costs a texture of about 512×341, not one of 96 MB. The size is
// rounded up to a power of two, so that resizing it a little, or
// animating it, does not decode it again; shown larger than that, a larger
// copy is decoded while the one it has stays on screen. MaxSize fixes the
// size instead, and FullResolution keeps the image's own.
//
// Once loaded it lays out and paints as Image does, with the same Fit,
// Size, Width, Height, Pixelated and Alt.
//
//	ggui.AsyncImage(photo.Path).Size(96, 96).Fit(ggui.FitCover).
//		Placeholder(ui.Skeleton(96, 96))
func AsyncImage(src string) *AsyncImageWidget {
	a := &AsyncImageWidget{loader: LoadImage, inner: Image(nil), cache: defaultImageCache}
	a.src.Set(src)
	a.hs = &asyncHandles{cur: &imgcache.Handle[*ggfx.Image]{}, next: &imgcache.Handle[*ggfx.Image]{}}
	// A widget dropped by a rebuild lets go of its image, so the cache may
	// evict it: through its owner when it was built under one, else when
	// it is collected.
	hs := a.hs
	if reactive.CurrentOwner() != nil {
		OnCleanup(hs.release)
	}
	runtime.AddCleanup(a, func(hs *asyncHandles) { hs.release() }, hs)
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
// size before it is uploaded, keeping its aspect ratio, whatever size it
// is shown at; the image then lays out at that smaller size, as an Image
// of it would. 0, the default, decodes it for the size it is shown at
// instead, and lays it out at its own size.
func (a *AsyncImageWidget) MaxSize(px int) *AsyncImageWidget {
	defer property.Watch(&a.props, &a.maxSize)()
	a.maxSize = max(px, 0)
	return a
}

// FullResolution decodes the image at its own size whatever it is shown
// at, for one that is zoomed into, or drawn larger than its layout. MaxSize
// undoes it.
func (a *AsyncImageWidget) FullResolution() *AsyncImageWidget {
	defer property.Watch(&a.props, &a.maxSize)()
	a.maxSize = fullSize
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

// The sides an image decoded for its display is rounded up to are powers
// of two from minBucket; past maxBucket it is decoded at its own size.
const (
	minBucket = 64
	maxBucket = 1 << 14
)

// bucket is the side of n device pixels rounded up to a power of two. A
// side with no bound is 0, as is one asking for nothing of a covering box.
func bucket(n float64, cover bool) int {
	if math.IsInf(n, 0) || math.IsNaN(n) || n > maxBucket || cover && n <= 0 {
		return 0
	}
	b := minBucket
	for float64(b) < n {
		b <<= 1
	}
	return b
}

// displayKey is the Key of src decoded for the box b on a canvas of scale
// device pixels per logical pixel.
func displayKey(src string, b imageBox, scale float64) imgcache.Key {
	return imgcache.Key{Src: src, W: bucket(b.W*scale, b.Cover), H: bucket(b.H*scale, b.Cover), Cover: b.Cover}
}

// grownKey is the Key an image decoded for cur should move to when it is
// now shown as want asks. It moves only when want needs more pixels on a
// side, and then to enough for both; and when every side needs a quarter
// or less of what it has, a sixteenth of the memory, to want. Anything in
// between keeps cur, so that an image shrunk for a moment, or measured
// with other bounds, is not decoded again.
func grownKey(cur, want imgcache.Key) imgcache.Key {
	if cur.Src != want.Src || cur.Cover != want.Cover {
		return want
	}
	cw, ch := keySides(cur)
	ww, wh := keySides(want)
	if cw >= ww && ch >= wh { // cur holds want
		if ww <= cw/4 && wh <= ch/4 {
			return want
		}
		return cur
	}
	bound := func(n int) int { // back to a Key's side
		if n == math.MaxInt {
			return 0
		}
		return n
	}
	return imgcache.Key{Src: want.Src, W: bound(max(cw, ww)), H: bound(max(ch, wh)), Cover: want.Cover}
}

// keySides are k's sides in pixels, math.MaxInt for one without a limit: a
// side of 0 sets none when fitting inside, and asks for nothing when
// covering.
func keySides(k imgcache.Key) (w, h int) {
	side := func(n int) int {
		if n == 0 && !k.Cover {
			return math.MaxInt
		}
		return n
	}
	return side(k.W), side(k.H)
}

// fixedKey is the Key MaxSize or FullResolution fix for src, and false
// when the image is decoded for its display.
func (a *AsyncImageWidget) fixedKey(src string) (imgcache.Key, bool) {
	switch {
	case a.maxSize > 0:
		return imgcache.Max(src, a.maxSize), true
	case a.maxSize == fullSize:
		return imgcache.Key{Src: src}, true
	}
	return imgcache.Key{}, false
}

// hostScale is the device pixels per logical pixel of the window or probe
// being laid out, 1 when there is none.
func hostScale() float64 {
	l := loopOf(reactive.CurrentOwner())
	if l == nil {
		l = runningLoop()
	}
	if l != nil {
		switch h := l.host.(type) {
		case *Window:
			return h.canvas.Scale()
		case *Probe:
			return h.canvas.Scale()
		}
	}
	return 1
}

// sync makes the handles hold the entries for the current src and reads
// what to show. It runs on the UI thread, in Layout.
//
// A new src is asked for at once, sized for the room c gives it, so that
// one already cached shows in this very frame. A change of size for the
// same src waits for Paint, which knows the room the image was finally
// given; Layout may be asked to measure it under other bounds first.
func (a *AsyncImageWidget) sync(c Constraints) {
	src := a.src.Get()
	a.box = a.inner.drawBound(c)
	if src == "" {
		a.hs.clear()
		a.status, a.err, a.inner.img, a.inner.source = Failed, errNoSrc, nil, Size{}
		return
	}
	want, fixed := a.fixedKey(src)
	if !fixed {
		scale := a.scale
		if scale == 0 {
			scale = hostScale()
		}
		want = displayKey(src, a.box, scale)
	}
	switch {
	case !fixed && a.hs.curKey.Src != src:
		a.adopt(want)
	case fixed && a.hs.curKey != want:
		a.retarget(want)
	default:
		a.promote()
	}
	a.read()
}

// adopt starts showing a new src decoded for its display, as want asks.
// When the cache has no entry for want but has the image ready at another
// size, as when it is shown elsewhere or this widget was built anew, that
// copy is used instead of loading another: as it is, if it has the pixels
// want asks for, or as a stand-in on screen while want loads.
func (a *AsyncImageWidget) adopt(want imgcache.Key) {
	hs := a.hs
	hs.dropNext()
	if !a.cache.Has(want) {
		holds := func(k imgcache.Key) bool { return grownKey(k, want) == k }
		if k, ok := hs.cur.AcquireReady(a.cache, want.Src, holds); ok {
			hs.setCur(k)
			if holds(k) {
				return
			}
		}
	}
	a.retarget(want)
}

// retarget moves the image to the entry for k. While the one shown is
// ready it stays on screen, and k loads in next until promote can swap
// them; otherwise k is loaded in its place.
func (a *AsyncImageWidget) retarget(k imgcache.Key) {
	hs := a.hs
	if k == hs.curKey {
		hs.dropNext()
		return
	}
	if st, _, _, ok := hs.cur.Get(a.cache, hs.curKey); !ok || st != imgcache.Ready || k.Src != hs.curKey.Src {
		hs.dropNext()
		hs.setCur(k)
		hs.cur.Acquire(a.cache, k, imgcache.Loader(a.loader), a.notifier())
		return
	}
	if _, _, _, ok := hs.next.Get(a.cache, k); !ok {
		hs.next.Acquire(a.cache, k, imgcache.Loader(a.loader), a.notifier())
	}
	hs.nextKey = k
	a.promote()
}

// promote swaps in the copy next was loading, once it is ready. One that
// failed stays held, so that it is not asked for again while the image it
// was to replace still shows.
func (a *AsyncImageWidget) promote() {
	hs := a.hs
	if hs.nextKey == (imgcache.Key{}) {
		return
	}
	if st, _, _, ok := hs.next.Get(a.cache, hs.nextKey); !ok || st != imgcache.Ready {
		return
	}
	hs.swap()
}

// read takes what the entry shown has, asking for it again when the
// cache let go of it.
func (a *AsyncImageWidget) read() {
	hs := a.hs
	st, img, err, ok := hs.cur.Get(a.cache, hs.curKey)
	if !ok {
		st, img, err = hs.cur.Acquire(a.cache, hs.curKey, imgcache.Loader(a.loader), a.notifier())
	}
	a.inner.img, a.err = img, err
	a.status = asyncStatus(st)
	a.inner.source = Size{}
	if a.status == Ready && a.maxSize <= 0 {
		// A copy decoded for the display lays out as the image does. The
		// size it was loaded at is the same for every copy of the src, so
		// it is asked for once per Key.
		if hs.source == (Size{}) {
			s := hs.cur.Source()
			hs.source = Sz(float64(s.X), float64(s.Y))
		}
		a.inner.source = hs.source
	}
}

// notifier is how the cache tells the widget a load ended.
func (a *AsyncImageWidget) notifier() imgcache.Waiter {
	if a.waiter.Notify != nil {
		return a.waiter
	}
	post := loopPost(reactive.CurrentOwner())
	if l := runningLoop(); post == nil && l != nil {
		post = l.post
	}
	if post == nil {
		return imgcache.Waiter{}
	}
	// The cache keeps notify until the load ends; holding the widget
	// weakly lets a widget dropped meanwhile be collected, and its cleanup
	// cancel the load.
	wa := weak.Make(a)
	a.waiter = imgcache.Waiter{Post: post, Notify: func() {
		if a := wa.Value(); a != nil {
			a.props.Changed()
		}
	}}
	return a.waiter
}

// Layout implements Widget.
func (a *AsyncImageWidget) Layout(c Constraints, env Env) Size {
	defer a.props.Layout().EndLayout()
	a.inner.props.Read()
	a.sync(c)
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
	a.resize(dst.Scale())
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

// resize moves an image decoded for its display to the size the last
// layout gave it on a canvas of scale, as grownKey says.
func (a *AsyncImageWidget) resize(scale float64) {
	a.scale = scale
	hs := a.hs
	src := hs.curKey.Src
	if src == "" {
		return
	}
	if _, fixed := a.fixedKey(src); fixed {
		return
	}
	k := grownKey(hs.curKey, displayKey(src, a.box, scale))
	if k == hs.curKey && hs.nextKey == (imgcache.Key{}) {
		return
	}
	before, img := a.status, a.inner.img
	a.retarget(k)
	a.read()
	if a.status != before || a.status == Ready && a.inner.img != img {
		a.props.Changed()
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
	return imgcache.New(limit, uploadImage, freeImage)
}

// uploadImage makes a texture of a decoded image, and says what it costs
// in bytes of RGBA pixels. The cache hands it RGBA pixels, which it only
// copies.
func uploadImage(src image.Image) (*ggfx.Image, int64) {
	img := render.NewTexture(src)
	b := img.Bounds()
	bytes := framestats.ImageBytes(b.Dx(), b.Dy())
	framestats.AddTexture(framestats.Images, bytes)
	return img, bytes
}

// freeImage deallocates a texture the cache dropped.
func freeImage(img *ggfx.Image) {
	b := img.Bounds()
	framestats.AddTexture(framestats.Images, -framestats.ImageBytes(b.Dx(), b.Dy()))
	img.Deallocate()
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
