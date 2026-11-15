package gfxtest

import (
	"os"
	"testing"
	"time"

	"github.com/ironpark/ggfx/host"
)

// MainHeadless runs m with no renderer, as the tests of the widgets run: what they paint is never
// shown, and no frame begins. The renderer keeps a draw made outside a frame, a copy of its
// vertices or pixels with it, for the next frame to run, so a benchmark that paints a million
// times with none would hold a million draws. MainHeadless drops them as the tests run, which
// costs a benchmark nothing it measures: the draw is still made and still deferred.
func MainHeadless(m *testing.M) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(20 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				host.DiscardDeferred()
			case <-done:
				return
			}
		}
	}()
	code := m.Run()
	close(done)
	os.Exit(code)
}
