package virtual

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
)

func TestFixedReachesTheRowsTheWindowOverlaps(t *testing.T) {
	for _, tt := range []struct {
		n                 int
		pitch, at, extent float64
		first, last       int
	}{
		{100, 20, 0, 100, 0, 5},
		{100, 20, 10, 100, 0, 6},      // half a row either end
		{100, 20, 1990, 100, 99, 100}, // past the end
		{100, 20, -50, 100, 0, 3},     // before the start
		{0, 20, 0, 100, 0, 0},
		{10, 0, 0, 100, 0, 10}, // no pitch: every row
	} {
		first, last := Fixed(tt.n, tt.pitch, tt.at, tt.extent)
		if first != tt.first || last != tt.last {
			t.Errorf("Fixed(%d, %v, %v, %v) = %d, %d; want %d, %d", tt.n, tt.pitch, tt.at, tt.extent, first, last, tt.first, tt.last)
		}
	}
}

func TestPlaceUsesMeasuredSizesAndTheEstimateForTheRest(t *testing.T) {
	var e Estimate[string]
	keys := []string{"a", "b", "c", "d"}
	e.Measure("b", 50)
	e.Measure("d", 10)
	e.Place(keys, 30, 2)
	want := []float64{0, 32, 84, 116, 128}
	for i, w := range want {
		if got, ok := e.Start(i); !ok || got != w {
			t.Fatalf("Start(%d) = %v, %v; want %v", i, got, ok, w)
		}
	}
	if _, ok := e.Start(len(want)); ok {
		t.Fatal("Start past the end was found")
	}
	if _, ok := e.Start(-1); ok {
		t.Fatal("Start(-1) was found")
	}
	if got := e.Total(); got != 126 {
		t.Fatalf("Total = %v, want 126", got)
	}
}

func TestRowAtFindsTheRowReachingPastV(t *testing.T) {
	var e Estimate[int]
	e.Place([]int{0, 1, 2}, 10, 0)
	for v, want := range map[float64]int{0: 0, 9.9: 0, 10: 1, 25: 2, 30: 3, 100: 3, -5: 0} {
		if got := e.RowAt(v); got != want {
			t.Errorf("RowAt(%v) = %d, want %d", v, got, want)
		}
	}
}

func TestAnchorMovesWithTheRowsAboveIt(t *testing.T) {
	var e Estimate[int]
	keys := []int{1, 2, 3, 4, 5}
	e.Place(keys, 20, 0)
	e.Anchor(keys, 45) // row 2, key 3, at 40
	if got := e.Moved(keys); got != 0 {
		t.Fatalf("Moved before any change = %v", got)
	}
	e.Measure(1, 50) // 30 more than estimated, above the anchor
	e.Measure(4, 90) // below it: no move
	e.Place(keys, 20, 0)
	if got := e.Moved(keys); got != 30 {
		t.Fatalf("Moved = %v, want 30", got)
	}
	// A row coming in above moves it by as much again.
	keys = append([]int{0}, keys...)
	e.Place(keys, 20, 0)
	if got := e.TakeMoved(keys); got != 50 {
		t.Fatalf("TakeMoved after an insert above = %v, want 50", got)
	}
	if got := e.Moved(keys); got != 0 {
		t.Fatalf("Moved after TakeMoved = %v", got)
	}
	// The anchor gone: nothing to measure from.
	e.Anchor(keys, 45)
	e.Place([]int{4, 5}, 20, 0)
	if got := e.Moved([]int{4, 5}); got != 0 {
		t.Fatalf("Moved with the anchor gone = %v", got)
	}
	e.Place(keys, 20, 0)
	// Past the end there is no row to anchor.
	e.Anchor(keys, 1e9)
	if got := e.Moved(keys); got != 0 {
		t.Fatalf("Moved anchored past the end = %v", got)
	}
}

func TestPruneForgetsRowsThatLeft(t *testing.T) {
	var e Estimate[int]
	for k := range 10 {
		e.Measure(k, 100)
	}
	e.Prune([]int{2, 3})
	if len(e.measured) != 2 {
		t.Fatalf("%d sizes kept, want 2", len(e.measured))
	}
	e.Place([]int{2, 5}, 10, 0)
	if got, _ := e.Start(1); got != 100 {
		t.Fatalf("row 2 kept no size: row 5 starts at %v", got)
	}
	if got := e.Total(); got != 110 {
		t.Fatalf("row 5 kept its size: Total %v, want 110", got)
	}
}

// However rows are measured, the starts climb by each row's size and gap,
// RowAt agrees with them, and the anchor moves by exactly the change in
// size of the rows above it.
func FuzzEstimate(f *testing.F) {
	f.Add(uint64(1), 20, 3.0)
	f.Add(uint64(7), 200, 0.0)
	f.Fuzz(func(t *testing.T, seed uint64, n int, gap float64) {
		if n < 1 || n > 500 || gap < 0 || gap > 100 {
			return
		}
		r := rand.New(rand.NewPCG(seed, 0))
		keys := make([]int, n)
		for i := range keys {
			keys[i] = i * 3
		}
		var e Estimate[int]
		e.Place(keys, 25, gap)
		a := r.IntN(n)
		at, _ := e.Start(a)
		e.Anchor(keys, at)
		above := 0.0
		for i, k := range keys {
			if r.IntN(2) == 0 {
				size := float64(r.IntN(100))
				e.Measure(k, size)
				if i < a {
					above += size - 25
				}
			}
		}
		e.Place(keys, 25, gap)
		for i := range n {
			s0, _ := e.Start(i)
			s1, _ := e.Start(i + 1)
			if s1 < s0+gap {
				t.Fatalf("row %d starts at %v, the next at %v, with gap %v", i, s0, s1, gap)
			}
			if s1 > s0 {
				if got := e.RowAt(s0); got != i {
					t.Fatalf("RowAt(%v) = %d, want %d", s0, got, i)
				}
			}
		}
		if got := e.Moved(keys); math.Abs(got-above) > 1e-9*max(1, math.Abs(above)) {
			t.Fatalf("Moved = %v, want %v", got, above)
		}
	})
}

func BenchmarkFixed(b *testing.B) {
	for b.Loop() {
		Fixed(100_000, 24, 1_234_567, 800)
	}
}

// A layout of a long list as EstimatedExtent makes it: placing every row,
// finding the window's first by where its anchor moved to, measuring the
// rows in view, placing again only if one changed, and anchoring anew.
func BenchmarkEstimateLayout(b *testing.B) {
	for _, n := range []int{1000, 100_000} {
		for _, resize := range []bool{false, true} {
			b.Run(fmt.Sprintf("%d/resize=%v", n, resize), func(b *testing.B) {
				keys := make([]int, n)
				var e Estimate[int]
				size := func(k int) float64 { return float64(20 + k%40) }
				for i := range keys {
					keys[i] = i
					if i%3 == 0 {
						e.Measure(i, size(i))
					}
				}
				e.Place(keys, 30, 4)
				e.Anchor(keys, e.Total()/2)
				b.ReportAllocs()
				grow := 0.0
				for b.Loop() {
					e.Place(keys, 30, 4)
					first := e.RowAt(e.Total()/2 + e.Moved(keys))
					changed := false
					if resize {
						grow++
					}
					for i := first; i < min(first+30, n); i++ {
						changed = e.Measure(keys[i], size(i)+grow) || changed
					}
					if changed {
						e.Place(keys, 30, 4)
					}
					at, _ := e.Start(first)
					e.TakeMoved(keys)
					e.Anchor(keys, at)
				}
			})
		}
	}
}

func BenchmarkEstimatePrune(b *testing.B) {
	keys := make([]int, 10_000)
	for i := range keys {
		keys[i] = i
	}
	b.ReportAllocs()
	for b.Loop() {
		var e Estimate[int]
		for k := range 12_000 {
			e.Measure(k, 1)
		}
		e.Prune(keys)
	}
}

// A Place after rows were measured by index works out only the rows from
// the first one changed, and lands every start where placing afresh does;
// the anchor moves by the change in size of the rows above it all the
// same. A Place with nothing changed keeps every start.
func TestPlaceAfterMeasureAtMatchesAFreshPlace(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 0))
	keys := make([]int, 500)
	for i := range keys {
		keys[i] = i * 7
	}
	var e Estimate[int]
	e.Place(keys, 25, 2)
	sizes := make([]float64, len(keys)) // each row's size, as the test keeps it
	for i := range sizes {
		sizes[i] = 25
	}
	for round := range 40 {
		a := r.IntN(len(keys))
		at, _ := e.Start(a)
		e.Anchor(keys, at)
		before := 0.0
		for _, size := range sizes[:a] {
			before += size
		}
		for range 1 + r.IntN(10) {
			i := r.IntN(len(keys))
			sizes[i] = float64(r.IntN(80))
			e.MeasureAt(i, keys[i], sizes[i])
		}
		above := -before
		for _, size := range sizes[:a] {
			above += size
		}
		e.Place(keys, 25, 2)
		if round%3 == 0 {
			e.Place(keys, 25, 2) // nothing changed: every start kept
		}
		var fresh Estimate[int]
		for k, size := range e.measured {
			fresh.Measure(k, size)
		}
		fresh.Place(keys, 25, 2)
		for i := range len(keys) + 1 {
			got, _ := e.Start(i)
			want, _ := fresh.Start(i)
			if got != want {
				t.Fatalf("round %d: row %d starts at %v, a fresh Place puts it at %v", round, i, got, want)
			}
		}
		if got := e.TakeMoved(keys); math.Abs(got-above) > 1e-9 {
			t.Fatalf("round %d: the anchor moved %v, want %v", round, got, above)
		}
	}
	// Keys changed in place are placed afresh once pruned.
	keys[0], keys[1] = keys[1], keys[0]
	e.Prune(keys)
	e.Place(keys, 25, 2)
	if got, _ := e.Start(1); got != sizes[1]+2 {
		t.Fatalf("row 1 starts at %v after the keys changed, want %v", got, sizes[1]+2)
	}
	e.Measure(keys[0], 1000)
	e.Place(keys, 25, 2)
	if got, _ := e.Start(1); got != 1002 {
		t.Fatalf("row 1 starts at %v after the keys changed, want 1002", got)
	}
}

// A layout that only scrolled places nothing again.
func BenchmarkEstimatePlaceUnchanged(b *testing.B) {
	keys := make([]int, 100_000)
	for i := range keys {
		keys[i] = i
	}
	var e Estimate[int]
	e.Place(keys, 30, 4)
	b.ReportAllocs()
	for b.Loop() {
		e.Place(keys, 30, 4)
	}
}
