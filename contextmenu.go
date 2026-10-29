package ggui

import (
	"github.com/ironpark/ggui/internal/nativemenu"
	"github.com/ironpark/ggwd/native"
)

// NativeContextMenu reports whether the platform supports native context menus.
func NativeContextMenu() bool { return nativemenu.ContextSupported() }

// ShowContextMenu opens a native menu at a position in window content coordinates
// (logical points, measured from the top left). It waits until dismissal and runs
// the chosen action on the UI thread before returning. Call it on the UI thread.
// It returns false if the platform is unsupported, the window is not open, or
// there are no entries; dismissal without selecting an action still returns true.
func (w *Window) ShowContextMenu(at Point, items ...MenuItem) bool {
	if w == nil || w.Closed() || !NativeContextMenu() || len(items) == 0 {
		return false
	}
	nw := w.window.Load()
	if nw == nil {
		return false
	}
	owner := native.Handle(nw)
	if owner == 0 {
		return false
	}
	s := w.app.newMenuSet(items)
	defer w.app.release(s)
	id := nativemenu.Popup(owner, float64(at.X), float64(at.Y), s.native)
	if id != 0 {
		w.app.runActionIn(w, id)
	}
	return true
}
