package ggui

import (
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/runtime"
)

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
	// Dialogs opens the file and message dialogs: the platform's under an
	// App, a runtime.StubFilePicker under a Probe.
	Dialogs() runtime.Dialogs
	// OnCloseRequest registers fn to decide whether the window closes when
	// the user asks to close it; see App.OnCloseRequest.
	OnCloseRequest(fn func() bool)
	// Clipboard holds the text cut, copy and paste move: the system's under
	// an App, a runtime.MemoryClipboard of the probe's own under a Probe.
	Clipboard() runtime.Clipboard
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
	_ Host = (*Window)(nil)
	_ Host = (*Probe)(nil)
)

// UseHost returns the Host whose tree is running: the Window a component
// was built in or a handler was called from, or the Probe under a test.
// It is how a widget reaches the dialogs, the clipboard or Post without
// the model carrying them:
//
//	ui.Button("Open…", func() {
//		path, err := ggui.UseHost().Dialogs().OpenFile(runtime.FileDialog{})
//		...
//	})
//
// A component captures it while it is built to use it later from a
// goroutine. It panics outside any window or probe.
func UseHost() Host {
	if l := loopOf(reactive.CurrentOwner()); l != nil {
		return l.host
	}
	if l := runningLoop(); l != nil {
		return l.host
	}
	panic("ggui: UseHost outside a window or probe")
}

// UseWindow returns the Window UseHost would, or nil under a Probe, which
// has no native window to move or retitle.
func UseWindow() *Window {
	w, _ := UseHost().(*Window)
	return w
}
