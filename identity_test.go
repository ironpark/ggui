package ggui

import (
	"testing"
)

func TestKeyedSurvivesParentRebuild(t *testing.T) {
	parentDep := State(0)
	setups := 0
	var local *Signal[int]
	p := ProbeBuilder(func() Widget {
		parentDep.Get()
		return Column(Keyed("k", func() Builder {
			setups++
			local = State(3)
			return func() Widget { return Box().Size(float64(local.Get()), 1) }
		}))
	}, Sz(100, 100))
	defer p.Close()
	p.Frame()
	local.Set(9)
	parentDep.Set(1) // parent rebuilds; the keyed instance is claimed again
	p.Frame()
	p.Frame()
	if setups != 1 {
		t.Fatalf("setups = %d, want 1", setups)
	}
	root := p.root
	c := root.(*ColumnWidget).children[0].(*ComponentWidget)
	if got := c.Layout(Loose(Sz(100, 100)), Env{}).W; got != 9 {
		t.Fatalf("width = %v, want 9 from the kept state", got)
	}
}

func TestKeyedDisposedWhenUnclaimed(t *testing.T) {
	show := State(true)
	cleanups := 0
	var root Widget
	dispose := Effect(func() {
		if show.Get() {
			root = Keyed("k", func() Builder {
				OnCleanup(func() { cleanups++ })
				return func() Widget { return Box() }
			})
		} else {
			root = Box()
		}
	})
	defer dispose()
	p := NewProbe(root, Sz(10, 10))
	p.Frame()
	show.Set(false)
	effects.flush()
	if cleanups != 1 {
		t.Fatalf("cleanups = %d, want 1", cleanups)
	}
}

func TestMountUpdatesProps(t *testing.T) {
	n := State(1)
	var seen []int
	p := ProbeBuilder(func() Widget {
		return Mount("m", n.Get(), func(p *Signal[int]) Builder {
			return func() Widget { seen = append(seen, p.Get()); return Box() }
		})
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	n.Set(2)
	p.Frame()
	p.Frame()
	if len(seen) != 2 || seen[1] != 2 {
		t.Fatalf("builder saw %v, want [1 2]", seen)
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
		if gen.Get() == 0 {
			return Column(a, Box().Size(10, 10))
		}
		// Rebuilt and moved: a2 has a's id at another Rect; b sits where a was.
		return Column(b, a2)
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

func TestKeyedGivesControlsAnIdentity(t *testing.T) {
	v := State("hello")
	extra := State(false)
	rebuild := State(0)
	var in *TextInputWidget
	p := ProbeBuilder(func() Widget {
		form := Keyed("form", func() Builder {
			return func() Widget {
				rebuild.Get()
				in = TextInput(v)
				return in
			}
		})
		if extra.Get() {
			// A sibling appears above, so the field moves; its parent
			// rebuilt too, so it is a new widget.
			return Column(Box().Size(10, 10), form)
		}
		return Column(form)
	}, Sz(200, 100))
	defer p.Close()
	p.Frame()
	if in.HitID() == nil {
		t.Fatal("a TextInput built inside Keyed has no identity")
	}
	p.Click(Pt(2, 5))
	p.Type(Mods{}, KeyArrowRight, KeyArrowRight)
	first := in
	if first.ed.caret != 2 {
		t.Fatalf("caret %d before the rebuild, want 2", first.ed.caret)
	}
	extra.Set(true)
	rebuild.Set(1) // the keyed component's builder re-runs as well
	p.Frame()
	p.Frame()
	p.Type(Mods{}, KeyArrowRight)
	if in == first || in.HitID() != first.HitID() {
		t.Fatal("the field was not rebuilt with the same identity")
	}
	if in.ed.caret != 3 || !in.Focused() || !p.Focused() {
		t.Fatalf("caret %d focused %v after the field moved and rebuilt; want the caret and focus kept", in.ed.caret, in.Focused())
	}
}

func TestDuplicateKeyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("two Keyed with one key in a build did not panic")
		}
	}()
	dispose := Effect(func() {
		Keyed("k", func() Builder { return func() Widget { return Box() } })
		Keyed("k", func() Builder { return func() Widget { return Box() } })
	})
	defer dispose()
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
	list := For(items, func(i int) int { return i }, func(r Reader[int]) Widget {
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
