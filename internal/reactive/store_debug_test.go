//go:build ggui_debug

package reactive

import (
	"runtime"
	"sync"
	"testing"
)

// Frames on goroutines of their own check the stores at once, while the
// stores of some are let go: each keeps what the others found live, and
// none reads past what another left.
func TestCheckStoresOnGoroutinesAtOnce(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				for range 4 {
					NewStore(struct{}{})
				}
				runtime.GC()
				CheckStores()
			}
		})
	}
	wg.Wait()
}

func TestCheckStoresSkipsUnreadAndDirtySelects(t *testing.T) {
	store := NewStore(&storeModel{title: "a"})
	evals := 0
	sel := Select(store, func(m *storeModel) string {
		evals++
		return m.title
	})
	defer sel.Dispose()

	// Never evaluated: CheckStores must not eagerly evaluate it.
	CheckStores()
	if evals != 0 {
		t.Fatalf("CheckStores evaluated unread Select %d times, want 0", evals)
	}

	// Evaluate once, then publish an update so Select is stateDirty.
	if got := sel.Get(); got != "a" || evals != 1 {
		t.Fatalf("got=%q evals=%d, want a and 1", got, evals)
	}
	store.Update(func(m *storeModel) { m.title = "b" })
	CheckStores()
	if evals != 1 {
		t.Fatalf("CheckStores eagerly evaluated dirty Select: evals=%d, want 1", evals)
	}
}
