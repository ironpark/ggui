package ggui

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui/internal/reactive"
)

// The tests here are of Cached, and the caches If, Key and View keep, with
// widgets that keep layout state of their own around and inside them: a
// scroll, a contained pane and a keyed list.

// measured is a leaf that counts its layouts.
type measured struct{ layouts int }

func (m *measured) Layout(Constraints, Env) Size { m.layouts++; return Sz(10, 10) }
func (m *measured) Paint(*Canvas, Rect)          {}

func TestCachedNestsAndFollowsScrollAndFor(t *testing.T) {
	t.Parallel()
	leaf := &measured{}
	items := State([]todo{{1, "a"}})
	var list Widget
	dispose := reactive.Observe(func() {
		list = EachKeyed(items, func(t todo) int { return t.ID }, func(EachItem[todo]) Widget { return leaf })
	})
	defer dispose()
	inner := Cached(list)
	offset := State(0.0)
	s := Scroll(Column(inner, Box().Size(10, 500))).BindOffset(offset)
	outer := Cached(s)
	env := Untrack(UseEnv)
	outer.Layout(Tight(Sz(100, 100)), env)
	outer.Layout(Tight(Sz(100, 100)), env)
	if leaf.layouts != 1 {
		t.Fatalf("leaf laid out %d times, want 1", leaf.layouts)
	}
	items.Set([]todo{{1, "a"}, {2, "b"}})
	reactive.Flush()
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
	outer.Layout(Tight(Sz(100, 100)), env)
	if leaf.layouts != 5 {
		t.Fatalf("leaf laid out %d times on a still frame, want 5", leaf.layouts)
	}
}

// A Scroll and a Contain laid out again, as their parent is, hand down
// the Env they did last time, carrying their boundaries, so a cache inside
// either, with nothing changed in it, keeps its measurement.
func TestACacheInsideABoundaryLaidOutAgainHolds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		wrap func(Widget) Widget
	}{
		{"scroll", func(w Widget) Widget { return Scroll(w) }},
		{"contain", func(w Widget) Widget { return Contain(Box(w).Size(100, 50)) }},
		{"contain in a scroll", func(w Widget) Widget { return Scroll(Contain(Box(w).Size(100, 50))) }},
		{"scroll in a contain", func(w Widget) Widget { return Contain(Box(Scroll(w)).Size(100, 50)) }},
	} {
		leaf := &measured{}
		dep := State(0)
		boundary, laid := tc.wrap(Cached(Column(leaf))), 0
		// A sibling rebuilt at the same size: the boundary is laid out
		// again under the constraints it had.
		p := NewProbe(Column(
			Reactive(func() Widget { return Box().Size(10, 10).Fill(color.Gray{uint8(dep.Get())}) }),
			FromFuncs(func(c Constraints, env Env) Size { laid++; return boundary.Layout(c, env) }, boundary.Paint),
		), Sz(100, 100))
		p.Frame()
		p.Frame()
		was, wasLaid := leaf.layouts, laid
		dep.Set(1)
		p.Frame()
		dep.Set(2)
		p.Frame()
		p.Close()
		if laid != wasLaid+2 {
			t.Fatalf("%s: the boundary was laid out %d times for two rebuilds beside it, want 2", tc.name, laid-wasLaid)
		}
		if was != 1 || leaf.layouts != was {
			t.Fatalf("%s: the leaf was laid out %d times, then %d more as the boundary was laid out again; want once, then none", tc.name, was, leaf.layouts-was)
		}
	}
}

// readsWidth is a leaf as wide as the signal it reads while laid out.
type readsWidth struct {
	w       *StateValue[float64]
	layouts int
}

func (r *readsWidth) Layout(cs Constraints, _ Env) Size {
	r.layouts++
	return cs.Constrain(Sz(r.w.Get(), 10))
}

func (r *readsWidth) Paint(*Canvas, Rect) {}

// A signal read deep inside If, Key, Each, View and Cached, each a cache of
// its own, measures the root afresh when it changes, and nothing at all
// when it does not.
func TestADeepReadReachesTheRootThroughEveryCache(t *testing.T) {
	t.Parallel()
	width := State(10.0)
	other := State(0)
	show := State(true)
	key := State(1)
	rows := State([]todo{{1, "a"}})
	leaf := &readsWidth{w: width}
	var tree Widget
	dispose := reactive.Observe(func() {
		tree = Cached(Column(
			View(other, func(int) Widget { return Box().Size(5, 5) }),
			If(show, func() Widget {
				return Key(key, func(int) Widget {
					return Cached(Column(EachKeyed(rows, func(t todo) int { return t.ID }, func(EachItem[todo]) Widget {
						return View(rows, func([]todo) Widget { return Cached(Row(leaf)) })
					})))
				})
			}),
		))
	})
	defer dispose()
	env := Untrack(UseEnv)
	layout := func() Size {
		reactive.Flush()
		return tree.Layout(Loose(Sz(500, 500)), env)
	}
	if got := layout(); got.W != 10 {
		t.Fatalf("width %v, want 10", got.W)
	}
	n := leaf.layouts
	layout()
	other.Set(1) // a sibling rebuilds; the leaf's subtree stands
	layout()
	if leaf.layouts != n {
		t.Fatalf("the leaf was laid out %d more times with nothing it reads changed", leaf.layouts-n)
	}
	width.Set(30)
	if got := layout(); got.W != 30 {
		t.Fatalf("width %v after the deep read changed, want 30", got.W)
	}
	width.Set(20)
	if got := layout(); got.W != 20 {
		t.Fatalf("width %v after it changed again, want 20", got.W)
	}
	n = leaf.layouts
	layout()
	if leaf.layouts != n {
		t.Fatal("the leaf was laid out again after settling")
	}
}
