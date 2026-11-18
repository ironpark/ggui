package record

import "image/color"

// An Option changes how WriteGIF or WritePNGs writes a tour.
type Option func(*settings)

// settings are the options given, resolved.
type settings struct {
	fps        int
	width      int
	colors     int
	dither     float64
	loop       int
	background color.Color

	frame    *FrameStyle // the Frame option's, or nil for the Tour's
	radius   float64     // the window's corner radius, logical; negative is DefaultCornerRadius
	margin   float64     // around the window, logical; negative is DefaultMargin
	backdrop color.Color // nil is DefaultBackdrop
	noCursor bool
	// flatten draws a transparent backdrop over white, for a GIF, which
	// has no partial transparency.
	flatten bool
}

func resolve(opts []Option) *settings {
	s := &settings{colors: 256, margin: -1, radius: -1}
	for _, o := range opts {
		o(s)
	}
	return s
}

// FPS keeps n frames for each second of the tour: the frame on screen at
// each 1/n of a second. Zero, or more than the tour draws, keeps every
// frame drawn. A tour drawn at 15 a second and written at 10 plays as
// smoothly as most UI needs and makes a smaller file.
func FPS(n int) Option { return func(s *settings) { s.fps = max(n, 0) } }

// Width scales the frames down to px pixels wide, the height in
// proportion, with an area filter, which keeps text legible. Zero, or a
// width the tour is drawn narrower than, keeps the size drawn.
func Width(px int) Option { return func(s *settings) { s.width = max(px, 0) } }

// Colors limits a GIF's palette, which one set of colors chosen for the
// whole recording makes, to n colors: 2 to 256, the default. Fewer make a
// smaller file and band gradients sooner.
func Colors(n int) Option { return func(s *settings) { s.colors = min(max(n, 2), 256) } }

// Dither mixes a GIF's palette colors in an ordered (Bayer) pattern
// where a color falls between them, strength 0, the default, to 1. Flat
// UI needs none; a little, as 0.3, smooths gradients and shadows that
// band, at some cost in size.
func Dither(strength float64) Option {
	return func(s *settings) { s.dither = min(max(strength, 0), 1) }
}

// Loop makes a GIF play n times. Zero, the default, loops forever.
func Loop(n int) Option { return func(s *settings) { s.loop = max(n, 0) } }

// Background fills each frame with c before the tree is drawn on it, in
// place of the background the tour's Env holds (ggui.Env.Background, which
// a theme sets). A color that is not opaque is shown over white.
func Background(c color.Color) Option { return func(s *settings) { s.background = c } }

// Frame draws f around every frame in place of the Tour's Frame: FrameMac
// draws the content as a macOS window, with a title bar, its buttons, a
// border and a shadow, on a backdrop. The output grows by the frame;
// Width scales the framed image.
func Frame(f FrameStyle) Option { return func(s *settings) { s.frame = &f } }

// CornerRadius rounds a framed window's corners by pt logical pixels in
// place of DefaultCornerRadius, the radius macOS 26 gives a window with a
// title bar: 10 is the radius of the macOS releases before it.
func CornerRadius(pt float64) Option { return func(s *settings) { s.radius = max(pt, 0) } }

// Margin leaves pt logical pixels of backdrop around a framed window, for
// its shadow, in place of DefaultMargin. Less crops the shadow.
func Margin(pt float64) Option { return func(s *settings) { s.margin = max(pt, 0) } }

// Backdrop is what a framed window is drawn on, in place of
// DefaultBackdrop. color.Transparent leaves it clear: PNG frames keep the
// transparency, and the shadow falls on nothing; a GIF, which has no
// partial transparency, is drawn over white.
func Backdrop(c color.Color) Option { return func(s *settings) { s.backdrop = c } }

// Cursor draws the pointer the Recorder's pointer steps move, true by
// default. Cursor(false) leaves it out; the steps still move, press and
// hover as they did.
func Cursor(on bool) Option { return func(s *settings) { s.noCursor = !on } }
