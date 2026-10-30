package ggui

import "github.com/ironpark/ggwd"

// OnQuitRequest registers fn to run when the user asks the app to quit, as
// with ⌘Q or the Dock's Quit, before every window's OnCloseRequest handlers
// are asked: how an app whose window hides on its close button, to live on
// in the tray, tells a quit from that button. fn runs on the UI thread.
// Only macOS reports a quit.
func (a *App) OnQuitRequest(fn func()) { a.quitAsked = append(a.quitAsked, fn) }

// OnReopen registers fn to run when the user opens the running app again,
// as by clicking its Dock icon: the way back to a window it hid, which the
// system does not show on its own. fn runs on the UI thread. Only macOS
// reports it.
func (a *App) OnReopen(fn func()) { a.reopened = append(a.reopened, fn) }

// postAll runs fns on the UI thread, in the order of the work posted before
// them: a quit's handlers run before the close requests that follow it.
func (a *App) postAll(fns []func()) {
	w := a.activeWindow()
	if w == nil {
		return
	}
	for _, fn := range fns {
		w.post(fn)
	}
}

// SetDockIcon puts the app's icon in the Dock and its menu in the menu bar,
// or takes both out, as an app that lives in the tray does while it shows
// no window. Out of the Dock, the app is not in the ⌘-Tab switcher, and
// its windows have no menu bar: put it back before showing one. Call it
// while the app runs, from any goroutine. Only macOS has a Dock.
func SetDockIcon(visible bool) {
	if appRunning.Load() {
		ggwd.SetDockIcon(visible)
	}
}
