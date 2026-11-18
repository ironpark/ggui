//go:build ggui_debug

package reactive

import (
	"fmt"
	"sync/atomic"

	"github.com/ironpark/ggui/internal/reactive/goid"
)

// Signals, effects, layout and paint all belong to the UI goroutine, and a
// write from another one races the frame however carefully StateValue locks its
// own value: the dirty marks it leaves are not guarded at all, and the frame
// may already have laid out the tree that write should have changed. A
// goroutine hands its result back with App.Post instead.
//
// Nothing enforced that. This build does, at the access rather than at
// whatever the race later corrupts. Build or test with -tags ggui_debug.
//
// There is no one UI goroutine to compare with: probes under t.Parallel
// run frames on a goroutine each, beside one another and beside an App. So
// the check goes by runtime. Every frame of an App's window, a Probe or a
// WindowProbe marks its runtime, and the Base it was made beside, with the
// goroutine running the frame (MarkUIThread). A signal remembers the
// runtime it was made in: its owner's, or with no owner its goroutine's
// Base, where a test's or main's model made before the first frame lands.
// An access panics only when that runtime is marked and the goroutine
// making it is not the one marked. So
//
//   - a probe's frames never compare with another probe's, nor an App's,
//     as their runtimes, and their goroutines' Bases, differ;
//   - what a goroutine makes before its first frame is checked from that
//     frame on, against the goroutine the frames run on, which is the one
//     that made it unless frames moved;
//   - a worker's goroutine runs no frames, so what it reaches of a world is
//     checked, while what it makes for itself, in its own Base, is not;
//   - work a worker posts runs in the frame's goroutine and passes.
//
// What is checked without a signal (Effect, Resource, Debounced, UIThread
// and the like) goes by the running goroutine's owner, or its Base when it
// has none.

// runtimeThread is the goroutine running a runtime's frames, or 0 while
// none does.
type runtimeThread struct{ ui atomic.Int64 }

// valueThread is the runtime a signal was made in.
type valueThread struct{ rt *Runtime }

// madeHere is the valueThread of a signal made now: in the current owner's
// runtime, else the goroutine's Base.
func madeHere() valueThread { return valueThread{current().runtime()} }

// runtime is the runtime what the goroutine makes now belongs to.
func (s *scope) runtime() *Runtime {
	if s.owner != nil && s.owner.rt != nil {
		return s.owner.rt
	}
	return s.baseRuntime()
}

// checkThread panics when op is happening on a goroutine that is not the
// one running the frames of the runtime the signal was made in.
func (v *valueThread) checkThread(op string) { checkRuntime(op, v.rt) }

// MarkUIThread records that the running goroutine runs rt's frames, and so
// the frames of the Base rt was made beside. Called at the start of every
// frame, so it follows a runtime that moves them.
func MarkUIThread(rt *Runtime) {
	id := goid.ID()
	rt.ui.Store(id)
	if rt.base != nil {
		rt.base.ui.Store(id)
	}
}

// UnmarkUIThread disarms rt's check when the loop running it closes, and
// its Base's if the same goroutine marked it, so that a test may go on
// using what it made with no frames left to race.
func UnmarkUIThread(rt *Runtime) {
	id := rt.ui.Swap(0)
	if rt.base != nil && id != 0 {
		rt.base.ui.CompareAndSwap(id, 0)
	}
}

// CheckUIThread panics when op is happening on a goroutine that is not the
// one running the frames of the current owner's runtime, or of the
// goroutine's Base when there is no owner. Before the first frame there is
// nothing to compare with.
func CheckUIThread(op string) {
	sc := current()
	rt := sc.base
	if sc.owner != nil && sc.owner.rt != nil {
		rt = sc.owner.rt
	}
	checkRuntime(op, rt)
}

func checkRuntime(op string, rt *Runtime) {
	if rt == nil {
		return
	}
	ui := rt.ui.Load()
	if ui == 0 {
		return
	}
	if me := goid.ID(); me != ui {
		panic(fmt.Sprintf("ggui: %s from goroutine %d; frames run on %d. "+
			"Do the work off the UI goroutine and hand the result back with App.Post.",
			op, me, ui))
	}
}
