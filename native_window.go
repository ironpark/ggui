package ggui

import "github.com/ironpark/ggui/runtime"

func (w *Window) nativeDialogs() runtime.Dialogs {
	return runtime.NativeDialogsForWindow(func() uintptr {
		if nw := w.window.Load(); nw != nil {
			return nw.NativeHandle()
		}
		return 0
	})
}
