package main

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// The turned button takes a click where it is drawn.
func TestEffectsTurnedButtonStillClicks(t *testing.T) {
	t.Parallel()
	p := ggui.NewProbe(effectsDemo(), ggui.Sz(600, 400))
	defer p.Close()
	p.Frame()
	before, _ := p.Find("Click me")
	p.Tap("Turn")
	p.Frame() // the tween starts its clock
	p.Advance(time.Second)
	after, _ := p.Find("Click me")
	if after.Rect.Size.H <= before.Rect.Size.H {
		t.Fatalf("button bounds %v after turning, %v before; want them taller", after.Rect, before.Rect)
	}
	p.Tap("Click me")
	if _, ok := p.Semantics().Find("", "1 clicks"); !ok {
		t.Fatal("a click on the turned button was lost")
	}
}
