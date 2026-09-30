package ggui

import "github.com/ironpark/ggui/runtime"

// SingleInstance makes the app the only one named id running for the
// user, where id is reverse-DNS by convention: "com.example.editor". Call
// it before Run. When another process already runs the app, that process
// is handed this launch's arguments and working directory, and
// SingleInstance returns runtime.ErrRunning: return from main without
// calling Run.
//
// In the process that runs, fn receives every later launch on the UI
// thread, where UseWindow is the window that last had the focus. It is
// where the app brings a window forward and opens the files it was given:
//
//	err := app.SingleInstance("com.example.editor", func(l runtime.Launch) {
//		app.Focus()
//		m.open(l.Args...)
//	})
//	if errors.Is(err, runtime.ErrRunning) {
//		return
//	}
func (a *App) SingleInstance(id string, fn func(runtime.Launch)) error {
	release, err := runtime.ClaimInstance(id, func(l runtime.Launch) {
		if w := a.activeWindow(); w != nil {
			w.post(func() { fn(l) })
		}
	})
	if err != nil {
		return err
	}
	a.releaseInstance = release
	return nil
}
