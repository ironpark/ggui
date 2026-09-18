package ggui_test

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"testing"
)

func TestKeyedIdentityAcrossParentsAndRemount(t *testing.T) {
	left, right := ggui.State(false), ggui.State(false)
	rebuild, parent := ggui.State(0), ggui.State(0)
	show := ggui.State(true)
	var controls [2]*ui.CheckboxWidget
	panel := func(index int, value *ggui.Signal[bool], label string) ggui.Widget {
		return ggui.Component(func() ggui.Builder {
			return func() ggui.Widget {
				parent.Get()
				if index == 0 && !show.Get() {
					return ggui.Box()
				}
				return ggui.Keyed(1, func() ggui.Builder {
					return func() ggui.Widget {
						rebuild.Get()
						controls[index] = ui.Checkbox(value, label)
						return controls[index]
					}
				})
			}
		})
	}
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Column(panel(0, left, "Left"), panel(1, right, "Right"))
	}, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	first := controls[0].HitID()
	if first == nil || first == controls[1].HitID() {
		t.Fatal("separate mounts must have distinct automatic IDs")
	}
	p.Tap("Left")
	p.Frame()
	if !controls[0].Focused || controls[1].Focused {
		t.Fatal("static paint transferred focus between mounts")
	}
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if left.Peek() || right.Peek() {
		t.Fatal("Space must toggle only the focused left checkbox")
	}
	parent.Set(1)
	rebuild.Set(1)
	p.Frame()
	p.Frame()
	if controls[0].HitID() != first {
		t.Fatal("rebuilding a mounted component changed its ID")
	}
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if !left.Peek() || right.Peek() {
		t.Fatal("Space after rebuild must toggle only the left checkbox")
	}
	show.Set(false)
	p.Frame()
	show.Set(true)
	p.Frame()
	if controls[0].HitID() == first {
		t.Fatal("a new mount reused the disposed mount's identity")
	}
}
