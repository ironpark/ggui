package ggui

import (
	"time"

	"github.com/ironpark/ggui/internal/reactive"
)

// A world is what the frame loops of one reactive runtime share beside the
// graph itself: the animations their frames step, the clock those frames
// read, and the groups their input holds busy. The App's windows share the
// process world, as they share reactive.Default; each Probe has one of its
// own, so its animations step in its frames alone and Advance moves its
// clock without moving anyone else's.
type world struct {
	anims animator
	frame frameClock
	busy  map[any]bool // see inputState.markBusyGroups
}

func newWorld() *world { return &world{busy: map[any]bool{}} }

// processWorld is the App's world, and the one anything made outside every
// frame loop's tree belongs to.
var processWorld = newWorld()

// loopWorld is l's world: a Probe's own, or the process world for a window.
func loopWorld(l *frameLoop) *world {
	if l == nil || l.world == nil {
		return processWorld
	}
	return l.world
}

// activeWorld is the world of the loop whose frame is running, or ran last:
// what Now, WakeAt and a painting widget reach.
func activeWorld() *world { return loopWorld(running.Load()) }

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
