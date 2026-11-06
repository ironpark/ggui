package fontsys

import (
	"runtime"
	"testing"

	"github.com/ironpark/ggui/internal/third_party/text"
)

// cachedFaces is how many faces f holds.
func cachedFaces(f *Font) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.faces) + len(f.oldFaces)
}

// A text size animated through thousands of sizes, a frame each, leaves
// a font holding the faces of the last few frames, not of every size it
// ever drew at.
func TestAnimatedSizesKeepTheFaceCacheBounded(t *testing.T) {
	f := emojiChain(t)
	for i := range 100 { // what the first faces load once
		f.Face(100+float64(i), 400, nil)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	const n = 2000
	for i := range n {
		text.AdvanceTick()
		f.Face(12+float64(i)/100, 400, nil)
		f.Face(14, 400, nil) // a label beside it, at rest
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	t.Logf("%.0f bytes per animated size", float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/n)
	if got := cachedFaces(f); got > 2*faceCacheMin {
		t.Errorf("the font holds %d faces after %d sizes, want at most %d", got, n, 2*faceCacheMin)
	}
	// The face drawn every frame survived every turn of the cache.
	a := f.Face(14, 400, nil)
	text.AdvanceTick()
	for i := range 4 * faceCacheMin {
		text.AdvanceTick()
		f.Face(30+float64(i), 400, nil)
		if b := f.Face(14, 400, nil); b != a {
			t.Fatalf("the face in use every frame was made again after %d new sizes", i)
		}
	}
}

// Within one frame a font keeps every face asked for, however many: a
// frame that draws more sizes than the cache's floor makes none of them
// twice.
func TestAFrameKeepsEveryFaceItAsksFor(t *testing.T) {
	f := emojiChain(t)
	text.AdvanceTick()
	const n = 3 * faceCacheMin
	faces := make([]text.Face, n)
	for i := range n {
		faces[i] = f.Face(10+float64(i), 400, nil)
	}
	for i := range n {
		if f.Face(10+float64(i), 400, nil) != faces[i] {
			t.Fatalf("face %d of one frame was made again", i)
		}
	}
	// And frame after frame, the same set is found, not made again.
	for range 3 {
		text.AdvanceTick()
		for i := range n {
			if f.Face(10+float64(i), 400, nil) != faces[i] {
				t.Fatalf("face %d was made again a frame later", i)
			}
		}
	}
}

// A face asked for again, in the same frame or any later one, is found
// without allocating, over a working set the cache turns over.
func TestSteadyFaceLookupsDoNotAllocate(t *testing.T) {
	f := emojiChain(t)
	sizes := []float64{11, 12, 13, 14, 16, 18, 24, 32}
	for _, s := range sizes {
		f.Face(s, 400, nil)
	}
	allocs := testing.AllocsPerRun(100, func() {
		text.AdvanceTick()
		for _, s := range sizes {
			f.Face(s, 400, nil)
		}
	})
	if allocs != 0 {
		t.Errorf("a frame's face lookups allocate %v times, want 0", allocs)
	}
}
