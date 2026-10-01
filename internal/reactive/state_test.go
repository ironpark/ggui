package reactive

import (
	"slices"
	"testing"
	"time"
)

// counter observes r and counts the runs, for a test asking whether a write
// reached a reader. It is disposed when the test ends.
func counter[T any](t *testing.T, r Readable[T]) *int {
	t.Helper()
	runs := 0
	dispose := Observe(func() { r.Get(); runs++ })
	t.Cleanup(dispose)
	runs = 0
	return &runs
}

func TestEqualWriteNotifiesNoOne(t *testing.T) {
	t.Parallel()
	s := State(3)
	runs := counter(t, s)
	state, layout := StateGen(), LayoutGen()
	s.Set(3)
	Flush()
	if *runs != 0 || StateGen() != state || LayoutGen() != layout {
		t.Fatalf("an equal write ran the reader %d times and moved the generations", *runs)
	}
	s.Set(4)
	Flush()
	if *runs != 1 || StateGen() == state || LayoutGen() == layout {
		t.Fatalf("a new value ran the reader %d times, want 1, and must advance both generations", *runs)
	}
}

// point declares its own equality, which Set prefers to ==.
type point struct {
	x, y int
	tags []string // makes point incomparable
}

func (p point) Equal(q point) bool { return p.x == q.x && p.y == q.y }

// ref is a pointer type declaring equality, to exercise the nil receiver.
type ref struct{ id int }

func (n *ref) Equal(o *ref) bool { return o != nil && n.id == o.id }

// shape is an interface that declares equality over itself.
type shape interface{ Equal(shape) bool }

type square int

func (s square) Equal(o shape) bool { q, ok := o.(square); return ok && q == s }

func TestSetDropsWritesTheTypeCallsEqual(t *testing.T) {
	t.Parallel()
	p := State(point{1, 2, nil})
	pRuns := counter(t, p)
	p.Set(point{1, 2, []string{"ignored by Equal"}})
	Flush()
	if *pRuns != 0 {
		t.Fatal("a write its type's Equal accepts notified the reader")
	}

	at := time.Unix(100, 0)
	when := State(at.UTC())
	whenRuns := counter(t, when)
	when.Set(at.In(time.FixedZone("X", 3600)))
	Flush()
	if *whenRuns != 0 {
		t.Fatal("the same instant in another zone notified the reader")
	}

	n := State[*ref](nil)
	nRuns := counter(t, n)
	n.Set(nil)
	Flush()
	if *nRuns != 0 {
		t.Fatal("writing nil over nil notified the reader")
	}
	n.Set(&ref{1})
	n.Set(&ref{1})
	Flush()
	if *nRuns != 1 {
		t.Fatalf("nil then two equal nodes ran the reader %d times, want 1", *nRuns)
	}

	sh := State[shape](nil)
	shRuns := counter(t, sh)
	sh.Set(nil)
	sh.Set(square(2))
	sh.Set(square(2))
	Flush()
	if *shRuns != 1 {
		t.Fatalf("an interface with Equal ran its reader %d times for nil, 2, 2; want 1", *shRuns)
	}
}

func TestIncomparableAndInterfaceValuesNotifyEveryWrite(t *testing.T) {
	t.Parallel()
	list := State([]int{1})
	listRuns := counter(t, list)
	list.Set([]int{1})
	Flush()
	if *listRuns != 1 {
		t.Fatalf("an equal slice ran the reader %d times, want 1: slices have no equality", *listRuns)
	}

	// == on an interface panics for a dynamic value that is not comparable,
	// so an interface-typed state never compares.
	v := State[any](1)
	vRuns := counter(t, v)
	v.Set(1)
	v.Set([]int{1})
	v.Set([]int{1})
	Flush()
	if *vRuns != 1 || Untrack(v.Get) == nil {
		t.Fatalf("interface writes ran the reader %d times, want once per flush", *vRuns)
	}
}

func TestWithEqualReplacesTheRedundantWriteTest(t *testing.T) {
	t.Parallel()
	near := func(a, b float64) bool { return a-b < 0.5 && b-a < 0.5 }
	s := State(1.0).WithEqual(near)
	runs := counter(t, s)
	s.Set(1.2)
	Flush()
	if *runs != 0 || Untrack(s.Get) != 1.0 {
		t.Fatalf("a write within tolerance ran %d times and stored %v", *runs, Untrack(s.Get))
	}
	s.WithEqual(nil)
	s.Set(1.0)
	Flush()
	if *runs != 1 {
		t.Fatalf("with no equality an equal write ran the reader %d times, want 1", *runs)
	}
}

func TestUpdateHelpersWriteThroughWritable(t *testing.T) {
	t.Parallel()
	on := State(false)
	Toggle(on)
	if !Untrack(on.Get) {
		t.Fatal("Toggle left false")
	}
	n := State(1.5)
	Add(n, 2)
	if Untrack(n.Get) != 3.5 {
		t.Fatalf("Add gave %v, want 3.5", Untrack(n.Get))
	}
	s := State("a")
	s.Update(func(v string) string { return v + "b" })
	if Untrack(s.Get) != "ab" {
		t.Fatalf("Update gave %q, want %q", Untrack(s.Get), "ab")
	}
}

// Append copies, so the old value a reader holds keeps its contents even
// when its backing array had room for the new items.
func TestAppendAndRemoveMakeNewSlices(t *testing.T) {
	t.Parallel()
	backing := make([]int, 2, 8)
	old := backing[:2]
	items := State(old)
	Append(items, 7, 8)
	if got := Untrack(items.Get); !slices.Equal(got, []int{0, 0, 7, 8}) {
		t.Fatalf("Append gave %v, want [0 0 7 8]", got)
	}
	if backing[:4][2] != 0 {
		t.Fatal("Append wrote into the old value's backing array")
	}

	cur := Untrack(items.Get)
	gen := StateGen()
	Remove(items, func(v int) bool { return v > 100 })
	if StateGen() != gen {
		t.Fatal("Remove matching nothing wrote the state")
	}
	Remove(items, func(v int) bool { return v == 0 })
	if got := Untrack(items.Get); !slices.Equal(got, []int{7, 8}) {
		t.Fatalf("Remove gave %v, want [7 8]", got)
	}
	if !slices.Equal(cur, []int{0, 0, 7, 8}) {
		t.Fatalf("Remove changed the old value to %v", cur)
	}
}

type form struct {
	Name string
	Age  int
}

func TestLensAndFieldReadAndWriteOnePart(t *testing.T) {
	t.Parallel()
	f := State(form{Name: "ann", Age: 30})
	name := f.Lens(func(v form) string { return v.Name }, func(v form, n string) form { v.Name = n; return v })
	age := f.Field(func(v *form) *int { return &v.Age })

	var seen []string
	dispose := Observe(func() { seen = append(seen, name.Get()) })
	defer dispose()

	name.Set("bob")
	age.Update(func(a int) int { return a + 1 })
	Flush()
	if got := Untrack(f.Get); got != (form{"bob", 31}) {
		t.Fatalf("the whole is %+v, want {bob 31}", got)
	}
	if !slices.Equal(seen, []string{"ann", "bob"}) {
		t.Fatalf("a reader of the lens saw %v, want [ann bob]", seen)
	}
	name.Update(func(n string) string { return n + "!" })
	age.Set(5)
	if got := Untrack(f.Get); got != (form{"bob!", 5}) {
		t.Fatalf("after Update and Set the whole is %+v, want {bob! 5}", got)
	}
	if name.GetAny() != "bob!" || age.GetAny() != 5 {
		t.Fatalf("GetAny read %v and %v", name.GetAny(), age.GetAny())
	}
	var _ Writable[int] = age
}

func TestGetAnyAndConstReadTheValue(t *testing.T) {
	t.Parallel()
	s := State(2)
	d := s.Map(func(v int) string { return string(rune('a' + v)) })
	defer d.Dispose()
	c := Const([]int{1, 2})
	for name, tc := range map[string]struct {
		r    AnyReader
		want any
	}{
		"state":   {s, 2},
		"derived": {d, "c"},
	} {
		if got := tc.r.GetAny(); got != tc.want {
			t.Errorf("%s: GetAny = %v, want %v", name, got, tc.want)
		}
	}
	if got := c.Get(); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("Const.Get = %v, want [1 2]", got)
	}
	if got, ok := c.(AnyReader); !ok || !slices.Equal(got.GetAny().([]int), []int{1, 2}) {
		t.Fatal("Const is not an AnyReader of its value")
	}
}
