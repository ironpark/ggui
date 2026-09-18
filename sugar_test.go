package ggui

import "testing"

func TestTextfFollowsSignals(t *testing.T) {
	n := State(1)
	w := Textf("n=%d %s", n, "x")
	p := NewProbe(w, Sz(200, 30))
	p.Frame()
	if w.value != "n=1 x" {
		t.Fatalf("value = %q", w.value)
	}
	n.Set(2)
	p.Frame()
	if w.value != "n=2 x" {
		t.Fatalf("after set value = %q", w.value)
	}
}

func TestViewAndWhen(t *testing.T) {
	s := State("a")
	on := State(true)
	then, other := Text("then"), Text("other")
	builds := 0
	root := Column(View(s, func(v string) *TextWidget { builds++; return Text(v) }), If(on, func() Widget {
		return then
	}).Else(func() Widget {
		return other
	}))
	p := NewProbe(root, Sz(200, 100))
	p.Frame()
	s.Set("b")
	p.Frame()
	if builds != 2 {
		t.Fatalf("builds = %d", builds)
	}
	w := root.children[1].(*IfWidget)
	if w.current() != then {
		t.Fatal("When did not show then")
	}
	on.Set(false)
	p.Frame()
	if w.current() != other {
		t.Fatal("When did not switch to other")
	}
}

// If reads every condition in order, so a later branch's condition is
// subscribed even though it was chained on after construction; without an
// Else, nothing is shown when no condition holds.
func TestIfPicksTheFirstTrueBranch(t *testing.T) {
	a, b := State(false), State(true)
	ta, tb, tc := Text("A"), Text("B"), Text("C")
	w := If(a, func() Widget {
		return ta
	}).ElseIf(b, func() Widget {
		return tb
	}).Else(func() Widget {
		return tc
	})
	p := NewProbe(w, Sz(200, 100))
	p.Frame()
	if w.current() != tb {
		t.Fatal("B should show while a is false and b is true")
	}
	b.Set(false)
	p.Frame()
	if w.current() != tc {
		t.Fatal("changing only the ElseIf condition did not switch to Else")
	}
	a.Set(true)
	b.Set(true)
	p.Frame()
	if w.current() != ta {
		t.Fatal("A should win once a is true")
	}

	bare := If(a, func() Widget {
		return ta
	})
	a.Set(false)
	q := NewProbe(bare, Sz(200, 100))
	q.Frame()
	if bare.current() != nil {
		t.Fatalf("If without Else showed %v, want nothing", bare.current())
	}
	if _, ok := q.Semantics().Find(RoleText, ""); ok {
		t.Fatal("If without Else painted a text node")
	}
}

// An If showing nothing is left out of its flow: no gap beside it in a
// Column, Row or Wrap, and justification counts the children that are there.
func TestIfShowingNothingTakesNoGap(t *testing.T) {
	off := State(false)
	col := Column(Box().Size(10, 10), If(off, func() Widget {
		return Box().Size(10, 10)
	}), Box().Size(10, 10)).Gap(4)
	p := NewProbe(Align(col).At(0, 0), Sz(100, 100))
	p.Frame()
	if h := col.sizes[0].H + col.sizes[1].H + col.sizes[2].H; h != 20 || col.offsets[2].Y != 14 {
		t.Fatalf("third child at y=%v with %v of content, want 14 after one box and one gap", col.offsets[2].Y, h)
	}
	off.Set(true)
	p.Frame()
	if got := col.offsets[2].Y; got != 28 {
		t.Fatalf("third child at y=%v once shown, want 28 after two boxes and two gaps", got)
	}

	off.Set(false)
	p.Frame()
	row := Row(Box().Size(10, 10), If(off, func() Widget {
		return Box().Size(10, 10)
	}), Box().Size(10, 10)).Justify(SpaceBetween)
	row.Layout(Tight(Sz(100, 10)), rootEnv())
	if got := row.offsets[2].X; got != 90 {
		t.Fatalf("space-between placed the last box at x=%v, want 90 with the absent child ignored", got)
	}

	wrap := Wrap(Box().Size(10, 10), If(off, func() Widget {
		return Box().Size(10, 10)
	}), Box().Size(10, 10)).Gap(4)
	if w := wrap.Layout(Loose(Sz(100, 100)), rootEnv()).W; w != 24 {
		t.Fatalf("wrap width = %v, want two boxes and one gap", w)
	}
}

// Branches are constructed once and kept, so the one shown keeps its state
// across being hidden; the choice rebuilds the If alone, not its parent.
func TestIfKeepsBranchesAndRebuildsAlone(t *testing.T) {
	on := State(true)
	then := TextOf(State("kept"))
	parentBuilds := 0
	var w *IfWidget
	p := ProbeBuilder(func() Widget {
		parentBuilds++
		w = If(on, func() Widget {
			return then
		}).Else(func() Widget {
			return Text("other")
		})
		return w
	}, Sz(200, 100))
	p.Frame()
	on.Set(false)
	p.Frame()
	on.Set(true)
	p.Frame()
	if w.current() != then {
		t.Fatal("the then branch was not the same widget after being hidden")
	}
	if parentBuilds != 1 {
		t.Fatalf("parent built %d times, want 1: If must switch in its own effect", parentBuilds)
	}
}

// An If the parent rebuilt away stops reacting: its effect is owned by the
// builder run that made it.
func TestIfIsDisposedWithItsParent(t *testing.T) {
	on := State(true)
	gen := State(0)
	var first *IfWidget
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			w := If(on, func() Widget {
				return Text("a")
			}).Else(func() Widget {
				return Text("b")
			})
			if gen.Get() == 0 {
				first = w
			}
			return w
		})
	}, Sz(200, 100))
	p.Frame()
	gen.Set(1)
	p.Frame()
	before := first.current()
	on.Set(false)
	p.Frame()
	if first.current() != before {
		t.Fatal("a replaced If kept switching branches after its parent rebuilt")
	}
}

func TestSpaceAndRoles(t *testing.T) {
	th := DefaultTheme()
	col := Column(Box().Size(10, 10), Box().Size(10, 10)).Space(2)
	if h := col.Layout(Loose(Sz(100, 100)), rootEnv()).H; h != 20+2*th.Space {
		t.Fatalf("height = %v", h)
	}
	title := Title("x")
	title.Layout(Loose(Sz(100, 100)), rootEnv())
	if title.resolved.Size != th.Title.Size {
		t.Fatalf("title size = %v", title.resolved.Size)
	}
}
