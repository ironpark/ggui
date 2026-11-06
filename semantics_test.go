package ggui

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui/a11y"
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
	var b a11y.Builder
	runs := []TextRun{{Start: 0, End: 2, Rect: Rct(Pt(1, 2), Sz(20, 10)), Stops: []TextStop{{Byte: 0, X: 0}, {Byte: 2, X: 20}}}}
	n := Node{Role: RoleTextField, Name: "editor", Expanded: Expandable(false), Runs: runs}
	c := &Canvas{}
	c.Leaf(Rct(Pt(0, 0), Sz(100, 20)), n)
	first := buildSemTree(c, nil, nil, new(a11y.Builder))
	// Equal contents in fresh buffers must compare by value.
	c.fs().sem[0].node = freezeNode(n)
	if next := buildSemTree(c, nil, first, new(a11y.Builder)); next != first {
		t.Fatal("equal description allocated a new snapshot")
	}
	// A custom widget may reuse the buffers supplied in the previous frame.
	n.Expanded = Expandable(true)
	runs[0].End = 3
	runs[0].Stops[1].X = 30
	c.fs().sem[0].node = n
	second := buildSemTree(c, nil, first, new(a11y.Builder))
	if second == first || !second.At(0).Expanded.Open() || second.At(0).Runs[0].Stops[1].X != 30 {
		t.Fatal("new frame lost changed description values")
	}
	if first.At(0).Expanded.Open() || first.At(0).Runs[0].End != 2 || first.At(0).Runs[0].Stops[1].X != 20 {
		t.Fatal("widget buffer reuse mutated an older snapshot")
	}
	if allocs := testing.AllocsPerRun(100, func() { buildSemTree(c, nil, second, &b) }); allocs != 0 {
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
	base := buildSemTree(c, nil, nil, new(a11y.Builder))
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
			if buildSemTree(&dst, nil, base, new(a11y.Builder)) == base {
				t.Fatal("changed frame reused stale snapshot")
			}
		})
	}
	focus := &hitRegion{key: w, rect: r}
	next := buildSemTree(c, focus, base, new(a11y.Builder))
	if next == base || next.FocusIndex() != 1 || buildSemTree(c, nil, next, new(a11y.Builder)) == next {
		t.Fatal("focus changes were not published")
	}
	empty := &Canvas{}
	first := buildSemTree(empty, nil, next, new(a11y.Builder))
	if first.Len() != 0 || buildSemTree(empty, nil, first, new(a11y.Builder)) != first {
		t.Fatal("empty snapshot was not reusable")
	}
}

func TestSemTreeOldSnapshotCanBeReadDuringBufferReuse(t *testing.T) {
	t.Parallel()
	c := &Canvas{}
	n := Node{Role: RoleTextField, Expanded: Expandable(false), Runs: []TextRun{{Stops: []TextStop{{}}}}}
	c.Leaf(Rect{}, n)
	old := buildSemTree(c, nil, nil, new(a11y.Builder))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 1000 {
			e := old.At(0)
			if e.Expanded.Open() || e.Runs[0].Stops[0].X != 0 {
				t.Error("published snapshot changed")
				return
			}
		}
	}()
	for i := range 1000 {
		n.Runs[0].Stops[0].X = float64(i)
		buildSemTree(c, nil, old, new(a11y.Builder))
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

// The accessibility tests that need a running app: a real widget tree, a
// Probe driving it, and the bridge reading what the frame published. The
// bridge's own unit tests live beside it, in the a11y package.

// axBell counts presses, and says so, which is what the round trip through
// the bridge has to arrive at.
type axBell struct {
	Interactive
	rung int
}

func (b *axBell) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(40, 20)) }
func (b *axBell) Paint(dst *Canvas, r Rect)        { b.Hit(dst, r, b, 0) }
func (b *axBell) HandlePointer(PointerEvent) bool  { return true }
func (b *axBell) HandleKey(ev KeyEvent)            { b.Keyboard(ev, func() { b.rung++ }) }

// axDetailed runs a frame with the detail a text field only freezes while
// something is reading it, and puts the flag back.
func axDetailed(t *testing.T, p *Probe) *SemTree {
	t.Helper()
	a11y.SetWantsDetail(true)
	t.Cleanup(func() { a11y.SetWantsDetail(false) })
	return p.Semantics()
}

func TestAXTextFieldSaysWhereItsCharactersAre(t *testing.T) {
	value := State("hello world")
	w := TextInput(value)
	p := NewProbe(w, Sz(300, 40))
	defer p.Close()

	field, ok := axDetailed(t, p).Find(RoleTextField, "")
	if !ok {
		t.Fatal("no text field in the tree")
	}
	if got := a11y.CharCount(field.Node); got != 11 {
		t.Errorf("character count = %d, want 11", got)
	}
	if got := a11y.StringForRange(field.Node, 6, 5); got != "world" {
		t.Errorf("string for range = %q, want %q", got, "world")
	}
	if len(field.Runs) != 1 {
		t.Fatalf("%d runs, want one line", len(field.Runs))
	}
	if n := len(field.Runs[0].Stops); n != 12 {
		t.Errorf("%d stops, want one per character boundary (12)", n)
	}
	// The caret sits at the end after TextInput loaded the value, and the
	// field is one line, so that is where the insertion point is.
	if got := a11y.InsertionLine(field.Node); got != 0 {
		t.Errorf("insertion line = %d, want 0", got)
	}
	if loc, length, has := a11y.RangeForLine(field.Node, 0); !has || loc != 0 || length != 11 {
		t.Errorf("line 0 = %d+%d, %v; want the whole field", loc, length, has)
	}
	if _, _, has := a11y.RangeForLine(field.Node, 3); has {
		t.Error("a field with one line answered for line 3")
	}
	// A character's box is inside the field, is not empty, and moves right
	// as the offset does.
	first := a11y.RectForRange(field, 0, 1)
	later := a11y.RectForRange(field, 6, 1)
	if first.Empty() || later.Empty() {
		t.Fatalf("character boxes = %v, %v; want real rectangles", first, later)
	}
	if later.Origin.X <= first.Origin.X {
		t.Errorf("character 6 at x=%g is not right of character 0 at x=%g", later.Origin.X, first.Origin.X)
	}
	if first.Origin.X < field.Full.Origin.X || later.Origin.X > field.Full.Origin.X+field.Full.Size.W {
		t.Errorf("character boxes fell outside the field %v", field.Full)
	}
}

// Not parallel: whether anything is reading is the process's, and the tests
// that turn it on with axDetailed run one at a time beside this one.
func TestAXTextDetailIsOnlyFrozenWhenSomethingIsReading(t *testing.T) {
	p := NewProbe(TextInput(State("hello")), Sz(300, 40))
	defer p.Close()
	field, ok := p.Semantics().Find(RoleTextField, "")
	if !ok {
		t.Fatal("no text field")
	}
	if field.Runs != nil {
		t.Error("the layout was frozen with nobody reading it; that is a measurement per character per frame")
	}
	// The selection is cheap and always there, so that the caret is known
	// the moment a bridge attaches.
	if field.SelEnd != len("hello") {
		t.Errorf("caret = %d, want the end of the text", field.SelEnd)
	}
	// Without a layout the field is still one line covering everything.
	if loc, length, has := a11y.RangeForLine(field.Node, 0); !has || loc != 0 || length != 5 {
		t.Errorf("line 0 = %d+%d, %v; want the whole field", loc, length, has)
	}
	if got := a11y.RectForRange(field, 0, 1); got != field.Full {
		t.Errorf("rect for range = %v, want the whole field %v", got, field.Full)
	}
}

func TestAXPasswordKeepsItsShapeToItself(t *testing.T) {
	p := NewProbe(TextInput(State("hunter2")).Password(), Sz(300, 40))
	defer p.Close()
	field, _ := axDetailed(t, p).Find(RoleTextField, "")
	if field.Runs != nil {
		t.Error("a password field froze its character positions")
	}
}

func TestAXSetSelectionMovesTheCaret(t *testing.T) {
	t.Parallel()
	w := TextInput(State("hello world"))
	p := NewProbe(w, Sz(300, 40))
	defer p.Close()
	field, ok := p.Semantics().Find(RoleTextField, "")
	if !ok {
		t.Fatal("no text field")
	}
	if !field.Actions.Has(ActionSetSelection) {
		t.Fatal("a text field does not offer its selection")
	}
	start, end := a11y.ByteRange(field.Node, 6, 5)
	p.Perform(field.ID, Action{Kind: ActionSetSelection, SelStart: start, SelEnd: end})
	after, _ := p.Semantics().Find(RoleTextField, "")
	if after.SelStart != 6 || after.SelEnd != 11 {
		t.Errorf("selection = %d..%d, want 6..11", after.SelStart, after.SelEnd)
	}
	if got := a11y.Selected(after.Node); got != "world" {
		t.Errorf("selected = %q, want %q", got, "world")
	}
}

func TestAXMultilineFieldReportsItsLines(t *testing.T) {
	w := TextInput(State("one\ntwo\nthree")).Multiline()
	p := NewProbe(w, Sz(300, 100))
	defer p.Close()
	field, ok := axDetailed(t, p).Find(RoleTextField, "")
	if !ok {
		t.Fatal("no text field")
	}
	if len(field.Runs) != 3 {
		t.Fatalf("%d runs, want three lines:\n%v", len(field.Runs), field.Runs)
	}
	for i, want := range []string{"one", "two", "three"} {
		loc, length, has := a11y.RangeForLine(field.Node, i)
		if !has {
			t.Fatalf("no line %d", i)
		}
		if got := a11y.StringForRange(field.Node, loc, length); got != want {
			t.Errorf("line %d = %q, want %q", i, got, want)
		}
	}
	// A byte on the second line is on line 1, and the lines go down the
	// screen in order.
	if got := a11y.LineForIndex(field.Node, 5); got != 1 {
		t.Errorf("line for offset 5 = %d, want 1", got)
	}
	if field.Runs[1].Rect.Origin.Y <= field.Runs[0].Rect.Origin.Y {
		t.Error("the second line is not below the first")
	}
	// The caret loaded at the end of the text, which is the last line.
	if got := a11y.InsertionLine(field.Node); got != 2 {
		t.Errorf("insertion line = %d, want 2", got)
	}
}

// TestAXPressRingsTheWidget is the other half of the round trip: the app
// carries out the press an assistive technology asked for, and the widget
// hears it. The bridge's half, where the element finds the node and asks,
// is TestAXPressReachesTheApp in the a11y package.
func TestAXPressRingsTheWidget(t *testing.T) {
	t.Parallel()
	w := &axBell{}
	w.Role = RoleButton
	w.SetName("ring")
	p := NewProbe(w, Sz(100, 100))
	defer p.Close()

	node, ok := p.Semantics().Find(RoleButton, "ring")
	if !ok {
		t.Fatal("the button is not in the published tree")
	}
	if !node.Actions.Has(ActionPress) {
		t.Fatal("the button does not offer a press")
	}
	p.Perform(node.ID, Action{Kind: ActionPress})
	if w.rung != 1 {
		t.Errorf("rung %d times, want 1", w.rung)
	}
}

// A tree is published once and never written again, though the next one
// shares every page of nodes that did not change with it: a reader holding
// an old tree across later frames sees exactly what it saw first.
func TestSemTreePublishedTreeNeverChanges(t *testing.T) {
	t.Parallel()
	names := make([]string, 1000)
	for i := range names {
		names[i] = fmt.Sprint("row ", i)
	}
	changed := State(0)
	w := FromFuncs(
		func(c Constraints, _ Env) Size { return c.Constrain(Sz(100, 100)) },
		func(dst *Canvas, r Rect) {
			at := changed.Get()
			dst.Node(r, Node{Role: RoleList}, func(dst *Canvas) {
				for i, name := range names {
					if i == at && at != 0 {
						name += " (changed)"
					}
					dst.Leaf(Rct(Pt(0, float64(i)), Sz(10, 1)), Node{Role: RoleListItem, Name: name})
				}
				if at%2 == 1 {
					dst.Leaf(r, Node{Role: RoleButton, Name: "extra"})
				}
			})
		})
	p := NewProbe(w, Sz(100, 100))
	defer p.Close()
	type held struct {
		tree  *SemTree
		text  string
		nodes []SemNode
	}
	hold := func() held {
		tr := p.Semantics()
		return held{tr, tr.String(), slices.Collect(func(yield func(SemNode) bool) {
			for _, n := range tr.All() {
				if !yield(n) {
					return
				}
			}
		})}
	}
	var trees []held
	trees = append(trees, hold())
	for _, at := range []int{500, 501, 3, 998, 0} {
		changed.Set(at)
		p.Frame()
		trees = append(trees, hold())
	}
	for k, h := range trees {
		if got := h.tree.String(); got != h.text {
			t.Fatalf("tree %d changed after publication", k)
		}
		for i := range h.tree.Len() {
			if !reflect.DeepEqual(h.tree.At(i), h.nodes[i]) {
				t.Fatalf("tree %d node %d changed after publication", k, i)
			}
		}
	}
	// One changed row shares the pages that hold none of the change.
	a, b := trees[0].tree, trees[1].tree
	if a == b || b.At(501).Name != "row 500 (changed)" || a.At(501).Name != "row 500" {
		t.Fatal("the changed row was not published")
	}
	if a.Ref(1) != b.Ref(1) || a.Ref(1000) != b.Ref(1000) {
		t.Error("pages without a change were copied instead of shared")
	}
	// An added node changes its parent's children, which are on the first page.
	if c := trees[2].tree; c.Ref(0) == b.Ref(0) || len(c.At(0).Children) != 1001 || len(b.At(0).Children) != 1000 {
		t.Error("a parent whose children changed kept its old page")
	}
}
