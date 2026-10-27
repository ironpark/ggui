package ggui

import (
	"testing"

	"github.com/ironpark/ggfx"
)

func TestAppUsesNativeModifierSnapshot(t *testing.T) {
	t.Parallel()
	a := &Window{}
	a.handle(ggfx.KeyEvent{Key: ggfx.KeyA, Pressed: true, Modifiers: ggfx.KeyModifiers{Meta: true}})
	if !a.takeInput().mods.Meta {
		t.Fatal("native modifier snapshot was lost without a separate modifier key event")
	}
}

func TestAppKeepsModifierOnQueuedKeyPress(t *testing.T) {
	t.Parallel()
	a := &Window{}
	for _, e := range []ggfx.KeyEvent{
		{Key: ggfx.KeyMetaLeft, Pressed: true},
		{Key: ggfx.KeyO, Pressed: true},
		{Key: ggfx.KeyO},
		{Key: ggfx.KeyMetaLeft},
	} {
		if err := a.handle(e); err != nil {
			t.Fatal(err)
		}
	}
	f := a.takeInput()
	if !f.mods.Meta {
		t.Fatal("Cmd+O lost its modifier when Cmd was released before the frame")
	}
	if a.keyboard.Mods().Meta {
		t.Fatal("released modifier remains held")
	}
}

func TestPointerEventsCarryTheirModifiers(t *testing.T) {
	t.Parallel()
	for _, e := range []ggfx.Event{
		ggfx.MouseButtonEvent{Button: ggfx.MouseButtonLeft, Pressed: true, Modifiers: ggfx.KeyModifiers{Shift: true}},
		ggfx.ScrollEvent{Y: -1, Modifiers: ggfx.KeyModifiers{Shift: true}},
	} {
		a := &Window{}
		if err := a.handle(e); err != nil {
			t.Fatal(err)
		}
		if !a.takeInput().mods.Shift {
			t.Errorf("%T: a modifier held as it arrived, with no KeyEvent for it, was lost", e)
		}
	}
}

// A modifier whose release the window system never reported, taken by a
// system shortcut or an input source switch, is let go by the next event
// that says it is not held.
func TestLostModifierReleaseIsForgotten(t *testing.T) {
	t.Parallel()
	for _, later := range []ggfx.Event{
		ggfx.KeyEvent{Key: ggfx.KeyA, Pressed: true},
		ggfx.MouseButtonEvent{Button: ggfx.MouseButtonLeft, Pressed: true},
		ggfx.ScrollEvent{Y: -1},
	} {
		a := &Window{}
		if err := a.handle(ggfx.KeyEvent{Key: ggfx.KeyShiftLeft, Pressed: true, Modifiers: ggfx.KeyModifiers{Shift: true}}); err != nil {
			t.Fatal(err)
		}
		a.takeInput()
		if err := a.handle(later); err != nil {
			t.Fatal(err)
		}
		if a.takeInput().mods.Shift || a.keyboard.Mods().Shift {
			t.Errorf("after %T Shift is still held", later)
		}
	}
	// One the event says is held stays held.
	a := &Window{}
	a.handle(ggfx.KeyEvent{Key: ggfx.KeyShiftLeft, Pressed: true, Modifiers: ggfx.KeyModifiers{Shift: true}})
	a.handle(ggfx.KeyEvent{Key: ggfx.KeyA, Pressed: true, Modifiers: ggfx.KeyModifiers{Shift: true}})
	a.takeInput()
	if !a.keyboard.Mods().Shift {
		t.Error("a held Shift was let go")
	}
}

// The fingers lifting reaches the widgets though it moves nothing, and is
// not lost to the momentum that follows within the frame.
func TestScrollPhaseReachesTheFrame(t *testing.T) {
	t.Parallel()
	a := &Window{}
	for _, e := range []ggfx.ScrollEvent{
		{X: -1, Phase: ggfx.ScrollPhaseChanged},
		{Phase: ggfx.ScrollPhaseEnded},
		{X: -0.5, Phase: ggfx.ScrollPhaseMomentum},
	} {
		if err := a.handle(e); err != nil {
			t.Fatal(err)
		}
	}
	if f := a.takeInput(); f.phase != ScrollPhaseEnded || f.wheel.X != -1.5 {
		t.Fatalf("phase %v wheel %v", f.phase, f.wheel)
	}
	a.handle(ggfx.ScrollEvent{Phase: ggfx.ScrollPhaseEnded})
	if f := a.takeInput(); f.phase != ScrollPhaseEnded || f.wheel != (Point{}) {
		t.Fatalf("a lift alone: phase %v wheel %v", f.phase, f.wheel)
	}
}
