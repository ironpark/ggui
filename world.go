package ggui

import (
	"iter"

	"github.com/ironpark/ggui/internal/reactive"
)

// A world is what the frame loops of one reactive runtime share beside the
// graph itself: the animations their frames step, the clock those frames
// read, and the groups their input holds busy. It is kept on the runtime,
// so the App's windows share the App's world, each Probe has one of its
// own, and what is made outside every tree lands in the world of its
// goroutine's Base runtime. A probe's animations step in its frames alone
// and Advance moves its clock without moving anyone else's.
//
// A world also holds the settings SetClock, SetDefaultFont and
// SetEmojiFont make, and a test's IME. One it has not set is its parent's:
// the world of the Base its runtime was made beside. Set on the goroutine
// before any App or Probe runs, a setting reaches every one made there;
// set during their frames, it is theirs alone.
type world struct {
	parent *world // nil for a Base's own

	anims animator
	frame frameClock
	busy  map[any]bool // see inputState.markBusyGroups

	env     *StateValue[Env] // see UseEnv; nil until first read
	envMemo envMemo

	font        *Font                      // see SetDefaultFont; nil inherits
	mono        *Font                      // see SetDefaultMonoFont; nil inherits
	nativeFonts bool                       // an App's: defaults to the platform's fonts
	emoji       *emojiChoice               // see SetEmojiFont; nil inherits
	fontGen     uint64                     // counts this world's font and emoji settings
	ime         func(*TextInputWidget) ime // a test's IME; nil inherits
}

// worldOf returns rt's world, making it on first use. A runtime is used by
// one goroutine at a time, so making it needs no lock.
func worldOf(rt *reactive.Runtime) *world {
	if w, ok := rt.Host.(*world); ok {
		return w
	}
	w := &world{busy: map[any]bool{}}
	if p := rt.Parent(); p != nil {
		w.parent = worldOf(p)
		w.frame.parent = &w.parent.frame
	}
	rt.Host = w
	return w
}

// chain yields w and then its parents, nearest first: where a setting w
// has not made is looked up.
func (w *world) chain() iter.Seq[*world] {
	return func(yield func(*world) bool) {
		for ; w != nil && yield(w); w = w.parent {
		}
	}
}

// activeWorld is the world of the loop whose frame the running goroutine
// runs, or ran last: what Now, WakeAt and a painting widget reach. Before
// any frame on the goroutine, it is its Base runtime's.
func activeWorld() *world { return runningLoop().world() }

// ownerWorld is the world of the tree being built now, for something that
// keeps its world from creation on, as an animation does: the current
// owner's loop's, else the active one.
func ownerWorld() *world {
	if l := loopOf(reactive.CurrentOwner()); l != nil {
		return l.world()
	}
	return activeWorld()
}
