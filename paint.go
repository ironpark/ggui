package ggui

import (
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/vector"
	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/framestats"
	"github.com/ironpark/ggui/internal/render"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
)

// Drawing on a Canvas. The rectangles, circles and lines a widget draws
// most go first: a rectangle as one batched image, the rest as signed
// distance fields the render package shades. Paths, gradients, clips to a
// shape and layers follow. Each culls what lands outside the image, while
// noting where it would have drawn for a frame painted in part. A line of
// text drawn by hand comes last.

// drewPath notes path, with extra around it, as drawn, in a frame painted
// in part: that one painted where a widget drew must catch what it now
// draws past its Rect. A whole frame skips the walk over every point of the
// path, which a chart's would make cost as much as drawing it: what a widget
// drew past its Rect there shows up as the frame painted in part after it
// finds the widget drawing past what it painted, and paints whole.
func (c *Canvas) drewPath(path *Path, extra float64) {
	if f := c.fs(); f.partial {
		f.drew(path.Bounds(), extra)
	}
}

// visiblePaintBounds tests drawing only; callers must still collect input,
// semantics and traces. Keep one physical pixel for antialiasing and round to
// float32 like the vector API. Unusual geometry takes the original draw path.
func (c *Canvas) visiblePaintBounds(r Rect, extra float64) bool {
	if c == nil {
		return false
	}
	// Seen or not, it is where the widget painting draws.
	c.fs().drew(r, extra)
	return c.overlaps(r, extra)
}

// Draws notes r, and as far as reach past it, as where the widget painting
// draws, and reports whether any of that lands where this frame paints: a
// widget that draws on Image itself, glyphs through a text engine say,
// skips what lies wholly outside, line by line, and still says where it
// would have drawn, which a frame painted in part needs to catch what it now
// draws past its Rect. A draw at the edge of what is painted counts, with a
// physical pixel to spare for antialiasing. Like Shows, it decides drawing
// only: the widget still collects input and semantics.
func (c *Canvas) Draws(r Rect, reach float64) bool { return c.visiblePaintBounds(r, reach) }

// Shows reports whether any of r, in the canvas's coordinates, lands where
// this frame paints. A frame painted in part paints only around what
// changed: a widget whose drawing costs more than the draws themselves,
// copying a screen out of a terminal say, need not do that work for a
// frame that keeps what it showed. It must still collect input and
// semantics, as all widgets do.
//
// Only r itself counts, not the pixel drawing at its edge may smear past
// it, and r's edge on the edge of what is painted does not.
func (c *Canvas) Shows(r Rect) bool {
	if c == nil || c.Image == nil {
		return false
	}
	return c.physical(r).Overlaps(c.Image.Bounds())
}

func (c *Canvas) overlaps(r Rect, extra float64) bool {
	if c.Image == nil {
		return false
	}
	bounds := c.Image.Bounds()
	if bounds.Empty() {
		return false
	}
	x, y, w, h := c.Px(r.Origin.X), c.Px(r.Origin.Y), c.Px(r.Size.W), c.Px(r.Size.H)
	pad := c.px(extra) + 1
	// Avoid culling huge coordinates where float32 intermediate rounding
	// can exceed the one-pixel guard.
	for _, v := range [...]float64{float64(x), float64(y), float64(w), float64(h), pad} {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1<<20 {
			return true
		}
	}
	if w < 0 || h < 0 || pad < 1 {
		return true
	}
	return float64(x+w)+pad > float64(bounds.Min.X) && float64(y+h)+pad > float64(bounds.Min.Y) &&
		float64(x)-pad < float64(bounds.Max.X) && float64(y)-pad < float64(bounds.Max.Y)
}

// snapPx rounds a physical pixel coordinate to the nearest pixel boundary.
// Using Floor(v+0.5) instead of math.Round keeps rounding translation-invariant
// across zero so a 1px band centered on 0 stays 1px wide rather than 2px.
func snapPx(v float64) float64 { return math.Floor(v + 0.5) }

func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// snapFillRect converts logical r to integer physical pixel coordinates so
// axis-aligned fills land on the pixel grid without anti-aliasing: adjacent
// rects share snapped boundaries with no seams or overlaps, and any positive
// logical width or height remains at least one physical pixel wide.
func (c *Canvas) snapFillRect(r Rect) (x, y, w, h float32, ok bool) {
	if !(r.Size.W > 0) || !(r.Size.H > 0) {
		return 0, 0, 0, 0, false
	}
	x0, x1 := snapPx(c.px(r.Origin.X)), snapPx(c.px(r.Origin.X+r.Size.W))
	y0, y1 := snapPx(c.px(r.Origin.Y)), snapPx(c.px(r.Origin.Y+r.Size.H))
	pw, ph := max(x1-x0, 1), max(y1-y0, 1)
	if !isFinite(x0) || !isFinite(y0) || !isFinite(pw) || !isFinite(ph) {
		return 0, 0, 0, 0, false
	}
	return float32(x0), float32(y0), float32(pw), float32(ph), true
}

// snapAxisLine converts a horizontal or vertical stroke of logical width w to
// integer physical pixel coordinates. Endpoints snap so collinear segments join
// cleanly, while thickness snaps from w alone so parallel lines of the same
// logical width keep a uniform physical pixel width at fractional scales.
func (c *Canvas) snapAxisLine(start, end, cross, w float64, horizontal bool) (x, y, pw, ph float32, ok bool) {
	if !(w > 0) || start == end {
		return 0, 0, 0, 0, false
	}
	p0, p1 := snapPx(c.px(min(start, end))), snapPx(c.px(max(start, end)))
	length := max(p1-p0, 1)
	thick := max(snapPx(c.px(w)), 1)
	c0 := snapPx(c.px(cross) - thick/2)
	if !isFinite(p0) || !isFinite(length) || !isFinite(c0) || !isFinite(thick) {
		return 0, 0, 0, 0, false
	}
	if horizontal {
		return float32(p0), float32(c0), float32(length), float32(thick), true
	}
	return float32(c0), float32(p0), float32(thick), float32(length), true
}

// FillRect fills the logical Rect r with col.
//
// A rectangle is drawn without anti-aliasing, which its axis-aligned edges
// have no use for. That matters more than it sounds: ggfx draws an
// anti-aliased shape by rendering it eight times into an offscreen stencil
// buffer the size of the shape and compositing the result, which breaks the
// batch both ways. A plain rectangle is instead one batched image, and most
// of the large fills in a frame are rectangles: table rows, sidebars, text
// selection, a modal's scrim.
func (c *Canvas) FillRect(r Rect, col color.Color) {
	if c == nil || c.Image == nil || col == nil || !(r.Size.W > 0) || !(r.Size.H > 0) {
		return
	}
	if !c.visiblePaintBounds(r, 0) {
		return
	}
	if x, y, w, h, ok := c.snapFillRect(r); ok {
		vector.FillRect(c.Image, x, y, w, h, col, false)
	}
}

// FillCircle fills a circle of logical radius around center with col.
func (c *Canvas) FillCircle(center Point, radius float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || radius <= 0 {
		return
	}
	r := Rct(center.Add(Pt(-radius, -radius)), Sz(2*radius, 2*radius))
	if !c.visiblePaintBounds(r, 0) {
		return
	}
	// A circle is a rounded rectangle whose corner is its own half size.
	c.shadeRoundRect(r, radius, 0, col)
}

// StrokeLine draws a line of logical width w from a to b in col. The ends
// are cut square, so a line drawn in pieces joins up.
func (c *Canvas) StrokeLine(a, b Point, w float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || !(w > 0) || c.Image.Bounds().Empty() || a == b {
		return
	}
	if !c.visiblePaintBounds(Rct(Pt(min(a.X, b.X), min(a.Y, b.Y)), Sz(math.Abs(b.X-a.X), math.Abs(b.Y-a.Y))), w/2) {
		return
	}
	// A line along an axis is a rectangle, and a rectangle draws as one
	// batched image rather than a shaded quad of its own. Rules, table
	// borders and the straight runs of a dashed outline are all this.
	switch {
	case a.Y == b.Y:
		if x, y, pw, ph, ok := c.snapAxisLine(a.X, b.X, a.Y, w, true); ok {
			vector.FillRect(c.Image, x, y, pw, ph, col, false)
		}
	case a.X == b.X:
		if x, y, pw, ph, ok := c.snapAxisLine(a.Y, b.Y, a.X, w, false); ok {
			vector.FillRect(c.Image, x, y, pw, ph, col, false)
		}
	default:
		c.shadeSegment(a, b, w, col)
	}
}

// FillRoundRect fills the logical Rect r with col, with corners rounded by
// radius. A zero radius is FillRect.
func (c *Canvas) FillRoundRect(r Rect, radius float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || !(r.Size.W > 0) || !(r.Size.H > 0) {
		return
	}
	if !c.visiblePaintBounds(r, 0) {
		return
	}
	if radius <= 0 {
		c.FillRect(r, col)
		return
	}
	c.shadeRoundRect(r, radius, 0, col)
}

// StrokeRoundRect draws a line of logical width w in col just inside r,
// with corners rounded by radius.
func (c *Canvas) StrokeRoundRect(r Rect, radius, w float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || w <= 0 || c.Image.Bounds().Empty() {
		return
	}
	if w <= r.Size.W && w <= r.Size.H && !c.visiblePaintBounds(r, w) {
		return
	}
	// A border as thick as the box it outlines leaves no hole to draw
	// around, so it is that box filled.
	if w >= r.Size.W || w >= r.Size.H {
		c.FillRoundRect(r, radius, col)
		return
	}
	// The border straddles a rectangle inset by half its width, which is
	// what keeps it just inside r.
	inset := Rct(r.Origin.Add(Pt(w/2, w/2)), Sz(r.Size.W-w, r.Size.H-w))
	c.shadeRoundRect(inset, max(radius-w/2, 0), w/2, col)
}

func pathOptions(col color.Color) *vector.DrawPathOptions {
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(col)
	return op
}

// The shapes below are drawn by the render package from signed distance
// fields; these methods turn logical coordinates into the Image's pixels.

// shadeRoundRect draws the rounded rectangle shape in col. A zero halfStroke
// fills it; otherwise shape is the centreline of a border of twice that
// width. Both are logical, as radius is.
func (c *Canvas) shadeRoundRect(shape Rect, radius, halfStroke float64, col color.Color) {
	scale := c.Scale()
	if math.IsNaN(scale) || math.IsInf(scale, 0) {
		return
	}
	render.Shade(c.Image, render.Field{
		Centre: Pt(c.px(shape.Origin.X+shape.Size.W/2), c.px(shape.Origin.Y+shape.Size.H/2)),
		Axis:   Pt(1, 0),
		Half:   Sz(shape.Size.W*scale/2, shape.Size.H*scale/2),
		Radius: radius * scale,
		Stroke: halfStroke * scale,
	}, col)
}

// shadeSegment draws the line from a to b, w wide, in col. The ends are cut
// square, as the vector renderer's default cap is.
func (c *Canvas) shadeSegment(a, b Point, w float64, col color.Color) {
	scale := c.Scale()
	if math.IsNaN(scale) || math.IsInf(scale, 0) {
		return
	}
	dx, dy := c.px(b.X-a.X), c.px(b.Y-a.Y)
	length := math.Hypot(dx, dy)
	if length == 0 || math.IsNaN(length) || math.IsInf(length, 0) {
		return
	}
	render.Shade(c.Image, render.Field{
		Centre: Pt(c.px((a.X+b.X)/2), c.px((a.Y+b.Y)/2)),
		Axis:   Pt(dx/length, dy/length),
		Half:   Sz(length/2, w*scale/2),
	}, col)
}

// maskRoundRect draws img, a part of a layer, onto c where it lies within
// the logical Rect r with corners rounded by radius.
func (c *Canvas) maskRoundRect(img *ggfx.Image, r Rect, radius float64) {
	scale := c.Scale()
	centre := r.Center()
	render.MaskRoundRect(c.Image, img, Pt(c.px(centre.X), c.px(centre.Y)), Sz(r.Size.W*scale/2, r.Size.H*scale/2), radius*scale)
}

// scratch lends fn the part of a cleared layer image within area, the whole
// window when empty: a Layer's, or one more for an effect that needs it.
func (c *Canvas) scratch(area image.Rectangle, fn func(img *ggfx.Image)) {
	f := c.fs()
	img := f.layers.Borrow(c.root().Image.Bounds(), area, newLayerImage)
	defer f.layers.Return()
	if !area.Empty() {
		img = img.SubImage(area).(*ggfx.Image)
	}
	img.Clear()
	fn(img)
}

// LayerOptions says how Layer draws the finished layer. The zero value draws
// it as it was painted.
type LayerOptions struct {
	// Fade is how far the layer fades out: 0 draws it opaque, 1 not at all.
	Fade float64
	// Scale sizes the layer about the logical point About. 0 and 1 leave it
	// at its size.
	Scale float64
	About Point
}

// Layer paints into an offscreen image the size of the window, then draws
// that image onto c as o says: the way to apply an opacity or a transform to
// a subtree as a whole, which a Transition's fade and scale do. paint gets a
// Canvas like c in every respect but the image it draws into, so hit regions,
// clipping, focus traps and inertness are what they would be without the
// layer. Layers nest.
//
// The image is the window's, not the caller's: it goes back when paint
// returns, the next Layer reuses it, and one no Layer needed during a frame
// is freed when the frame ends. A widget holds no texture of its own, so
// nothing is left for the garbage collector when it goes away.
//
// Without an image to draw into, Layer calls paint with c.
func (c *Canvas) Layer(o LayerOptions, paint func(layer *Canvas)) {
	scaled := o.Scale != 0 && o.Scale != 1
	// Unscaled, nothing painted outside the clip can show, so the layer
	// is cleared and composited over the clip alone.
	var area image.Rectangle
	if !scaled && c != nil && c.Image != nil {
		area = c.Image.Bounds()
	}
	c.layer(area, paint, func(img *ggfx.Image) {
		op := &ggfx.DrawImageOptions{}
		// A source is drawn from its corner, a destination's coordinates
		// are the window's.
		op.GeoM.Translate(float64(img.Bounds().Min.X), float64(img.Bounds().Min.Y))
		if scaled {
			op.Filter = ggfx.FilterLinear
			cx, cy := c.px(o.About.X), c.px(o.About.Y)
			op.GeoM.Translate(-cx, -cy)
			op.GeoM.Scale(o.Scale, o.Scale)
			op.GeoM.Translate(cx, cy)
		}
		if o.Fade != 0 {
			op.ColorScale.ScaleAlpha(float32(1 - o.Fade))
		}
		c.Image.DrawImage(img, op)
	})
}

// ClipRoundRect is Clip with the corners of r rounded by radius, antialiased,
// for a subtree: a card whose content runs to its rounded edge, say. paint
// draws into a Layer covering r alone, which is composited through the
// rounded shape. Hit regions are clipped to r as a rectangle. A zero radius
// is Clip. A single image is cheaper drawn with ImageOptions.Radius.
func (c *Canvas) ClipRoundRect(r Rect, radius float64, paint func(dst *Canvas)) {
	clip := c.Clip(r)
	if radius <= 0 || clip == nil || clip.Image == nil || clip.Image.Bounds().Empty() {
		paint(clip)
		return
	}
	clip.layer(clip.Image.Bounds(), paint, func(img *ggfx.Image) { clip.maskRoundRect(img, r, radius) })
}

// layer paints into the part of a borrowed layer image within area, the
// whole window when empty, and hands that part to draw. Without an image it
// paints into c.
func (c *Canvas) layer(area image.Rectangle, paint func(*Canvas), draw func(*ggfx.Image)) {
	if c == nil || c.Image == nil || c.root().Image == nil {
		paint(c)
		return
	}
	c.scratch(area, func(img *ggfx.Image) {
		paint(c.into(img))
		draw(img)
	})
}

// newLayerImage makes an image for a layer. Unmanaged, the layer is a
// texture of its own size: on the atlas it would take a page rounded up to
// powers of two, 4096×2048 for a 3252×1534 window, 32 MB rather than 20.
func newLayerImage(b image.Rectangle) *ggfx.Image {
	return ggfx.NewImageWithOptions(b, &ggfx.NewImageOptions{Unmanaged: true})
}

// gaugeLayers counts the bytes of layer images made and freed.
func gaugeLayers(bytes int64) { framestats.AddTexture(framestats.Layers, bytes) }

// trimLayers ends a frame for the layer images: it keeps as many as the frame
// had open at once and frees the rest.
func (c *Canvas) trimLayers() {
	if f := c.frameState; f != nil {
		f.layers.EndFrame()
	}
}

// freeLayers frees every layer image, for an App or Probe that is closing.
func (c *Canvas) freeLayers() {
	if f := c.frameState; f != nil {
		f.layers.FreeAll()
	}
}

// DrawText draws s as one unwrapped line with its top-left corner at the
// logical point at, in font at size logical pixels; a nil font is the
// default. A custom widget's Paint draws a label with it in the style its
// Layout kept from Env.ResolveText, so the label follows the theme; text
// that wraps, selects or is read by a screen reader is a Text painted with
// Canvas.Paint.
func (c *Canvas) DrawText(s string, font *Font, size float64, at Point, col color.Color) {
	if c == nil || c.Image == nil || col == nil {
		return
	}
	if font == nil {
		font = fallbackFont()
	}
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(col)
	op.GeoM.Translate(c.px(at.X), c.px(at.Y))
	fontsys.DrawText(c.Image, s, font.face(c.px(size), 0), op)
}

// TextWidth measures s as DrawText would draw it, in logical pixels.
func (c *Canvas) TextWidth(s string, font *Font, size float64) float64 {
	if font == nil {
		font = fallbackFont()
	}
	return c.dp(textlayout.Width(s, font.face(c.px(size), 0)))
}

// DrawTextStyle is DrawText in a whole TextStyle: its Font, Size, Color
// and Tracking, and its Weight, drawn with the font's face nearest it or
// along the weight axis of a variable font. A label kept from Env.ResolveText keeps its
// weight this way; zero fields fall back as a Text's would.
func (c *Canvas) DrawTextStyle(s string, st TextStyle, at Point) {
	if c == nil || c.Image == nil {
		return
	}
	st = st.resolved()
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(st.Color)
	op.GeoM.Translate(c.px(at.X), c.px(at.Y))
	fontsys.DrawText(c.Image, s, st.face(c.Scale()), op)
}

// DrawTextCells draws s as DrawTextStyle does, but with each grapheme at
// the start of a cell of its own, cellW wide, from at: the layout of a
// terminal or any other grid of monospaced text. A grid's cells are best a
// whole number of device pixels, so that fills meet without seams, while a
// font's advance rarely is (SF Mono is 8.03 pixels wide at 13pt), and text
// drawn at its own advance would drift off the grid along a row. Each
// grapheme is placed on a whole device pixel, so the error stays under
// half a pixel instead of growing with the column. The run is shaped
// once, so it costs what DrawTextStyle does rather than a draw per cell.
func (c *Canvas) DrawTextCells(s string, st TextStyle, at Point, cellW float64) {
	if c == nil || c.Image == nil || c.Image.Bounds().Empty() || s == "" {
		return
	}
	st = st.resolved()
	// A terminal draws a run per row and colour every frame, each a string
	// of its own: the run's graphemes, glyphs, shifts and images are kept
	// in buffers it reuses.
	buf := theCellsBufPool.Get()
	defer func() {
		clear(buf.glyphs)
		clear(buf.entries)
		theCellsBufPool.Put(buf)
	}()
	buf.bounds = textedit.AppendBounds(buf.bounds[:0], s)
	// A grid sets its own advance, so a cell's glyph is the font's, with
	// no tracking.
	face := st.Font.face(c.px(st.Size), st.Weight)
	face = fontsys.PlainFace(face, s)
	if face == nil {
		// An emoji is drawn from a font of its own, with its own
		// metrics: one grapheme at a time keeps it in its cell.
		x := at.X
		for k := 0; k+1 < len(buf.bounds); k++ {
			c.DrawTextStyle(s[buf.bounds[k]:buf.bounds[k+1]], st, Pt(x, at.Y))
			x += cellW
		}
		return
	}

	var tint, colored ggfx.ColorScale
	tint.ScaleWithColor(st.Color)
	colored.ScaleAlpha(tint.A())
	x0, y0, cell := c.px(at.X), c.px(at.Y), c.px(cellW)

	buf.glyphs = text.AppendLazyGlyphs(buf.glyphs[:0], s, face, nil)
	buf.shifts = cellShifts(buf.shifts, buf.bounds, buf.glyphs, x0, cell)
	// Realize every image before drawing any, as text.Draw does, so the
	// atlas is written in one batch, then draw those on one atlas page as
	// one.
	buf.entries = buf.entries[:0]
	for i, g := range buf.glyphs {
		if g.ImageBounds.Empty() {
			continue
		}
		if img := g.Image(); img != nil {
			e := ggfx.ImageDraw{Image: img, X: x0 + (float64(g.ImageBounds.Min.X) + buf.shifts[i]), Y: y0 + float64(g.ImageBounds.Min.Y), ColorScale: tint}
			if g.Colored() {
				e.ColorScale = colored
			}
			buf.entries = append(buf.entries, e)
		}
	}
	c.Image.DrawImages(buf.entries, nil)
}

// cellsBuf holds what DrawTextCells works out for a run between runs.
type cellsBuf struct {
	bounds  []int
	glyphs  []text.LazyGlyph
	shifts  []float64
	entries []ggfx.ImageDraw
}

var theCellsBufPool = gem.Pool[*cellsBuf]{
	New: func() *cellsBuf { return &cellsBuf{} },
}

// cellShifts is how far each glyph moves from where its run shaped it, in
// device pixels, to start its grapheme's cell: the kth grapheme, from
// bounds[k] to bounds[k+1] of the run as textedit.Bounds gives them, has
// its first glyph at the whole pixel nearest x0+k·cell, and the rest of a
// grapheme, its marks or the parts of a ligature, move with it. It is kept
// in dst's storage when that has room.
func cellShifts(dst []float64, bounds []int, glyphs []text.LazyGlyph, x0, cell float64) []float64 {
	shifts := slices.Grow(dst[:0], len(glyphs))[:len(glyphs)]
	k, shift := -1, 0.0
	for i, g := range glyphs {
		if g.StartIndexInBytes >= bounds[k+1] {
			for k+1 < len(bounds)-1 && g.StartIndexInBytes >= bounds[k+1] {
				k++
			}
			shift = math.Round(x0+float64(k)*cell) - x0 - g.OriginX
		}
		shifts[i] = shift
	}
	return shifts
}

// TextStyleWidth measures s as DrawTextStyle would draw it, in logical
// pixels.
func (c *Canvas) TextStyleWidth(s string, st TextStyle) float64 {
	st = st.resolved()
	return c.dp(textlayout.Width(s, st.face(c.Scale())))
}

// FitText returns s as it fits in width at size, cut with "…" when it is
// too wide: what Text's Ellipsis draws, for a widget drawing its own.
func (c *Canvas) FitText(s string, font *Font, size, width float64) string {
	if font == nil {
		font = fallbackFont()
	}
	return textlayout.Ellipsize(s, font.face(c.px(size), 0), c.px(width), false)
}

// Physical is r in the pixels of Image, for an Overlay that keeps an image
// of its own the size of what it draws.
func (c *Canvas) Physical(r Rect) image.Rectangle { return c.physical(r) }
