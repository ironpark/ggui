package ggui

import (
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

// described paints w at size through a Probe and returns the published tree.
func described(t *testing.T, w Widget, size Size) *SemTree {
	t.Helper()
	p := NewProbe(w, size)
	t.Cleanup(p.Close)
	return p.Semantics()
}

func TestSemanticsNestingComesFromTheBuilder(t *testing.T) {
	t.Parallel()
	// Column registers nothing, so paint depth would fold the leaf under
	// the group beside it; the explicit scope is what says otherwise.
	inner := FromFuncs(
		func(c Constraints, _ Env) Size { return c.Constrain(Sz(50, 20)) },
		func(dst *Canvas, r Rect) {
			dst.Leaf(r, Node{Role: RoleButton, Name: "one"})
			dst.Node(r, Node{Role: RoleGroup, Name: "box"}, func(dst *Canvas) {
				dst.Leaf(r, Node{Role: RoleButton, Name: "two"})
			})
			dst.Leaf(r, Node{Role: RoleButton, Name: "three"})
		})
	got := described(t, Column(inner), Sz(100, 100)).String()
	want := `button "one"
group "box"
  button "two"
button "three"
`
	if got != want {
		t.Fatalf("tree =\n%s\nwant\n%s", got, want)
	}
}

func TestSemanticsKeepsClippedNodesWithTheirBounds(t *testing.T) {
	t.Parallel()
	// A node scrolled out of view is present, flagged, and still says where
	// it is, so an assistive technology can ask to be taken there.
	tall := FromFuncs(
		func(c Constraints, _ Env) Size { return Sz(c.MaxW, 400) },
		func(dst *Canvas, r Rect) {
			dst.Leaf(Rct(r.Origin, Sz(r.Size.W, 20)), Node{Role: RoleButton, Name: "top"})
			dst.Leaf(Rct(Pt(r.Origin.X, r.Origin.Y+300), Sz(r.Size.W, 20)), Node{Role: RoleButton, Name: "far"})
		})
	tree := described(t, Scroll(tall), Sz(100, 50))
	far, ok := tree.Find(RoleButton, "far")
	if !ok {
		t.Fatal("the clipped node was dropped instead of flagged")
	}
	if !far.Offscreen {
		t.Error("far.Offscreen = false, want true")
	}
	if far.Full.Origin.Y != 300 || far.Full.Size.H != 20 {
		t.Errorf("far.Full = %v, want the bounds it painted at", far.Full)
	}
	if top, _ := tree.Find(RoleButton, "top"); top.Offscreen {
		t.Error("the visible node was flagged offscreen")
	}
}

// twice describes itself from two places, as ui.TextField does around the
// editor inside it.
type twice struct{ Interactive }

func (t *twice) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(40, 20)) }
func (t *twice) Paint(dst *Canvas, r Rect) {
	t.Hit(dst, r, t, CursorShapeText)
	dst.Describe(Rct(r.Origin, Sz(20, 10)), t)
}
func (t *twice) HandlePointer(PointerEvent) bool { return true }
func (t *twice) HandleKey(KeyEvent)              {}

func TestSemanticsDescribesOneHandlerOnce(t *testing.T) {
	t.Parallel()
	w := &twice{}
	w.Role = RoleTextField
	w.SetName("Name")
	tree := described(t, w, Sz(100, 100))
	n := 0
	for range tree.Nodes(RoleTextField) {
		n++
	}
	if n != 1 {
		t.Fatalf("%d textfield nodes, want 1:\n%s", n, tree)
	}
	if got := tree.At(0).Rect.Size; got == (Size{W: 20, H: 10}) {
		t.Error("kept the inner node; the first description of a handler wins")
	}
}

func TestSemanticsSnapshotIsFrozen(t *testing.T) {
	t.Parallel()
	label := State("a")
	p := ProbeBuilder(func() Widget { return TextOf(label) }, Sz(100, 100))
	defer p.Close()
	first := p.Semantics()
	before := first.String()
	label.Set("b")
	second := p.Semantics()
	if first == second {
		t.Fatal("the same tree was published twice")
	}
	if first.String() != before {
		t.Errorf("the published tree changed under its reader:\n%s", first)
	}
	if !strings.Contains(second.String(), `"b"`) {
		t.Errorf("the new tree missed the new text:\n%s", second)
	}
}

func TestSemanticsMirrorsFocus(t *testing.T) {
	t.Parallel()
	one, two := Focus(Box().Size(40, 20)), Focus(Box().Size(40, 20))
	p := NewProbe(Column(one, two), Sz(100, 100))
	defer p.Close()
	p.Click(Pt(20, 10))
	if _, ok := p.Semantics().Focused(); ok {
		t.Error("a bare Focus describes nothing, so nothing can be focused in the tree")
	}
	// A described control is found by its handler, whatever it moved to.
	w := &twice{}
	w.Role = RoleTextField
	w.SetName("Name")
	q := NewProbe(w, Sz(100, 100))
	defer q.Close()
	q.Click(Pt(10, 5))
	n, ok := q.Semantics().Focused()
	if !ok || n.Name != "Name" {
		t.Fatalf("focused node = %v, %v; want the text field", n, ok)
	}
}

type uncomparableFocus []int

func (uncomparableFocus) HandleKey(KeyEvent) {}

func TestFocusedNodePrefersHandlerAndFallsBackAfterRebuild(t *testing.T) {
	t.Parallel()
	c := Canvas{}
	r := Rct(Pt(0, 0), Sz(100, 20))
	first, exact, rebuilt := &twice{}, &twice{}, &twice{}
	for _, w := range []*twice{first, exact, rebuilt} {
		w.Role = RoleTextField
		w.SetKey("shared")
	}
	c.Describe(r, first)
	c.Describe(r, exact)
	for _, test := range []struct {
		name string
		hit  hitRegion
		want int
	}{
		{"exact before identity", hitRegion{key: exact, id: "shared", rect: r}, 1},
		{"rebuilt identity", hitRegion{key: rebuilt, id: "shared", rect: Rct(Pt(0, 50), r.Size)}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := focusedNode(&c, &test.hit); got != test.want {
				t.Fatalf("focused node = %d, want %d", got, test.want)
			}
		})
	}
	// No handler index entry exists for a noncomparable handler; the
	// unkeyed geometry fallback must still work without a map-key panic.
	c.resetSemantics()
	h := uncomparableFocus{1}
	c.addSem(r, Node{Role: RoleTextField}, h)
	if got := focusedNode(&c, &hitRegion{key: h, rect: r}); got != 0 {
		t.Fatalf("uncomparable handler fallback = %d, want 0", got)
	}
	if got := focusedNode(&c, &hitRegion{key: &twice{}, rect: r}); got != 0 {
		t.Fatalf("rebuilt unkeyed handler fallback = %d, want 0", got)
	}
}

func TestSemanticsListCountsItems(t *testing.T) {
	t.Parallel()
	items := State([]string{"a", "b", "c"})
	tree := described(t, Each(items, func(rowItem EachItem[string]) Widget { s := rowItem.Value; return TextOf(s) }), Sz(100, 200))
	list, ok := tree.Find(RoleList, "")
	if !ok {
		t.Fatalf("no list:\n%s", tree)
	}
	if list.Max != 3 {
		t.Errorf("list.Max = %g, want 3", list.Max)
	}
	if len(list.Children) != 3 {
		t.Fatalf("%d items, want 3:\n%s", len(list.Children), tree)
	}
	third := tree.At(list.Children[2])
	if third.Role != RoleListItem || third.Now != 3 || third.Max != 3 {
		t.Errorf("third item = %v, want item 3 of 3", third.Node)
	}
	if kids := third.Children; len(kids) != 1 || tree.At(kids[0]).Name != "c" {
		t.Errorf("the item does not hold its row:\n%s", tree)
	}
}

func TestSemanticsTextRoles(t *testing.T) {
	t.Parallel()
	tree := described(t, Column(Text("Heading").Role(RoleHeading), Text("body"), Text("")), Sz(200, 200))
	want := `heading "Heading"
text "body"
`
	if got := tree.String(); got != want {
		t.Fatalf("tree =\n%s\nwant\n%s", got, want)
	}
}

func TestSemanticsStagingReusePreservesOldTree(t *testing.T) {
	t.Parallel()
	rows := State([]string{"one", "two", "three"})
	p := ProbeBuilder(func() Widget {
		return Each(rows, func(rowItem EachItem[string]) Widget { s := rowItem.Value; return TextOf(s) })
	}, Sz(100, 200))
	defer p.Close()
	old := p.Semantics()
	before := old.String()
	rows.Set([]string{"new"})
	current := p.Semantics()
	if old.String() != before || old.At(0).Max != 3 {
		t.Fatal("reusing staging nodes changed an already published tree")
	}
	if current.At(0).Max != 1 || !strings.Contains(current.String(), `"new"`) {
		t.Fatalf("new snapshot has stale nodes: %s", current)
	}
	// Shrinking must release references in unused staging slots as well.
	for _, n := range p.canvas.fs().sem[len(p.canvas.fs().sem):cap(p.canvas.fs().sem)] {
		if n.handler != nil || n.node.Name != "" || n.node.Runs != nil {
			t.Fatal("unused staging slot retains a previous frame's data")
		}
	}
}

func TestSemTreeReusesEqualValuesAndFreezesDescriptions(t *testing.T) {
	expanded := false
	runs := []TextRun{{Start: 0, End: 2, Rect: Rct(Pt(1, 2), Sz(20, 10)), Stops: []TextStop{{Byte: 0, X: 0}, {Byte: 2, X: 20}}}}
	n := Node{Role: RoleTextField, Name: "editor", Expanded: &expanded, Runs: runs}
	c := &Canvas{}
	c.Leaf(Rct(Pt(0, 0), Sz(100, 20)), n)
	first := buildSemTree(c, nil, nil)
	// Equal contents in fresh buffers must compare by value.
	c.fs().sem[0].node = freezeNode(n)
	if next := buildSemTree(c, nil, first); next != first {
		t.Fatal("equal description allocated a new snapshot")
	}
	// A custom widget may reuse the buffers supplied in the previous frame.
	expanded = true
	runs[0].End = 3
	runs[0].Stops[1].X = 30
	c.fs().sem[0].node = n
	second := buildSemTree(c, nil, first)
	if second == first || !*second.At(0).Expanded || second.At(0).Runs[0].Stops[1].X != 30 {
		t.Fatal("new frame lost changed description values")
	}
	if *first.At(0).Expanded || first.At(0).Runs[0].End != 2 || first.At(0).Runs[0].Stops[1].X != 20 {
		t.Fatal("widget buffer reuse mutated an older snapshot")
	}
	if allocs := testing.AllocsPerRun(100, func() { buildSemTree(c, nil, second) }); allocs != 0 {
		t.Fatalf("equal descriptions allocated %g times", allocs)
	}
}

// Exercise every field, including nested structs/slices. A field added to
// Node, TextRun or TextStop must participate in equality without having to
// remember to add a new test case beside the production comparison. An
// unexported field is what a11y.Build derives from the rest, not part of
// a description, so it is left out.
func semanticFieldChanges(v reflect.Value, visit func()) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				semanticFieldChanges(v.Field(i), visit)
			}
		}
	case reflect.Bool:
		v.SetBool(true)
		visit()
		v.SetBool(false)
	case reflect.String:
		v.SetString("changed")
		visit()
		v.SetString("")
	case reflect.Int:
		v.SetInt(1)
		visit()
		v.SetInt(0)
	case reflect.Uint8, reflect.Uint32:
		v.SetUint(1)
		visit()
		v.SetUint(0)
	case reflect.Float64:
		v.SetFloat(1)
		visit()
		v.SetFloat(0)
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		visit()
		semanticFieldChanges(v.Elem(), visit)
		v.SetZero()
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
		visit()
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		visit()
		semanticFieldChanges(v.Index(0), visit)
		v.SetZero()
	default:
		panic("add equality coverage for " + v.Type().String())
	}
}

func TestNodeEqualityCoversEveryField(t *testing.T) {
	t.Parallel()
	var n Node
	semanticFieldChanges(reflect.ValueOf(&n).Elem(), func() {
		// Use a populated baseline too, so changes inside a slice cannot
		// hide behind a difference in slice length or nilness.
		copy := freezeNode(n)
		if !sameNode(&n, &copy) {
			t.Fatalf("equal nodes compared unequal: %#v", n)
		}
		var zero Node
		if sameNode(&zero, &n) != reflect.DeepEqual(zero, n) {
			t.Fatalf("equality missed changed field: %#v", n)
		}
	})
	for _, typ := range []reflect.Type{reflect.TypeFor[Node](), reflect.TypeFor[TextRun](), reflect.TypeFor[TextStop]()} {
		for i := 0; i < typ.NumField(); i++ {
			if !typ.Field(i).IsExported() {
				continue
			}
			t.Run(typ.Name()+"/"+typ.Field(i).Name, func(t *testing.T) {
				a := Node{Expanded: Expandable(false), Runs: []TextRun{{Stops: []TextStop{{}}}}}
				b := freezeNode(a)
				value := reflect.ValueOf(&b).Elem()
				if typ == reflect.TypeFor[TextRun]() {
					value = reflect.ValueOf(&b.Runs[0]).Elem()
				} else if typ == reflect.TypeFor[TextStop]() {
					value = reflect.ValueOf(&b.Runs[0].Stops[0]).Elem()
				}
				semanticFieldChanges(value.Field(i), func() {
					if sameNode(&a, &b) != reflect.DeepEqual(a, b) {
						t.Fatal("comparison disagrees with DeepEqual")
					}
				})
			})
		}
	}
}

func TestNodeEqualityMatchesDeepEqual(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(1, 2))
	for range 1000 {
		a := Node{Role: RoleTextField, Name: "a", Value: "xy", Expanded: Expandable(rng.IntN(2) == 0),
			SelStart: rng.IntN(3), SelEnd: rng.IntN(3),
			Runs: []TextRun{{Start: rng.IntN(3), End: rng.IntN(3), Stops: []TextStop{{Byte: rng.IntN(3), X: rng.Float64()}}}}}
		b := freezeNode(a)
		if rng.IntN(2) == 0 {
			b.Runs[0].Stops[0].X = rng.Float64()
		}
		if sameNode(&a, &b) != reflect.DeepEqual(a, b) {
			t.Fatal("comparison disagrees with DeepEqual")
		}
	}
}

func TestSemTreeInvalidatesGeometryIdentityHierarchyAndFocus(t *testing.T) {
	t.Parallel()
	w := &twice{}
	w.Role = RoleTextField
	c := &Canvas{}
	r := Rct(Pt(0, 0), Sz(100, 20))
	c.Node(r, Node{Role: RoleGroup}, func(c *Canvas) { c.Describe(r, w) })
	base := buildSemTree(c, nil, nil)
	for _, test := range []struct {
		name string
		edit func(*Canvas)
	}{
		{"clipped rect", func(c *Canvas) { c.fs().sem[1].rect.Size.W-- }},
		{"full rect", func(c *Canvas) { c.fs().sem[1].full.Origin.Y++ }},
		{"identity", func(c *Canvas) { c.fs().sem[1].id = "new" }},
		{"uncomparable identity", func(c *Canvas) { c.fs().sem[1].id = []int{1} }},
		{"parent", func(c *Canvas) { c.fs().sem[1].parent = 0 }},
		{"removed", func(c *Canvas) { c.fs().sem = c.fs().sem[:1] }},
	} {
		t.Run(test.name, func(t *testing.T) {
			dst := *c
			dst.fs().sem = append([]semNode(nil), c.fs().sem...)
			test.edit(&dst)
			if buildSemTree(&dst, nil, base) == base {
				t.Fatal("changed frame reused stale snapshot")
			}
		})
	}
	focus := &hitRegion{key: w, rect: r}
	next := buildSemTree(c, focus, base)
	if next == base || next.FocusIndex() != 1 || buildSemTree(c, nil, next) == next {
		t.Fatal("focus changes were not published")
	}
	empty := &Canvas{}
	first := buildSemTree(empty, nil, next)
	if first.Len() != 0 || buildSemTree(empty, nil, first) != first {
		t.Fatal("empty snapshot was not reusable")
	}
}

func TestSemTreeOldSnapshotCanBeReadDuringBufferReuse(t *testing.T) {
	t.Parallel()
	c := &Canvas{}
	n := Node{Role: RoleTextField, Expanded: Expandable(false), Runs: []TextRun{{Stops: []TextStop{{}}}}}
	c.Leaf(Rect{}, n)
	old := buildSemTree(c, nil, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 1000 {
			e := old.At(0)
			if *e.Expanded || e.Runs[0].Stops[0].X != 0 {
				t.Error("published snapshot changed")
				return
			}
		}
	}()
	for i := range 1000 {
		*n.Expanded = i%2 == 0
		n.Runs[0].Stops[0].X = float64(i)
		buildSemTree(c, nil, old)
	}
	<-done
}

func TestAnnounceQueuesWithoutAConsumer(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return Box() })
	a.Announce("Saved", Polite)
	a.Announce("", Assertive) // nothing to say, nothing queued
	a.Announce("Disk full", Assertive)
	got := a.takeAnnouncements()
	if len(got) != 2 || got[0] != (Announcement{Text: "Saved"}) ||
		got[1] != (Announcement{Text: "Disk full", Politeness: Assertive}) {
		t.Fatalf("queue = %v", got)
	}
	if rest := a.takeAnnouncements(); rest != nil {
		t.Errorf("taking the queue left %v behind", rest)
	}
}

func TestPerformDoesNotWaitForTheUIGoroutine(t *testing.T) {
	t.Parallel()
	// An action is queued and returns at once: the platform thread that
	// asked is blocked, and so may the UI goroutine be, so nothing may
	// wait on anything. Without a frame, nothing has run.
	a := New(Config{}, func() Widget { return Box() })
	done := make(chan struct{})
	go func() {
		a.Perform(NodeID{Rect: Rct(Pt(0, 0), Sz(10, 10))}, Action{Kind: ActionPress})
		close(done)
	}()
	<-done
	a.postMu.Lock()
	queued := len(a.posted)
	a.postMu.Unlock()
	if queued != 1 {
		t.Fatalf("%d posted, want the action waiting for the UI goroutine", queued)
	}
}

func TestPerformFocusesAndScrollsIntoView(t *testing.T) {
	t.Parallel()
	w := &twice{}
	w.Role = RoleTextField
	w.SetName("Name")
	tall := Column(Box().Size(40, 200), w)
	p := NewProbe(Scroll(tall), Sz(100, 60))
	defer p.Close()
	field, ok := p.Semantics().Find(RoleTextField, "Name")
	if !ok {
		t.Fatal("no field")
	}
	if !field.Offscreen {
		t.Fatal("the field should start out of view")
	}
	p.Perform(field.ID, Action{Kind: ActionFocus})
	if !p.Focused() {
		t.Fatal("the field did not take focus")
	}
	// Focus moved by the keyboard reveals, and the action goes through the
	// same path, so the scroll came along.
	if now, _ := p.Semantics().Find(RoleTextField, "Name"); now.Offscreen {
		t.Error("the field was focused but not scrolled into view")
	}
}
