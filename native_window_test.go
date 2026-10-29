package ggui

import (
	"testing"

	"github.com/ironpark/ggwd"
)

func TestAppUsesNativeModifierSnapshot(t *testing.T) {
	t.Parallel()
	a := &Window{}
	a.handle(ggwd.KeyEvent{Key: ggwd.KeyA, Pressed: true, Modifiers: ggwd.KeyModifiers{Meta: true}})
	if !a.takeInput().mods.Meta {
		t.Fatal("native modifier snapshot was lost without a separate modifier key event")
	}
}

func TestAppKeepsModifierOnQueuedKeyPress(t *testing.T) {
	t.Parallel()
	a := &Window{}
	for _, e := range []ggwd.KeyEvent{
		{Key: ggwd.KeyMetaLeft, Pressed: true},
		{Key: ggwd.KeyO, Pressed: true},
		{Key: ggwd.KeyO},
		{Key: ggwd.KeyMetaLeft},
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
	for _, e := range []ggwd.Event{
		ggwd.MouseButtonEvent{Button: ggwd.MouseButtonLeft, Pressed: true, Modifiers: ggwd.KeyModifiers{Shift: true}},
		ggwd.ScrollEvent{Y: -1, Modifiers: ggwd.KeyModifiers{Shift: true}},
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
	for _, later := range []ggwd.Event{
		ggwd.KeyEvent{Key: ggwd.KeyA, Pressed: true},
		ggwd.MouseButtonEvent{Button: ggwd.MouseButtonLeft, Pressed: true},
		ggwd.ScrollEvent{Y: -1},
	} {
		a := &Window{}
		if err := a.handle(ggwd.KeyEvent{Key: ggwd.KeyShiftLeft, Pressed: true, Modifiers: ggwd.KeyModifiers{Shift: true}}); err != nil {
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
	a.handle(ggwd.KeyEvent{Key: ggwd.KeyShiftLeft, Pressed: true, Modifiers: ggwd.KeyModifiers{Shift: true}})
	a.handle(ggwd.KeyEvent{Key: ggwd.KeyA, Pressed: true, Modifiers: ggwd.KeyModifiers{Shift: true}})
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
	for _, e := range []ggwd.ScrollEvent{
		{X: -1, Phase: ggwd.ScrollPhaseChanged},
		{Phase: ggwd.ScrollPhaseEnded},
		{X: -0.5, Phase: ggwd.ScrollPhaseMomentum},
	} {
		if err := a.handle(e); err != nil {
			t.Fatal(err)
		}
	}
	if f := a.takeInput(); f.phase != ScrollPhaseEnded || f.wheel.X != -1.5 {
		t.Fatalf("phase %v wheel %v", f.phase, f.wheel)
	}
	a.handle(ggwd.ScrollEvent{Phase: ggwd.ScrollPhaseEnded})
	if f := a.takeInput(); f.phase != ScrollPhaseEnded || f.wheel != (Point{}) {
		t.Fatalf("a lift alone: phase %v wheel %v", f.phase, f.wheel)
	}
}
