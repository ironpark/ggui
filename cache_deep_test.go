package ggui

import (
	"testing"

	"github.com/ironpark/ggui/internal/reactive"
)

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
	env := rootEnv()
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
