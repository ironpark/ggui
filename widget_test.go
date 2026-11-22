package ggui

import (
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/reactive"
)

// A direct write to Inert, bypassing SetInert, still asks for a layout
// once Sync sees it.
func TestInteractiveDirectInertWriteRequestsLayout(t *testing.T) {
	t.Parallel()
	var s Interactive
	s.Sync()
	before := reactive.LayoutGen()
	s.SetInert(true)
	s.Sync()
	if reactive.LayoutGen() == before {
		t.Fatal("direct Inert write not noticed by Sync")
	}
	before = reactive.LayoutGen()
	s.Sync()
	if reactive.LayoutGen() != before {
		t.Fatal("Sync requested a layout with nothing changed")
	}
}

func TestConfigInspectorChord(t *testing.T) {
	t.Parallel()
	a := New(Config{Inspector: "f1"}, func() Widget { return sized(0, 0) })
	if a.inspectChord.Key != KeyF1 {
		t.Fatalf("chord key = %v", a.inspectChord.Key)
	}
	if New(Config{}, func() Widget { return sized(0, 0) }).inspectChord.Key != 0 {
		t.Fatal("empty Inspector parsed a chord")
	}
}

func TestComponentSetupIsStable(t *testing.T) {
	t.Parallel()
	dep := State(0)
	setups := 0
	var local *StateValue[int]
	p := ProbeBuilder(func() Widget {
		return Component(func() Widget {
			setups++
			dep.Get()
			local = State(3)
			return View(local, func(n int) Widget { return sized(float64(n), 1) })
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
	t.Parallel()
	key := State(1)
	setups, cleanups := 0, 0
	p := ProbeBuilder(func() Widget {
		return Key(key, func(int) Widget {
			setups++
			OnCleanup(func() { cleanups++ })
			return sized(0, 0)
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
	t.Parallel()
	n := State(1)
	var seen []int
	p := ProbeBuilder(func() Widget {
		return Component(func() Widget {
			return View(n, func(value int) Widget { seen = append(seen, value); return sized(0, 0) })
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
	t.Parallel()
	a := &keyedBox{id: "a", count: 5}
	a2, b := &keyedBox{id: "a"}, &keyedBox{id: "b"}
	gen := State(0)
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			if gen.Get() == 0 {
				return column(a, sized(10, 10))
			}
			// Rebuilt and moved: a2 has a's id at another Rect; b sits where a was.
			return column(b, a2)
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
	t.Parallel()
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

func TestFramesThatOnlyPaintKeepTheDerivedEnv(t *testing.T) {
	t.Parallel()
	k := NewEnvKey[float64]("w")
	gen := State(0)
	layouts := 0
	cached := Cached(FromFuncs(func(c Constraints, env Env) Size {
		layouts++
		return c.Constrain(Sz(10, 10))
	}, func(*Canvas, Rect) {}))
	provided := Provide(k, 1, cached)
	root := FromFuncs(func(c Constraints, env Env) Size {
		gen.Get()
		return provided.Layout(c, env)
	}, func(dst *Canvas, r Rect) { provided.Paint(dst, r) })
	p := NewProbe(root, Sz(100, 100))
	defer p.Close()
	p.Frame()
	p.Frame()
	p.Frame()
	gen.Set(1)
	p.Frame()
	if layouts != 1 {
		t.Fatalf("the cached leaf laid out %d times, want once: frames that only painted aged out the Env its cache is keyed by", layouts)
	}
}

// counting is a leaf that counts its layouts.
type counting struct{ layouts int }

func (c *counting) Layout(Constraints, Env) Size { c.layouts++; return Sz(10, 10) }
func (c *counting) Paint(*Canvas, Rect)          {}

func TestCachedSkipsLayoutUntilSomethingInsideChanges(t *testing.T) {
	t.Parallel()
	leaf := &counting{}
	dep := State(0)
	var inner Widget
	dispose := reactive.Observe(func() {
		inner = Reactive(func() Widget { dep.Get(); return column(leaf) })
	})
	defer dispose()
	c := Cached(inner)
	tree := column(c, sized(10, 10))
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
	reactive.Flush()
	tree.Layout(Tight(Sz(200, 100)), env)
	if leaf.layouts != 3 {
		t.Fatalf("leaf laid out %d times after a rebuild inside, want 3", leaf.layouts)
	}
	tree.Layout(Tight(Sz(200, 100)), env.WithText(TextStyle{Size: 20}))
	if leaf.layouts != 4 {
		t.Fatalf("leaf laid out %d times after the inherited style changed, want 4", leaf.layouts)
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
	t.Parallel()
	a, b := State(0), State(0)
	leafA, leafB := &countingLeaf{}, &countingLeaf{}
	p := ProbeBuilder(func() Widget {
		return column(
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

func TestACachedSkippedWhileOthersChangeKeepsTheCachesInside(t *testing.T) {
	t.Parallel()
	leaf := &counting{}
	depA, depB := State(0), State(0)
	a := Cached(column(Reactive(func() Widget { depA.Get(); return sized(10, 10) }), Cached(column(leaf))))
	b := Cached(column(Reactive(func() Widget { depB.Get(); return sized(10, 10) })))
	p := NewProbe(column(a, b), Sz(100, 100))
	defer p.Close()
	p.Frame()
	before := leaf.layouts
	// Frames that lay out only b leave a skipped: nothing inside it derives
	// its Envs, which must still be there when a is next laid out.
	for i := 1; i <= 5; i++ {
		depB.Set(i)
		p.Frame()
	}
	depA.Set(1)
	p.Frame()
	if leaf.layouts != before {
		t.Fatalf("a Cached inside a laid out again %d times after a sibling of it changed", leaf.layouts-before)
	}
}

func TestEnvMemoStaysBoundedAsDerivationsChurn(t *testing.T) {
	var m envMemo
	for i := range 100_000 {
		m.put(derivedKey{from: uint64(i), key: "k", val: i}, Env{})
	}
	if n := len(m.cur) + len(m.prev); n > 2*envMemoMin {
		t.Fatalf("the memo holds %d entries after churn, want at most %d", n, 2*envMemoMin)
	}
	k := derivedKey{from: 1, key: "kept"}
	m.put(k, Env{rev: 42})
	for i := range envMemoMin * 4 {
		m.get(k) // in use: it survives however many others come and go
		m.put(derivedKey{from: uint64(i), key: "other"}, Env{})
	}
	if e, ok := m.get(k); !ok || e.rev != 42 {
		t.Fatal("an entry in use aged out")
	}
}

func TestViewAndWhen(t *testing.T) {
	t.Parallel()
	s := State("a")
	on := State(true)
	then, other := labelled("then"), labelled("other")
	builds := 0
	root := column(View(s, func(v string) *label { builds++; return labelled(v) }), If(on, func() Widget {
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
	w := root.kids[1].(*IfWidget)
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
	t.Parallel()
	a, b := State(false), State(true)
	ta, tb, tc := labelled("A"), labelled("B"), labelled("C")
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

// Branches are constructed once and kept, so the one shown keeps its state
// across being hidden; the choice rebuilds the If alone, not its parent.
func TestIfKeepsBranchesAndRebuildsAlone(t *testing.T) {
	t.Parallel()
	on := State(true)
	then := labelled("kept")
	parentBuilds := 0
	var w *IfWidget
	p := ProbeBuilder(func() Widget {
		parentBuilds++
		w = If(on, func() Widget {
			return then
		}).Else(func() Widget {
			return labelled("other")
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
	t.Parallel()
	on := State(true)
	gen := State(0)
	var first *IfWidget
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			w := If(on, func() Widget {
				return labelled("a")
			}).Else(func() Widget {
				return labelled("b")
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

// ViewOf rebuilds when the part it picks changes and not when the rest of
// the state does, even under a parent that rebuilds it.
func TestViewOfRebuildsOnlyForItsPart(t *testing.T) {
	t.Parallel()
	type doc struct {
		Title    string
		Selected int
		Rows     []string // makes doc incomparable: every Set notifies
	}
	d := State(doc{Title: "a"})
	parent := State(0)
	var built []string
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			parent.Get()
			return ViewOf(d, func(d doc) string { return d.Title }, func(title string) *label {
				built = append(built, title)
				return labelled(title)
			})
		})
	}, Sz(200, 30))
	defer p.Close()
	p.Frame()
	d.Update(func(v doc) doc { v.Selected = 3; v.Rows = []string{"x"}; return v })
	p.Frame()
	d.Update(func(v doc) doc { v.Title = "b"; return v })
	p.Frame()
	parent.Set(1)
	p.Frame()
	d.Update(func(v doc) doc { v.Selected = 4; return v })
	p.Frame()
	if want := []string{"a", "b", "b"}; !slices.Equal(built, want) {
		t.Fatalf("built %q, want %q: once, once for the title, once for the parent", built, want)
	}
}

func TestPropertyBindingsRejectNilReaders(t *testing.T) {
	t.Parallel()
	var text *StateValue[string]
	var flag *StateValue[bool]
	cases := map[string]func(){
		"name":     func() { new(Interactive).BindName(text) },
		"disabled": func() { new(Interactive).BindInert(flag) },
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				v := recover()
				if v == nil || !strings.Contains(v.(string), "non-nil reader") {
					t.Fatalf("unexpected panic: %v", v)
				}
			}()
			run()
		})
	}
}

// mustPanic fails the test unless fn panics.
func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s did not panic", what)
		}
	}()
	fn()
}

func TestInsetsShorthand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   []float64
		want EdgeInsets
	}{
		{nil, EdgeInsets{}},
		{[]float64{8}, EdgeInsets{Top: 8, Right: 8, Bottom: 8, Left: 8}},
		{[]float64{4, 12}, EdgeInsets{Top: 4, Right: 12, Bottom: 4, Left: 12}},
		{[]float64{1, 2, 3, 4}, EdgeInsets{Top: 1, Right: 2, Bottom: 3, Left: 4}},
	}
	for _, c := range cases {
		if got := Insets(c.in...); got != c.want {
			t.Fatalf("Insets(%v) = %+v, want %+v", c.in, got, c.want)
		}
	}
	mustPanic(t, "Insets(1, 2, 3)", func() { Insets(1, 2, 3) })
}

// A subtree mounted in a Scope and disposed by its Cleanup is let go of by
// the scope's owner, so a list that mounts and evicts rows as it scrolls,
// in its own scope, holds only the rows it keeps.
func TestScopeLetsGoOfWhatItsCleanupDisposed(t *testing.T) {
	t.Parallel()
	var s Scope
	dispose := Root(func() { s = UseScope() })
	defer dispose()
	var kept []Cleanup
	mounted, cleaned := 0, 0
	for i := range 100 {
		_, cleanup := s.MountAs(i, "row", func() Widget {
			mounted++
			OnCleanup(func() { cleaned++ })
			return sized(10, 10)
		})
		kept = append(kept, cleanup)
		if len(kept) > 3 {
			kept[0]()
			kept = kept[1:]
		}
	}
	held := 0
	for _, c := range s.owner.Children() {
		if c.Disposed() {
			t.Fatal("the scope's owner holds a disposed subtree")
		}
		held++
	}
	if held != len(kept) || mounted-cleaned != len(kept) {
		t.Fatalf("the scope's owner holds %d subtrees, %d alive; want the %d kept", held, mounted-cleaned, len(kept))
	}
}

func TestFocusRefFocusFromPostNotSkippedAndCachedWidgetManyKids(t *testing.T) {
	t.Parallel()
	var ref FocusRef
	field := &editField{value: State("hi")}
	p := NewWindowProbe(func() Widget { return stack(ref.Attach(field)) }, Sz(100, 40))
	defer p.Close()
	p.Step(0)
	if field.Focused {
		t.Fatal("field focused before FocusRef.Focus")
	}
	p.w.Post(func() { ref.Focus() })
	fr := p.Step(0)
	if fr.Painted == FrameSkipped {
		t.Fatal("frame after FocusRef.Focus() in Post was skipped")
	}
	if !field.Focused {
		t.Fatal("FocusRef.Focus() in Post did not focus the widget")
	}

	// CachedWidget with > 32 child CachedWidgets exercises kidIdx lookup/update.
	kids := make([]Widget, 48)
	vals := make([]*StateValue[int], 48)
	for i := range kids {
		vals[i] = State(i)
		s := vals[i]
		kids[i] = Cached(FromFuncs(func(c Constraints, _ Env) Size {
			return c.Constrain(Sz(10, float64(s.Get()+1)))
		}, func(*Canvas, Rect) {}))
	}
	outer := Cached(column(kids...))
	outer.Layout(Loose(Sz(200, 5000)), Env{})
	if len(outer.kids) != 48 || len(outer.kidIdx) != 48 {
		t.Fatalf("kids=%d kidIdx=%d, want 48 and 48", len(outer.kids), len(outer.kidIdx))
	}
	vals[40].Set(100)
	sz := outer.Layout(Loose(Sz(200, 5000)), Env{})
	if sz.H <= 100 || len(outer.kids) != 48 || len(outer.kidIdx) != 48 {
		t.Fatalf("after update: sz=%v kids=%d kidIdx=%d", sz, len(outer.kids), len(outer.kidIdx))
	}
}
