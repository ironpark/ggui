package ggui

import "github.com/ironpark/ggui/internal/devstate"

// StateValue.Keep marks state that ggui dev keeps across a restart; package
// devstate is the channel it travels over. When ggui dev asks for it, the
// kept values are read on the UI thread, between frames, so that one is
// not read while a handler is halfway through changing it: the running
// App is what posts there. Before Run, and in a process with no App, the
// channel reads them itself.
func init() {
	devstate.Dispatch = func(fn func()) bool {
		a := runningApp.Load()
		if a == nil {
			return false
		}
		w := a.activeWindow()
		if w == nil {
			return false
		}
		w.post(fn)
		return true
	}
}
