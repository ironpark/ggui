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
	root := Column(View(s, func(v string) *TextWidget { builds++; return Text(v) }), When(on, then, other))
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
	w := If(a, ta).ElseIf(b, tb).Else(tc)
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

	bare := If(a, ta)
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

// Branches are constructed once and kept, so the one shown keeps its state
// across being hidden; the choice rebuilds the If alone, not its parent.
func TestIfKeepsBranchesAndRebuildsAlone(t *testing.T) {
	on := State(true)
	then := TextOf(State("kept"))
	parentBuilds := 0
	var w *IfWidget
	p := ProbeBuilder(func() Widget {
		parentBuilds++
		w = If(on, then).Else(Text("other"))
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
		w := If(on, Text("a")).Else(Text("b"))
		if gen.Get() == 0 {
			first = w
		}
		return w
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
