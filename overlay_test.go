package ggui

import "testing"

// recorder is an Overlay that takes input over its rect and asks for a
// crosshair there.
type recorder struct {
	rect   Rect
	inputs int
	paints int
}

func (r *recorder) Paint(*Canvas) { r.paints++ }
func (r *recorder) Input(in OverlayInput) bool {
	r.inputs++
	return r.rect.Contains(in.Pos)
}
func (r *recorder) Cursor(p Point) (CursorShape, bool) {
	return CursorShapeCrosshair, r.rect.Contains(p)
}

func TestOverlayTakesInputBeforeWidgetsExceptDuringADrag(t *testing.T) {
	a := &App{}
	taps := 0
	w := Tap(Box().Size(400, 400), func() { taps++ })
	paintFrame(&a.input, w, Sz(800, 600))
	o := &recorder{rect: Rct(Pt(0, 0), Sz(100, 100))}
	a.SetOverlay(o)

	a.dispatchInput(frameInput{pos: Pt(50, 50), down: []MouseButton{MouseButtonLeft}})
	a.dispatchInput(frameInput{pos: Pt(50, 50), up: []MouseButton{MouseButtonLeft}})
	if taps != 0 || o.inputs != 2 {
		t.Fatalf("taps = %d, overlay saw %d frames; the overlay should have taken the click", taps, o.inputs)
	}
	a.dispatchInput(frameInput{pos: Pt(300, 300), down: []MouseButton{MouseButtonLeft}})
	a.dispatchInput(frameInput{pos: Pt(50, 50), up: []MouseButton{MouseButtonLeft}})
	if o.inputs != 3 {
		t.Fatalf("overlay saw %d frames; a widget's press should keep the release from it", o.inputs)
	}
	if a.input.pressed != nil || taps != 1 {
		t.Fatalf("the widget's release did not reach it: taps = %d", taps)
	}
	a.SetOverlay(nil)
	a.dispatchInput(frameInput{pos: Pt(50, 50), down: []MouseButton{MouseButtonLeft}})
	a.dispatchInput(frameInput{pos: Pt(50, 50), up: []MouseButton{MouseButtonLeft}})
	if taps != 2 || o.inputs != 3 {
		t.Fatalf("taps = %d, overlay saw %d frames after removal", taps, o.inputs)
	}
}

func TestCanvasTextForOverlays(t *testing.T) {
	c := &Canvas{}
	c.scale = 2
	if w := c.TextWidth("hello", nil, 12); w <= 0 || w != c.TextWidth("hello", DefaultFont(), 12) {
		t.Fatalf("TextWidth = %v", w)
	}
	if c.TextWidth("hello hello", nil, 12) <= c.TextWidth("hello", nil, 12) {
		t.Fatal("a longer line should measure wider")
	}
	if got := c.Physical(Rct(Pt(1, 2), Sz(3, 4))); got.Min.X != 2 || got.Min.Y != 4 || got.Dx() != 6 {
		t.Fatalf("Physical = %v, want the rect at scale 2", got)
	}
	c.DrawText("no image, no panic", nil, 12, Pt(0, 0), nil)
}
