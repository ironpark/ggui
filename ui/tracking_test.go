package ui

import (
	"math"
	"testing"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// shadcn sets a row's shortcut hint in tracking-widest (0.1em of its 12px)
// and an Empty's title in tracking-tight (-0.025em of its 14px).
func TestShortcutHintsAndTheEmptyTitleAreTracked(t *testing.T) {
	th := uitheme.Default()
	env := th.Apply(ggui.Env{})
	room := ggui.Loose(ggui.Sz(400, 300))

	item := MenuItem("Save", nil).Shortcut("ABCD")
	item.Layout(room, env)
	plain := Caption("ABCD").NoWrap().Layout(room, env).W
	if got, want := item.shortcutSize.W, plain+4*0.1*12; math.Abs(got-want) > 0.1 {
		t.Errorf("shortcut is %v wide, want the plain %v and 1.2px after each letter, %v", got, plain, want)
	}

	empty := Empty("ABCD", "")
	empty.Layout(room, env)
	plain = ggui.Text("ABCD").Style(th.Text.Merge(th.Label)).Layout(room, env).W
	if got, want := empty.title.Layout(room, env).W, plain-4*0.025*14; math.Abs(got-want) > 0.1 {
		t.Errorf("empty title is %v wide, want the plain %v less 0.35px after each letter, %v", got, plain, want)
	}
}
