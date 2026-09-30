package ggui

// Async runs work on a goroutine and hands what it returns to done on the
// UI thread, before the next frame of the window or probe that called it.
// It is the shape of every slow call a handler makes: a file dialog, a
// request, a query. done may write signals; work must not. If the window
// closes first, done never runs.
//
//	ui.Button("Open…", func() {
//		host := ggui.UseHost()
//		ggui.Async(func() (string, error) {
//			return host.Dialogs().OpenFile(runtime.FileDialog{})
//		}, func(path string, err error) {
//			if err == nil {
//				m.Path.Set(path)
//			}
//		})
//	})
//
// Resource is the form for a value that follows a signal and is cancelled
// when it changes; Async is for a one-off action.
func Async[T any](work func() (T, error), done func(T, error)) {
	post := UseHost().Post
	go func() {
		v, err := work()
		post(func() { done(v, err) })
	}()
}
