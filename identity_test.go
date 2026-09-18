package ggui

import (
	"testing"
)

func TestComponentSetupIsStable(t *testing.T) {
	dep := State(0)
	setups := 0
	var local *StateValue[int]
	p := ProbeBuilder(func() Widget {
		return Component(func() Widget {
			setups++
			dep.Get()
			local = State(3)
			return View(local, func(n int) Widget { return Box().Size(float64(n), 1) })
		})
	}, Sz(100, 100))
	defer p.Close()
	p.Frame()
	local.Set(9)
	dep.Set(1)
	p.Frame()
	if setups != 1 || local.Get() != 9 {
		t.Fatal("setup reran or state was lost")
	}
}

func TestKeyDisposesAndRecreates(t *testing.T) {
	key := State(1)
	setups, cleanups := 0, 0
	p := ProbeBuilder(func() Widget {
		return Key(key, func(int) Widget {
			setups++
			OnCleanup(func() { cleanups++ })
			return Box()
		})
	}, Sz(10, 10))
	p.Frame()
	key.Set(1)
	p.Frame()
	if setups != 1 || cleanups != 0 {
		t.Fatal("equal key remounted")
	}
	key.Set(2)
	p.Frame()
	if setups != 2 || cleanups != 1 {
		t.Fatal("changed key did not remount")
	}
	p.Close()
	if cleanups != 2 {
		t.Fatal("close did not clean up")
	}
}

func TestComponentPropsAreReadable(t *testing.T) {
	n := State(1)
	var seen []int
	p := ProbeBuilder(func() Widget {
		return Component(func() Widget {
			return View(n, func(value int) Widget { seen = append(seen, value); return Box() })
		})
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	n.Set(2)
	p.Frame()
	if len(seen) != 2 || seen[1] != 2 {
		t.Fatalf("props: %v", seen)
	}
}

func TestScrollAdoptsOffsetAcrossRebuild(t *testing.T) {
	dep := State(0)
	p := ProbeBuilder(func() Widget {
		dep.Get()
		return Scroll(Box().Size(50, 500))
	}, Sz(50, 100))
	defer p.Close()
	p.Scroll(Pt(10, 10), Pt(0, -3))
	if off := p.root.(*ScrollWidget).offset; off != 60 {
		t.Fatalf("offset = %v, want 60", off)
	}
	dep.Set(1)
	p.Frame()
	if off := p.root.(*ScrollWidget).offset; off != 60 {
		t.Fatalf("after rebuild offset = %v, want 60", off)
	}
}

// keyedBox is a pointer handler with an identity and a counter it adopts.
type keyedBox struct {
	id    any
	count int
}

func (k *keyedBox) Layout(c Constraints, env Env) Size { return c.Constrain(Sz(10, 10)) }
func (k *keyedBox) Paint(dst *Canvas, r Rect)          { dst.HitPointer(r, k) }
func (k *keyedBox) HandlePointer(ev PointerEvent) bool { return true }
func (k *keyedBox) HitID() any                         { return k.id }
func (k *keyedBox) Adopt(prev any) {
	if p, ok := prev.(*keyedBox); ok {
		k.count = p.count
	}
}

func TestAdoptionFollowsIDNotRect(t *testing.T) {
	a := &keyedBox{id: "a", count: 5}
	a2, b := &keyedBox{id: "a"}, &keyedBox{id: "b"}
	gen := State(0)
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			if gen.Get() == 0 {
				return Column(a, Box().Size(10, 10))
			}
			// Rebuilt and moved: a2 has a's id at another Rect; b sits where a was.
			return Column(b, a2)
		})
	}, Sz(10, 100))
	defer p.Close()
	p.Frame()
	gen.Set(1)
	p.Frame()
	if a2.count != 5 || b.count != 0 {
		t.Fatalf("a2.count = %d, b.count = %d; want 5 and 0", a2.count, b.count)
	}
}

func TestCachedSeesProvideChange(t *testing.T) {
	k := NewEnvKey[float64]("w")
	layouts := 0
	leaf := FromFuncs(func(c Constraints, env Env) Size {
		layouts++
		w, _ := env.Get(k)
		return c.Constrain(Sz(w, 1))
	}, func(*Canvas, Rect) {})
	cached := Cached(leaf)
	env := rootEnv()
	at := func(v float64) Size { return Provide(k, v, cached).Layout(Loose(Sz(100, 100)), env) }
	if got := at(5).W; got != 5 {
		t.Fatalf("width = %v", got)
	}
	at(5)
	if layouts != 1 {
		t.Fatalf("same value laid out %d times, want 1", layouts)
	}
	if got := at(8).W; got != 8 || layouts != 2 {
		t.Fatalf("width = %v after %d layouts, want 8 after 2", got, layouts)
	}
}

func TestCachedTextInputFollowsSignal(t *testing.T) {
	v := State("a")
	in := TextInput(v)
	p := NewProbe(Cached(in), Sz(200, 30))
	p.Frame()
	v.Set("bbbbbbbb")
	p.Frame()
	p.Frame()
	if in.ed.text != "bbbbbbbb" {
		t.Fatalf("editor shows %q", in.ed.text)
	}
}

func TestComponentGivesControlsStableIdentity(t *testing.T) {
	value := State("hello")
	rebuild := State(0)
	var input *TextInputWidget
	p := ProbeBuilder(func() Widget {
		return Component(func() Widget {
			return Reactive(func() Widget { rebuild.Get(); input = TextInput(value); return input })
		})
	}, Sz(200, 100))
	defer p.Close()
	p.Frame()
	first := input
	if first.HitID() == nil {
		t.Fatal("missing identity")
	}
	p.Click(Pt(2, 5))
	p.Type(Mods{}, KeyArrowRight, KeyArrowRight)
	rebuild.Set(1)
	p.Frame()
	p.Type(Mods{}, KeyArrowRight)
	if first == input || first.HitID() != input.HitID() || input.ed.caret != 3 || !input.Focused() {
		t.Fatal("rebuild lost identity, caret or focus")
	}
}

func TestTabReachesAControlOutsideTheScrollWindow(t *testing.T) {
	items := Column(Focus(Box().Size(50, 50)), Focus(Box().Size(50, 50)), Focus(Box().Size(50, 50)))
	sc := Scroll(items)
	p := NewProbe(sc, Sz(50, 40))
	defer p.Close()
	p.Type(Mods{}, KeyTab, KeyTab, KeyTab)
	if sc.offset != 110 {
		t.Fatalf("offset = %v after tabbing to the third, fully hidden item; want 110", sc.offset)
	}
}

func TestRetainKeepsTheFocusedRow(t *testing.T) {
	items := State([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	built := map[int]int{}
	list := EachKeyed(items, func(i int) int { return i }, func(rowItem EachItem[int]) Widget {
		r := rowItem.Value
		built[r.Get()]++
		return Focus(Box().Size(50, 10))
	}).ItemExtent(10).Retain(1)
	sc := Scroll(list)
	p := NewProbe(sc, Sz(50, 30))
	defer p.Close()
	p.Click(Pt(5, 5)) // focus row 0
	p.Scroll(Pt(5, 5), Pt(0, -5))
	p.Frame()
	p.Frame()
	if sc.offset < 30 {
		t.Fatalf("offset %v, want the first rows out of view", sc.offset)
	}
	p.Scroll(Pt(5, 5), Pt(0, 5)) // back up
	p.Frame()
	if built[0] != 1 {
		t.Fatalf("row 0 built %d times: the focused row was evicted", built[0])
	}
	if built[1]+built[2] < 3 {
		t.Fatalf("rows 1 and 2 built %d and %d times: one unfocused row should have been evicted with Retain(1)", built[1], built[2])
	}
}
