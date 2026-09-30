package ggui

import (
	"time"

	"github.com/ironpark/ggui/internal/reactive"
)

// A world is what the frame loops of one reactive runtime share beside the
// graph itself: the animations their frames step, the clock those frames
// read, and the groups their input holds busy. It is kept on the runtime,
// so the App's windows share the App's world, each Probe has one of its
// own, and what is made outside every tree lands in the world of its
// goroutine's Base runtime. A probe's animations step in its frames alone
// and Advance moves its clock without moving anyone else's.
type world struct {
	anims animator
	frame frameClock
	busy  map[any]bool // see inputState.markBusyGroups

	rt      *reactive.Runtime
	env     *StateValue[Env] // see UseEnv; nil until first read
	envMemo envMemo
}

// worldOf returns rt's world, making it on first use. A runtime is used by
// one goroutine at a time, so making it needs no lock.
func worldOf(rt *reactive.Runtime) *world {
	if w, ok := rt.Host.(*world); ok {
		return w
	}
	w := &world{busy: map[any]bool{}, rt: rt}
	rt.Host = w
	return w
}

// loopWorld is l's world: its runtime's.
func loopWorld(l *frameLoop) *world {
	if l == nil {
		return worldOf(reactive.Base())
	}
	return worldOf(l.runtime())
}

// activeWorld is the world of the loop whose frame the running goroutine
// runs, or ran last: what Now, WakeAt and a painting widget reach. Before
// any frame on the goroutine, it is its Base runtime's.
func activeWorld() *world { return loopWorld(runningLoop()) }

// ownerWorld is the world of the tree being built now, for something that
// keeps its world from creation on, as an animation does: the current
// owner's loop's, else the active one.
func ownerWorld() *world {
	if l := loopOf(reactive.CurrentOwner()); l != nil {
		return loopWorld(l)
	}
	return activeWorld()
}

// raw is the time the frame clock advances from: the source a Probe's
// Advance installed, else the package clock.
func (f *frameClock) raw() time.Time {
	if f.source != nil {
		return f.source()
	}
	return clock()
}
