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
	w := root.children[1].(*ComponentWidget)
	if w.child != then {
		t.Fatal("When did not show then")
	}
	on.Set(false)
	p.Frame()
	if w.child != other {
		t.Fatal("When did not switch to other")
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
