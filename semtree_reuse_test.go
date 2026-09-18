package ggui

import (
	"math/rand/v2"
	"reflect"
	"testing"
)

func TestSemTreeReusesEqualValuesAndFreezesDescriptions(t *testing.T) {
	expanded := false
	runs := []TextRun{{Start: 0, End: 2, Rect: Rct(Pt(1, 2), Sz(20, 10)), Stops: []TextStop{{0, 0}, {2, 20}}}}
	n := Node{Role: RoleTextField, Name: "editor", Expanded: &expanded, Runs: runs}
	c := &Canvas{}
	c.Leaf(Rct(Pt(0, 0), Sz(100, 20)), n)
	first := buildSemTree(c, nil, nil)
	// Equal contents in fresh buffers must compare by value.
	c.sem[0].node = freezeNode(n)
	if next := buildSemTree(c, nil, first); next != first {
		t.Fatal("equal description allocated a new snapshot")
	}
	// A custom widget may reuse the buffers supplied in the previous frame.
	expanded = true
	runs[0].End = 3
	runs[0].Stops[1].X = 30
	c.sem[0].node = n
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
// remember to add a new test case beside the production comparison.
func semanticFieldChanges(v reflect.Value, visit func()) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			semanticFieldChanges(v.Field(i), visit)
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
	rng := rand.New(rand.NewPCG(1, 2))
	for range 1000 {
		a := Node{Role: RoleTextField, Name: "a", Value: "xy", Expanded: Expandable(rng.IntN(2) == 0),
			SelStart: rng.IntN(3), SelEnd: rng.IntN(3),
			Runs: []TextRun{{Start: rng.IntN(3), End: rng.IntN(3), Stops: []TextStop{{rng.IntN(3), rng.Float64()}}}}}
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
		{"clipped rect", func(c *Canvas) { c.sem[1].rect.Size.W-- }},
		{"full rect", func(c *Canvas) { c.sem[1].full.Origin.Y++ }},
		{"identity", func(c *Canvas) { c.sem[1].id = "new" }},
		{"uncomparable identity", func(c *Canvas) { c.sem[1].id = []int{1} }},
		{"parent", func(c *Canvas) { c.sem[1].parent = 0 }},
		{"removed", func(c *Canvas) { c.sem = c.sem[:1] }},
	} {
		t.Run(test.name, func(t *testing.T) {
			dst := *c
			dst.sem = append([]semNode(nil), c.sem...)
			test.edit(&dst)
			if buildSemTree(&dst, nil, base) == base {
				t.Fatal("changed frame reused stale snapshot")
			}
		})
	}
	focus := &hitRegion{key: w, rect: r}
	next := buildSemTree(c, focus, base)
	if next == base || next.focused != 1 || buildSemTree(c, nil, next) == next {
		t.Fatal("focus changes were not published")
	}
	empty := &Canvas{}
	first := buildSemTree(empty, nil, next)
	if first.Len() != 0 || buildSemTree(empty, nil, first) != first {
		t.Fatal("empty snapshot was not reusable")
	}
}

func TestSemTreeOldSnapshotCanBeReadDuringBufferReuse(t *testing.T) {
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
