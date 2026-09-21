package ggui

import "testing"

// counting is a leaf that counts its layouts.
type counting struct{ layouts int }

func (c *counting) Layout(Constraints, Env) Size { c.layouts++; return Sz(10, 10) }
func (c *counting) Paint(*Canvas, Rect)          {}

func TestCachedSkipsLayoutUntilSomethingInsideChanges(t *testing.T) {
	leaf := &counting{}
	dep := State(0)
	var inner Widget
	dispose := observe(func() {
		inner = Reactive(func() Widget { dep.Get(); return Column(leaf) })
	})
	defer dispose()
	c := Cached(inner)
	tree := Column(c, Box().Size(10, 10))
	env := rootEnv()
	tree.Layout(Tight(Sz(100, 100)), env)
	tree.Layout(Tight(Sz(100, 100)), env)
	if leaf.layouts != 1 {
		t.Fatalf("leaf laid out %d times under unchanged constraints, want 1", leaf.layouts)
	}
	tree.Layout(Tight(Sz(200, 100)), env)
	if leaf.layouts != 2 {
		t.Fatalf("leaf laid out %d times after a resize, want 2", leaf.layouts)
	}
	dep.Set(1)
	effects.flush()
	tree.Layout(Tight(Sz(200, 100)), env)
	if leaf.layouts != 3 {
		t.Fatalf("leaf laid out %d times after a rebuild inside, want 3", leaf.layouts)
	}
	tree.Layout(Tight(Sz(200, 100)), env.WithText(TextStyle{Size: 20}))
	if leaf.layouts != 4 {
		t.Fatalf("leaf laid out %d times after the inherited style changed, want 4", leaf.layouts)
	}
}

func TestCachedNestsAndFollowsScrollAndFor(t *testing.T) {
	leaf := &counting{}
	items := State([]todo{{1, "a"}})
	var list Widget
	dispose := observe(func() {
		list = EachKeyed(items, func(t todo) int { return t.ID }, func(EachItem[todo]) Widget { return leaf })
	})
	defer dispose()
	inner := Cached(list)
	offset := State(0.0)
	s := Scroll(Column(inner, Box().Size(10, 500))).BindOffset(offset)
	outer := Cached(s)
	env := rootEnv()
	outer.Layout(Tight(Sz(100, 100)), env)
	outer.Layout(Tight(Sz(100, 100)), env)
	if leaf.layouts != 1 {
		t.Fatalf("leaf laid out %d times, want 1", leaf.layouts)
	}
	items.Set([]todo{{1, "a"}, {2, "b"}})
	effects.flush()
	outer.Layout(Tight(Sz(100, 100)), env)
	if leaf.layouts != 3 {
		t.Fatalf("leaf laid out %d times after the list grew through two caches, want 3", leaf.layouts)
	}
	// A scroll through the bound signal changes the viewport the inner
	// cache saw, so it lays out again when the outer one is asked.
	offset.Set(40)
	s.Paint(nil, Rct(Pt(0, 0), Sz(100, 100)))
	outer.Layout(Tight(Sz(100, 100)), env)
	if leaf.layouts != 5 {
		t.Fatalf("leaf laid out %d times after scrolling, want 5", leaf.layouts)
	}
	if !outer.dirty && outer.valid {
		outer.Layout(Tight(Sz(100, 100)), env)
		if leaf.layouts != 5 {
			t.Fatalf("leaf laid out %d times on a still frame, want 5", leaf.layouts)
		}
	}
}

// countingLeaf records how often it was measured.
type countingLeaf struct{ n int }

func (c *countingLeaf) Layout(cs Constraints, env Env) Size {
	c.n++
	return cs.Constrain(Sz(10, 10))
}
func (c *countingLeaf) Paint(dst *Canvas, r Rect) {}

// A rebuild boundary is a layout boundary: writing a signal one subtree reads
// must not re-measure its siblings, even though the runtime lays out from the
// root whenever anything was written.
func TestRebuildBoundaryConfinesLayout(t *testing.T) {
	a, b := State(0), State(0)
	leafA, leafB := &countingLeaf{}, &countingLeaf{}
	p := ProbeBuilder(func() Widget {
		return Column(
			Reactive(func() Widget { a.Get(); return leafA }),
			Reactive(func() Widget { b.Get(); return leafB }),
		)
	}, Sz(200, 200))
	defer p.Close()
	p.Frame()

	wasA, wasB := leafA.n, leafB.n
	a.Set(1)
	p.Frame()
	if leafA.n == wasA {
		t.Fatalf("the subtree that changed was not measured again")
	}
	if leafB.n != wasB {
		t.Fatalf("the untouched subtree was measured %d more times", leafB.n-wasB)
	}

	wasA, wasB = leafA.n, leafB.n
	p.Frame()
	if leafA.n != wasA || leafB.n != wasB {
		t.Fatalf("a still frame measured again: A+%d B+%d", leafA.n-wasA, leafB.n-wasB)
	}
}
