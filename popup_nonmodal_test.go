package ggui

import "testing"

func TestPopupLogicalAlignment(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		align PopupAlignment
		dir   TextDirection
		x     float64
	}{
		{PopupStart, DirLTR, 0}, {PopupCenter, DirLTR, -20}, {PopupEnd, DirLTR, -40},
		{PopupStart, DirRTL, -40}, {PopupCenter, DirRTL, -20}, {PopupEnd, DirRTL, 0},
	} {
		p := Popup(Box().Size(60, 20), Box().Size(100, 50)).Align(tt.align)
		pr := NewProbe(Provide(DirectionKey, tt.dir, Column(Padding(p, 0, 100))), Sz(400, 250))
		p.Show()
		pr.Frame()
		want := p.anchorRect.Origin.X + tt.x
		if p.Rect().Origin.X != want {
			t.Errorf("align=%v dir=%v rect=%v want x=%v", tt.align, tt.dir, p.Rect(), want)
		}
		pr.Close()
	}
}

func TestPopupRejectedControlledCloseDoesNotNotify(t *testing.T) {
	t.Parallel()
	requests, closes := 0, 0
	p := Popup(Box().Size(50, 20), Box().Size(100, 100)).Modal(false).BindOpen(Controlled(true, func(bool) { requests++ })).OnClose(func() { closes++ })
	pr := NewProbe(p, Sz(300, 250))
	defer pr.Close()
	pr.Frame()
	p.Hide()
	if !p.IsOpen() || requests != 1 || closes != 0 {
		t.Fatalf("rejected close: open=%v requests=%d closes=%d", p.IsOpen(), requests, closes)
	}
}
