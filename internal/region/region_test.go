package region

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/ironpark/ggui/pkg/layout"
)

var (
	pt  = layout.Pt[float64]
	sz  = layout.Sz[float64]
	rct = layout.Rct
)

func TestCoverLeavesEmptyRectsOut(t *testing.T) {
	a := rct(pt(10, 10), sz(5, 5))
	if got := Cover(Rect{}, a); got != a {
		t.Fatalf("Cover(empty, a) = %v", got)
	}
	if got := Cover(a, rct(pt(-100, -100), sz(0, 9))); got != a {
		t.Fatalf("Cover(a, empty) = %v: an empty Rect far off widened it", got)
	}
	if got := CoverAll(nil); !got.Empty() {
		t.Fatalf("CoverAll(nil) = %v", got)
	}
	want := rct(pt(0, 0), sz(15, 15))
	if got := CoverAll([]Rect{{}, rct(pt(0, 0), sz(1, 1)), a}); got != want {
		t.Fatalf("CoverAll = %v, want %v", got, want)
	}
}

func TestGrowAndOverlaps(t *testing.T) {
	r := rct(pt(10, 10), sz(10, 10))
	if got := Grow(r, 2); got != rct(pt(8, 8), sz(14, 14)) {
		t.Fatalf("Grow = %v", got)
	}
	touching := rct(pt(20, 10), sz(5, 5))
	if Overlaps(r, touching) {
		t.Fatal("rects that only touch overlap")
	}
	if !Overlaps(Grow(r, 1), touching) {
		t.Fatal("a grown rect misses its neighbour")
	}
}

func TestMergeJoinsNearRegionsAndCutsToTheWindow(t *testing.T) {
	window := rct(pt(0, 0), sz(1000, 1000))
	in := []Rect{
		rct(pt(10, 10), sz(10, 10)), rct(pt(22, 10), sz(10, 10)), // close: one
		rct(pt(500, 500), sz(10, 10)), // apart
		rct(pt(-50, -50), sz(10, 10)), // off the window
	}
	keep := append([]Rect(nil), in...)
	got := Merge(in, window, 4, 6)
	if len(got) != 2 {
		t.Fatalf("regions %v; want the two near ones as one and the far one", got)
	}
	if fmt.Sprint(in) != fmt.Sprint(keep) {
		t.Fatal("Merge changed its input")
	}
	var many []Rect
	for i := range 7 {
		many = append(many, rct(pt(float64(i)*100, 0), sz(10, 10)))
	}
	if got := Merge(many, window, 4, 6); len(got) != 1 {
		t.Fatalf("%d regions for 7 apart; want one around them all", len(got))
	}
}

// A chain of regions each within slack of the next, given in an order that
// merges them only one pass at a time, still ends as one.
func TestMergeFollowsAChain(t *testing.T) {
	window := rct(pt(0, 0), sz(1000, 1000))
	var chain []Rect
	for _, i := range []int{0, 4, 2, 6, 1, 5, 3} {
		chain = append(chain, rct(pt(float64(i)*12, 0), sz(10, 10)))
	}
	if got := Merge(chain, window, 4, 100); len(got) != 1 {
		t.Fatalf("chain merged to %v", got)
	}
}

// Whatever comes in, what comes out is inside the window, covers every
// part of the input inside it, and no two regions are within slack.
func FuzzMerge(f *testing.F) {
	f.Add(uint64(1), 10)
	f.Add(uint64(9), 40)
	f.Fuzz(func(t *testing.T, seed uint64, n int) {
		if n < 0 || n > 64 {
			return
		}
		r := rand.New(rand.NewPCG(seed, 0))
		window := rct(pt(0, 0), sz(400, 300))
		in := make([]Rect, n)
		for i := range in {
			in[i] = rct(pt(r.Float64()*500-50, r.Float64()*400-50), sz(r.Float64()*40, r.Float64()*40))
		}
		const slack, most = 4, 6
		out := Merge(in, window, slack, most)
		if len(out) > most {
			t.Fatalf("%d regions, most %d", len(out), most)
		}
		for i, o := range out {
			if o.Intersect(window) != o {
				t.Fatalf("region %v leaves the window", o)
			}
			for _, p := range out[i+1:] {
				if len(out) > 1 && Overlaps(Grow(o, slack), p) {
					t.Fatalf("regions %v and %v were left apart", o, p)
				}
			}
		}
		for _, x := range in {
			x = x.Intersect(window)
			if x.Empty() {
				continue
			}
			covered := false
			for _, o := range out {
				if o.Intersect(x) == x {
					covered = true
				}
			}
			if !covered {
				t.Fatalf("%v is in no region of %v", x, out)
			}
		}
	})
}

func BenchmarkMerge(b *testing.B) {
	window := rct(pt(0, 0), sz(1920, 1080))
	for _, n := range []int{4, 32, 128} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			r := rand.New(rand.NewPCG(1, 2))
			in := make([]Rect, n)
			for i := range in {
				in[i] = rct(pt(r.Float64()*1900, r.Float64()*1060), sz(8+r.Float64()*16, 8+r.Float64()*16))
			}
			b.ReportAllocs()
			for b.Loop() {
				Merge(in, window, 4, 6)
			}
		})
	}
}
