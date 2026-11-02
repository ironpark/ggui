//go:build ggui_debug

package ggui

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
				checkStores()
			}
		})
	}
	wg.Wait()
}
