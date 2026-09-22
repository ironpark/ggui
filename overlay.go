package ggui

import (
	"image"
	"image/color"

	"github.com/ironpark/ggfx/text/v2"
)

// An overlay is drawn over the whole window after the widget tree, and is
// offered the frame's input before any widget: a development panel, a
// debugging readout, a screen-recording banner. The widget inspector is
// one. An overlay cannot be a widget, since it paints while the frame's
// trace is being read back, so it draws imperatively with the Canvas
// methods below; what a widget gets for free, text in particular, is here
// for it too.

// Overlay is something drawn over every frame that may take its input.
type Overlay interface {
	// Paint draws over the finished frame. dst covers the whole window.
	Paint(dst *Canvas)
	// Input is offered the frame's input before the widgets, and reports
	// whether it took it, in which case no widget sees it. It is not
	// offered while a widget holds a press, so an app drag finishes where
	// it started.
	Input(in OverlayInput) bool
	// Cursor is the mouse cursor the overlay wants at p, if any.
	Cursor(p Point) (CursorShape, bool)
}

// OverlayInput is one frame's input as an Overlay sees it.
type OverlayInput struct {
	Pos      Point
	Down, Up []MouseButton
	Wheel    Point
	Keys     []KeyboardKey // just pressed, plus repeats of held keys
	Text     string
	Mods     Mods
}

// overlayInput is the frame's input in the form an Overlay takes.
func overlayInput(f frameInput) OverlayInput {
	return OverlayInput{Pos: f.pos, Down: f.down, Up: f.up, Wheel: f.wheel, Keys: f.keys, Text: f.text, Mods: f.mods}
}

// SetOverlay installs o over the window, or removes the overlay when o is
// nil. There is one at a time: turning the inspector on installs it here
// and turning it off removes it.
func (a *App) SetOverlay(o Overlay) { a.overlay = o }

// dispatchOver offers f to the overlay before the widgets, unless a widget
// holds a press, so that a drag that began in the app finishes there.
func (in *inputState) dispatchOver(o Overlay, f frameInput) {
	if o == nil || in.pressed != nil || !o.Input(overlayInput(f)) {
		in.dispatch(f)
	}
}

// cursorOver is the cursor at p: the overlay's when it wants one and no
// widget holds a press, else what the widgets asked for.
func (in *inputState) cursorOver(o Overlay, p Point) CursorShape {
	if o != nil && in.pressed == nil {
		if shape, ok := o.Cursor(p); ok {
			return shape
		}
	}
	return in.cursor
}

// DrawText draws s as one unwrapped line with its top-left corner at the
// logical point at, in font at size logical pixels; a nil font is the
// default. It is for an Overlay: a widget draws text through Text.
func (c *Canvas) DrawText(s string, font *Font, size float64, at Point, col color.Color) {
	if c == nil || c.Image == nil {
		return
	}
	if font == nil {
		font = fallbackFont()
	}
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(col)
	op.GeoM.Translate(c.px(at.X), c.px(at.Y))
	drawText(c.Image, s, font.face(c.px(size)), op)
}

// TextWidth measures s as DrawText would draw it, in logical pixels.
func (c *Canvas) TextWidth(s string, font *Font, size float64) float64 {
	if font == nil {
		font = fallbackFont()
	}
	return c.dp(lineWidth(s, font.face(c.px(size))))
}

// Physical is r in the pixels of Image, for an Overlay that keeps an image
// of its own the size of what it draws.
func (c *Canvas) Physical(r Rect) image.Rectangle { return c.physical(r) }

// DefaultFont is the font Text uses when none is set.
func DefaultFont() *Font { return fallbackFont() }

// CurrentClipboard is the clipboard SetClipboard installed, or the
// platform's.
func CurrentClipboard() Clipboard { return currentClipboard() }
