package main

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// TestCustom drags the dial by its semantics rect, steps it from the
// keyboard, opens the disclosure and checks the list only mounts what it
// shows.
func TestCustom(t *testing.T) {
	m := newModel()
	p := ggui.ProbeBuilder(m.build, ggui.Sz(560, 640))
	defer p.Close()

	dial, ok := p.FindRole(ggui.RoleSlider, "Level")
	if !ok {
		t.Fatal("dial is not registered as a slider")
	}
	c := dial.Center()
	p.Press(ggui.Pt(c.X, c.Y-40)) // straight up: 12 o'clock is half way
	p.Release(ggui.Pt(c.X, c.Y-40))
	if got := ggui.Untrack(m.Level.Get); got < 0.45 || got > 0.55 {
		t.Fatalf("level after pressing at 12 o'clock = %.2f, want about 0.5", got)
	}
	p.Press(c)
	p.Move(ggui.Pt(c.X+40, c.Y)) // drag to 3 o'clock
	p.Release(ggui.Pt(c.X+40, c.Y))
	if got := ggui.Untrack(m.Level.Get); got < 0.78 || got > 0.88 {
		t.Fatalf("level after dragging to 3 o'clock = %.2f, want about 0.83", got)
	}
	// The dial holds focus from the click; the arrows step it.
	p.Type(ggui.Mods{}, ggui.KeyArrowRight, ggui.KeyArrowRight, ggui.KeyArrowRight, ggui.KeyArrowRight)
	if got := ggui.Untrack(m.Level.Get); got != 1 {
		t.Fatalf("level after four right arrows = %.2f, want clamped at 1", got)
	}
	p.Advance(time.Second) // let the spring settle; Paint reads its value

	if m.Detail.open {
		t.Fatal("disclosure starts closed")
	}
	if _, ok := p.Semantics().Find(ggui.RoleText, "How this works"); !ok {
		t.Fatal("disclosure header missing")
	}
	head, _ := p.Semantics().Find(ggui.RoleText, "How this works")
	p.Click(head.Rect.Center())
	if !m.Detail.open {
		t.Fatal("tapping the header did not open the disclosure")
	}
	if _, ok := p.Semantics().Find(ggui.RoleText, "This panel calls Invalidate instead of writing a signal when it opens."); !ok {
		t.Fatal("open disclosure did not lay out its body")
	}

	rows := len(p.FindAll(ggui.RoleCheckbox))
	if rows == 0 || rows >= rowCount/10 {
		t.Fatalf("%d rows mounted, want only the ones in view out of %d", rows, rowCount)
	}
	p.Tap("End")
	if _, ok := p.Find("Row 5000"); !ok {
		t.Fatal("jumping to the end did not reveal the last row")
	}
	if _, ok := p.Find("Row 1"); ok {
		t.Fatal("the first row is still mounted after jumping to the end")
	}
}
