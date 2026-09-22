package ggui

import (
	"testing"

	"github.com/ironpark/ggfx"
)

func TestAppUsesNativeModifierSnapshot(t *testing.T) {
	a := &App{}
	a.HandleEvent(ggfx.KeyEvent{Key: ggfx.KeyA, Pressed: true, Modifiers: ggfx.KeyModifiers{Meta: true}})
	if !a.takeInput().mods.Meta {
		t.Fatal("native modifier snapshot was lost without a separate modifier key event")
	}
}

func TestAppKeepsModifierOnQueuedKeyPress(t *testing.T) {
	a := &App{}
	for _, e := range []ggfx.KeyEvent{
		{Key: ggfx.KeyMetaLeft, Pressed: true},
		{Key: ggfx.KeyO, Pressed: true},
		{Key: ggfx.KeyO},
		{Key: ggfx.KeyMetaLeft},
	} {
		if err := a.HandleEvent(e); err != nil {
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
