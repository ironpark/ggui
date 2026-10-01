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
	if a.mods().Meta {
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
