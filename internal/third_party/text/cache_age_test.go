package text

import "testing"

// An entry unused for maxAge ticks goes, however few the cache holds; one
// in use stays, as does one that cannot expire.
func TestACacheLetsGoOfWhatItHasNotUsedForLong(t *testing.T) {
	c := newCache[string, int](512)
	start := currentTick()
	// The clock is the package's: tests after this one count from where it was.
	t.Cleanup(func() { tick.Store(start) })
	c.getOrCreate("old", func() (int, bool) { return 1, true })
	c.getOrCreate("kept", func() (int, bool) { return 2, false })
	for range maxAge + sweepEvery {
		AdvanceTick()
		c.getOrCreate("used", func() (int, bool) { return 3, true })
	}
	c.getOrCreate("new", func() (int, bool) { return 4, true })
	if _, ok := c.values["old"]; ok {
		t.Fatalf("an entry unused for %d ticks is kept", currentTick()-start)
	}
	for _, k := range []string{"kept", "used", "new"} {
		if _, ok := c.values[k]; !ok {
			t.Fatalf("%q was let go", k)
		}
	}
}
