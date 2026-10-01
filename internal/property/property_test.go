package property

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/reactive"
)

// read is one layout read an Owner reported: which source, at which version.
type read struct {
	src     reactive.LayoutSource
	version uint64
}

// measure runs fn under a layout recorder and returns the reads it reported.
func measure(fn func()) []read {
	var reads []read
	reactive.Measure(func(src reactive.LayoutSource, v uint64) { reads = append(reads, read{src, v}) }, fn)
	return reads
}

func TestOwnerReadRecordsItsRevision(t *testing.T) {
	t.Parallel()
	var o Owner
	o.Changed()
	o.Changed()
	if o.Version() != 2 || o.LayoutVersion() != 2 {
		t.Fatalf("after two changes Version = %d, LayoutVersion = %d, want 2", o.Version(), o.LayoutVersion())
	}
	reads := measure(o.Read)
	if len(reads) != 1 || reads[0].src != &o || reads[0].version != 2 {
		t.Fatalf("Read reported %v, want one read of the owner at version 2", reads)
	}
	o.Read() // no recorder installed: nothing to report to, and no panic
}

// An owner no layout has read yet has nothing on screen to move, so its
// changes schedule no frame; a read one does, unless the change is made
// while laying out, where the revision alone dirties the measurement.
func TestOwnerChangedSchedulesLayoutOnlyOnceMountedAndOutsideLayout(t *testing.T) {
	t.Parallel()
	var o Owner
	gen := reactive.LayoutGen()
	o.Changed()
	if reactive.LayoutGen() != gen {
		t.Fatal("a change to an owner no layout read scheduled a layout")
	}
	measure(o.Read)
	o.Changed()
	if reactive.LayoutGen() == gen {
		t.Fatal("a change to a mounted owner did not schedule a layout")
	}
	gen = reactive.LayoutGen()
	done := EnterLayout()
	o.Changed()
	done()
	if reactive.LayoutGen() != gen {
		t.Fatal("a change made during layout scheduled another layout")
	}
	if o.Version() != 3 {
		t.Fatalf("Version = %d after three changes, want 3", o.Version())
	}
}

// Layout records the revision on entry and again on leaving, so that a
// parent configuring its child between the two leaves the later revision
// in the measurement's dependencies.
func TestOwnerLayoutRecordsInitialAndFinalRevision(t *testing.T) {
	t.Parallel()
	var o Owner
	var inside bool
	reads := measure(func() {
		done := o.Layout()
		inside = reactive.InLayout()
		o.Changed()
		done()
	})
	if !inside {
		t.Fatal("Layout did not mark the goroutine as laying out")
	}
	if reactive.InLayout() {
		t.Fatal("the goroutine is still laying out after Layout's done")
	}
	if len(reads) != 2 || reads[0].version != 0 || reads[1].version != 1 {
		t.Fatalf("Layout reported %v, want reads at version 0 then 1", reads)
	}
}

func TestEqualComparesConfigurationValues(t *testing.T) {
	t.Parallel()
	nan := math.NaN()
	type style struct {
		Name string
		Tags []string
	}
	shared := &style{}
	for _, tc := range []struct {
		name string
		eq   bool
		want bool
	}{
		{"same ints", Equal(3, 3), true},
		{"different ints", Equal(3, 4), false},
		{"NaN and NaN", Equal(nan, nan), true},
		{"NaN and zero", Equal(nan, 0.0), false},
		{"float32 NaNs", Equal(float32(nan), float32(nan)), true},
		{"float32 values", Equal(float32(1), float32(2)), false},
		{"zero and negative zero", Equal(0.0, math.Copysign(0, -1)), true},
		{"RGBA and NRGBA rendering alike", Equal[color.Color](color.RGBA{255, 0, 0, 255}, color.NRGBA{255, 0, 0, 255}), true},
		{"translucent colors of different premultiplication", Equal[color.Color](color.RGBA{128, 0, 0, 128}, color.NRGBA{128, 0, 0, 128}), false},
		{"Gray and RGBA", Equal[color.Color](color.Gray{0x80}, color.RGBA{0x80, 0x80, 0x80, 0xff}), true},
		{"color and nil", Equal[color.Color](color.Black, nil), false},
		{"nil interfaces", Equal[any](nil, nil), true},
		{"structs holding slices", Equal(style{"a", []string{"x"}}, style{"a", []string{"x"}}), true},
		{"structs with different slices", Equal(style{"a", []string{"x"}}, style{"a", []string{"y"}}), false},
		{"same pointer", Equal(shared, shared), true},
		{"distinct pointers to equal values", Equal(&style{}, &style{}), true},
		{"slices", Equal([]int{1, 2}, []int{1, 2}), true},
		{"float and int in any", Equal[any](1.0, 1), false},
	} {
		if tc.eq != tc.want {
			t.Errorf("%s: Equal = %v, want %v", tc.name, tc.eq, tc.want)
		}
	}
}

func TestWatchReportsOnlyAChangedResult(t *testing.T) {
	t.Parallel()
	var o Owner
	gap := 4.0
	func() { defer Watch(&o, &gap)(); gap = 4 }()
	if o.Version() != 0 {
		t.Fatal("rewriting an equal value counted as a change")
	}
	func() { defer Watch(&o, &gap)(); gap += 2 }()
	if o.Version() != 1 || gap != 6 {
		t.Fatalf("a compound assignment gave Version %d gap %v, want 1 and 6", o.Version(), gap)
	}
	func() { defer Watch(&o, &gap)(); gap = math.NaN() }()
	func() { defer Watch(&o, &gap)(); gap = math.NaN() }()
	if o.Version() != 2 {
		t.Fatalf("Version = %d after setting NaN twice, want 2: a repeated NaN is no change", o.Version())
	}
}

// requirePanic returns the message Require panicked with, or "" if it did not.
func requirePanic(v any, name string) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg, _ = r.(string)
			if msg == "" {
				msg = "non-string panic"
			}
		}
	}()
	Require(v, name)
	return ""
}

func TestRequireRejectsNilReaders(t *testing.T) {
	t.Parallel()
	var nilPtr *Value[int]
	var nilFunc func() int
	var nilMap map[string]int
	var nilChan chan int
	var nilSlice []int
	for name, v := range map[string]any{
		"untyped nil": nil,
		"nil pointer": nilPtr,
		"nil func":    nilFunc,
		"nil map":     nilMap,
		"nil chan":    nilChan,
		"nil slice":   nilSlice,
	} {
		msg := requirePanic(v, "Text.Bind")
		if !strings.Contains(msg, "Text.Bind") || !strings.Contains(msg, "non-nil") {
			t.Errorf("%s: Require panicked with %q, want a message naming Text.Bind and non-nil", name, msg)
		}
	}
	for name, v := range map[string]any{
		"pointer": &Value[int]{},
		"func":    func() int { return 0 },
		"struct":  reactive.Const(1),
		"int":     0,
	} {
		if msg := requirePanic(v, "x"); msg != "" {
			t.Errorf("%s: Require panicked with %q on a usable reader", name, msg)
		}
	}
}

func TestSameIsIdentityForComparableValuesOnly(t *testing.T) {
	t.Parallel()
	p, q := &Value[int]{}, &Value[int]{}
	for _, tc := range []struct {
		name string
		same bool
		want bool
	}{
		{"nil and nil", Same(nil, nil), true},
		{"nil and value", Same(nil, 1), false},
		{"value and nil", Same(1, nil), false},
		{"same pointer", Same(p, p), true},
		{"different pointers", Same(p, q), false},
		{"equal ints", Same(1, 1), true},
		{"different types", Same(1, int64(1)), false},
		{"slices", Same([]int{1}, []int{1}), false},
		{"maps", Same(map[int]int{}, map[int]int{}), false},
	} {
		if tc.same != tc.want {
			t.Errorf("%s: Same = %v, want %v", tc.name, tc.same, tc.want)
		}
	}
}

func TestValueHoldsALiteralOrABorrowedReader(t *testing.T) {
	t.Parallel()
	var v Value[string]
	if v.Get() != "" || v.Bound() {
		t.Fatal("the zero Value is not an unbound empty literal")
	}
	if !v.Set("a") {
		t.Fatal("setting a new literal reported no change")
	}
	if v.Set("a") {
		t.Fatal("setting an equal literal reported a change")
	}

	s := reactive.State("from state")
	if !v.Bind(s, "Text") {
		t.Fatal("binding a reader reported no change")
	}
	if v.Bind(s, "Text") {
		t.Fatal("binding the same reader again reported a change")
	}
	if !v.Bound() {
		t.Fatal("a bound Value does not report Bound")
	}
	if v.Current() != "a" {
		t.Fatalf("Current before any Get = %q, want the last literal", v.Current())
	}
	if got := v.Get(); got != "from state" {
		t.Fatalf("Get = %q, want the reader's value", got)
	}
	s.Set("changed")
	if v.Current() != "from state" {
		t.Fatalf("Current = %q, want the value the last Get resolved", v.Current())
	}
	if got := v.Get(); got != "changed" {
		t.Fatalf("Get after the reader changed = %q, want %q", got, "changed")
	}
	if !v.Bind(reactive.Const("other"), "Text") || v.Get() != "other" {
		t.Fatal("binding a different reader did not replace the first")
	}
	// Unbinding is a change even when the literal equals the resolved value.
	if !v.Set("other") {
		t.Fatal("replacing a reader with an equal literal reported no change")
	}
	if v.Bound() || v.Get() != "other" {
		t.Fatal("Set did not unbind the reader")
	}
	s.Set("ignored")
	if v.Get() != "other" {
		t.Fatal("an unbound Value still follows its old reader")
	}
}

// Resolving a bound Value reads its reader in the caller's tracking scope,
// so an effect resolving it re-runs when the reader changes.
func TestValueGetSubscribesTheRunningEffect(t *testing.T) {
	t.Parallel()
	s := reactive.State(1)
	var v Value[int]
	v.Bind(s, "Width")
	runs, seen := 0, 0
	rt := reactive.NewRuntime()
	dispose := rt.Root(func() { reactive.Observe(func() { runs++; seen = v.Get() }) })
	defer dispose()
	s.Set(2)
	rt.Flush()
	if runs != 2 || seen != 2 {
		t.Fatalf("the effect ran %d times and saw %d, want 2 runs seeing 2", runs, seen)
	}
}

func TestValueBindNilPanics(t *testing.T) {
	t.Parallel()
	var v Value[int]
	defer func() {
		if msg, _ := recover().(string); !strings.Contains(msg, "Width") {
			t.Fatalf("Bind(nil) panicked with %q, want a message naming the property", msg)
		}
	}()
	v.Bind(nil, "Width")
}

// FuzzEqualFloat checks Equal on float64 against its definition: it is
// reflexive even for NaN, symmetric, and otherwise agrees with ==, so that
// no payload of NaN reads as a change of configuration.
func FuzzEqualFloat(f *testing.F) {
	for _, v := range []float64{0, math.Copysign(0, -1), 1, math.Inf(1), math.Inf(-1), math.NaN()} {
		f.Add(math.Float64bits(v), math.Float64bits(v))
	}
	f.Add(math.Float64bits(math.NaN()), uint64(0x7ff0000000000001)) // two different NaN payloads
	f.Add(uint64(1), uint64(2))
	f.Fuzz(func(t *testing.T, abits, bbits uint64) {
		a, b := math.Float64frombits(abits), math.Float64frombits(bbits)
		got := Equal(a, b)
		want := a == b || math.IsNaN(a) && math.IsNaN(b)
		if got != want {
			t.Fatalf("Equal(%v, %v) = %v, want %v", a, b, got, want)
		}
		if Equal(b, a) != got {
			t.Fatalf("Equal is not symmetric for %v and %v", a, b)
		}
		if !Equal(a, a) {
			t.Fatalf("Equal(%v, %v) is false for the same value", a, a)
		}
		if f32 := float32(a); !Equal(f32, f32) {
			t.Fatalf("Equal is false for the float32 %v and itself", f32)
		}
	})
}
