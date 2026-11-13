package ggui

import "testing"

// A region asking for the default cursor shows the arrow over whatever
// asked for another shape under it; one that asks for nothing lets that
// shape through.
func TestDefaultCursorIsARequest(t *testing.T) {
	t.Parallel()
	b := &reachButton{}
	root := FromFuncs(func(c Constraints, _ Env) Size { return c.Constrain(Sz(100, 40)) }, func(dst *Canvas, r Rect) {
		dst.HitCursor(r, CursorShapeText)
		dst.HitCursor(Rct(Pt(0, 0), Sz(20, 40)), CursorShapeDefault)
		dst.HitPointer(Rct(Pt(40, 0), Sz(20, 40)), b)
		b.Hit(dst, Rct(Pt(80, 0), Sz(20, 40)), b, CursorShapeDefault)
	})
	p := NewProbe(root, Sz(100, 40))
	defer p.Close()
	for _, c := range []struct {
		x    float64
		want CursorShape
		what string
	}{
		{10, CursorShapeDefault, "a HitCursor for the default"},
		{30, CursorShapeText, "the region underneath"},
		{50, CursorShapeText, "a pointer region asking for no cursor"},
		{90, CursorShapeDefault, "a control hit with the default"},
	} {
		p.Move(Pt(c.x, 20))
		if got := p.Cursor(); got != c.want {
			t.Errorf("cursor = %v over %s, want %v", got, c.what, c.want)
		}
	}
}
