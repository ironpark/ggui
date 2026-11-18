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
}

func resolve(opts []Option) *settings {
	s := &settings{colors: 256}
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
