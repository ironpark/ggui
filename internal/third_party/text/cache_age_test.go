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

func TestCacheHardLRUCapAndOnEvict(t *testing.T) {
	c := newCache[int, int](16)
	start := currentTick()
	t.Cleanup(func() { tick.Store(start) })

	var evicted []int
	c.onEvict = func(v int) { evicted = append(evicted, v) }

	// Populate across multiple recent ticks (all within 60 ticks so the soft pass keeps them)
	// until past hardLimit().
	c.getOrCreate(-1, func() (int, bool) { return -1, false }) // non-expirable
	limit := c.hardLimit()
	for i := range limit + 10 {
		if i%8 == 0 {
			AdvanceTick()
		}
		v := i
		c.getOrCreate(v, func() (int, bool) { return v, true })
	}

	if len(c.values) > limit {
		t.Fatalf("cache exceeded hardLimit %d: got %d", limit, len(c.values))
	}
	if _, ok := c.values[-1]; !ok {
		t.Fatal("non-expirable entry was evicted")
	}
	if len(evicted) == 0 {
		t.Fatal("expected onEvict to be invoked during LRU eviction")
	}

	// Evicted entries must be older ones (e.g. key 0), while the newest must remain.
	if _, ok := c.values[0]; ok {
		t.Fatal("oldest entry 0 should have been evicted by LRU cap")
	}
	if _, ok := c.values[limit+9]; !ok {
		t.Fatal("newest entry should remain in cache")
	}
}

func TestCacheSweepsStaleEntriesOnHit(t *testing.T) {
	c := newCache[string, int](512)
	start := currentTick()
	t.Cleanup(func() { tick.Store(start) })

	evictedCount := 0
	c.onEvict = func(int) { evictedCount++ }

	c.getOrCreate("stale", func() (int, bool) { return 1, true })
	c.getOrCreate("hot", func() (int, bool) { return 2, true })

	// Advance past maxAge + sweepEvery with ONLY cache hits (no new keys inserted).
	for range maxAge + sweepEvery {
		AdvanceTick()
		c.getOrCreate("hot", func() (int, bool) { t.Fatal("unexpected miss"); return 0, true })
	}

	if _, ok := c.values["stale"]; ok {
		t.Fatal("stale entry was not swept during cache hits")
	}
	if evictedCount != 1 {
		t.Fatalf("evictedCount = %d, want 1", evictedCount)
	}
}

func TestCacheHardLRUSameTickSkipsRepeatedRescan(t *testing.T) {
	c := newCache[int, int](16)
	start := currentTick()
	t.Cleanup(func() { tick.Store(start) })

	limit := c.hardLimit()
	// Fill past hardLimit within a single tick so no entry is eligible (< n-1).
	for i := range limit + 20 {
		v := i
		c.getOrCreate(v, func() (int, bool) { return v, true })
	}
	if c.evictExhausted != currentTick() {
		t.Fatalf("evictExhausted = %d, want current tick %d", c.evictExhausted, currentTick())
	}

	// Subsequent misses on the same tick must not allocate candidate slices.
	nextKey := limit + 100
	allocs := testing.AllocsPerRun(50, func() {
		k := nextKey
		nextKey++
		c.getOrCreate(k, func() (int, bool) { return k, true })
	})
	// Only the cacheValue struct (and occasional map bucket growth) may allocate;
	// previously evictLRULocked allocated a full O(len(c.values)) slice every miss.
	if allocs > 3 {
		t.Fatalf("same-tick misses past hardLimit allocated %v times/op, want <= 3", allocs)
	}

	// Advancing two ticks makes prior entries eligible again and brings len <= softLimit.
	AdvanceTick()
	AdvanceTick()
	c.getOrCreate(-99, func() (int, bool) { return -99, true })
	if len(c.values) > c.softLimit {
		t.Fatalf("after advancing 2 ticks len(c.values) = %d, want <= %d", len(c.values), c.softLimit)
	}
}
