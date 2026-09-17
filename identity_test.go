package ggui

import "testing"

func TestKeyedSurvivesParentRebuild(t *testing.T) {
	parentDep := State(0)
	setups := 0
	var local *Signal[int]
	var root Widget
	dispose := Effect(func() {
		parentDep.Get()
		root = Column(Keyed("k", func() Builder {
			setups++
			local = State(3)
			return func() Widget { return Box().Size(float64(local.Get()), 1) }
		}))
	})
	defer dispose()
	p := NewProbe(root, Sz(100, 100))
	p.Frame()
	local.Set(9)
	parentDep.Set(1) // parent rebuilds; the keyed instance is claimed again
	p.root = root
	p.Frame()
	p.root = root
	p.Frame()
	if setups != 1 {
		t.Fatalf("setups = %d, want 1", setups)
	}
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
	var root Widget
	dispose := Effect(func() {
		root = Mount("m", n.Get(), func(p *Signal[int]) Builder {
			return func() Widget { seen = append(seen, p.Get()); return Box() }
		})
	})
	defer dispose()
	p := NewProbe(root, Sz(10, 10))
	p.Frame()
	n.Set(2)
	p.Frame()
	p.root = root
	p.Frame()
	if len(seen) != 2 || seen[1] != 2 {
		t.Fatalf("builder saw %v, want [1 2]", seen)
	}
}

func TestScrollAdoptsOffsetAcrossRebuild(t *testing.T) {
	dep := State(0)
	var root Widget
	dispose := Effect(func() {
		dep.Get()
		root = Scroll(Box().Size(50, 500))
	})
	defer dispose()
	p := NewProbe(root, Sz(50, 100))
	p.Scroll(Pt(10, 10), Pt(0, -3))
	if off := root.(*ScrollWidget).offset; off != 60 {
		t.Fatalf("offset = %v, want 60", off)
	}
	dep.Set(1)
	effects.flush()
	p.root = root
	p.Frame()
	if off := root.(*ScrollWidget).offset; off != 60 {
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
	p := NewProbe(Column(a, Box().Size(10, 10)), Sz(10, 100))
	p.Frame()
	// Rebuilt and moved: a2 has a's id at another Rect; b sits where a was.
	a2, b := &keyedBox{id: "a"}, &keyedBox{id: "b"}
	p.root = Column(b, a2)
	p.Frame()
	if a2.count != 5 || b.count != 0 {
		t.Fatalf("a2.count = %d, b.count = %d; want 5 and 0", a2.count, b.count)
	}
}

func TestCachedSeesProvideChange(t *testing.T) {
	k := NewKey[float64]("w")
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
