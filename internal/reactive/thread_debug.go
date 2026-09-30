//go:build ggui_debug

package reactive

import (
	"fmt"
	"sync/atomic"

	"github.com/ironpark/ggui/internal/goid"
)

// Signals, effects, layout and paint all belong to the UI goroutine, and a
// write from another one races the frame however carefully StateValue locks its
// own value: the dirty marks it leaves are not guarded at all, and the frame
// may already have laid out the tree that write should have changed. A
// goroutine hands its result back with App.Post instead.
//
// Nothing enforced that. This build does: it remembers the goroutine the
// frames run on and panics on a Set from any other, at the write rather than
// at whatever the race later corrupts. Build or test with -tags ggui_debug.

var uiGoroutine atomic.Int64

// MarkUIThread records the goroutine frames run on. Called at the top of
// every frame, so it follows a runtime that moves them.
func MarkUIThread() { uiGoroutine.Store(goid.ID()) }

// UnmarkUIThread disarms the check when the app closes, so a process that
// outlives its window, or a test that opens a second app, starts clean.
func UnmarkUIThread() { uiGoroutine.Store(0) }

// CheckUIThread panics when op is happening on a goroutine that is not the
// one running frames. Before the first frame there is nothing to compare
// with, and the writes a main function makes while building its model are
// on the goroutine that will run them.
func CheckUIThread(op string) {
	ui := uiGoroutine.Load()
	if ui == 0 || ui == goid.ID() {
		return
	}
	panic(fmt.Sprintf("ggui: %s from goroutine %d; frames run on %d. "+
		"Do the work off the UI goroutine and hand the result back with App.Post.",
		op, goid.ID(), ui))
}
