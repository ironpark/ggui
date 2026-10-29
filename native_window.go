package ggui

import (
	"github.com/ironpark/ggui/runtime"
	"github.com/ironpark/ggwd/native"
)

func (w *Window) nativeDialogs() runtime.Dialogs {
	return runtime.NativeDialogsForWindow(func() uintptr {
		if nw := w.window.Load(); nw != nil {
			return native.Handle(nw)
		}
		return 0
	})
}
