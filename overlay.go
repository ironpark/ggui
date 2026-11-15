package ggui

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
func (w *Window) SetOverlay(o Overlay) { w.overlay = o }
