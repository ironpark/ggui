package ggui

// Host is what App and Probe have in common: the frame loop's surface for
// code that should run the same under a window and under a test. Write
// setup against it and hand either one in:
//
//	func shortcuts(h ggui.Host) {
//		h.Shortcut("cmd+s", save)
//	}
//	...
//	shortcuts(app) // in main
//	shortcuts(p)   // in a test, with p := ggui.ProbeBuilder(...)
//
// Setup is left out on purpose: it returns the concrete type for chaining.
type Host interface {
	// Post queues fn to run on the UI thread before the next frame's input.
	Post(fn func())
	// OnFrame registers fn to run once per frame, before input is dispatched.
	OnFrame(fn func())
	// OnKey registers a global key handler that sees every press first.
	OnKey(fn func(KeyEvent) bool)
	// Shortcut runs fn on the chord; see ParseChord for the names.
	Shortcut(chord string, fn func()) *ShortcutHandle
	// OnDrop registers a handler for files dropped onto the window that no
	// drop zone under the cursor took.
	OnDrop(fn func(DropEvent))
	// Perform carries out an accessibility action on a node.
	Perform(id NodeID, act Action)
	// Announce queues text for assistive technology to speak.
	Announce(text string, politeness Politeness)
	// Semantics returns the accessibility tree the last frame published.
	Semantics() *SemTree
	// Close disposes everything the host built.
	Close()
}

var (
	_ Host = (*App)(nil)
	_ Host = (*Probe)(nil)
)
