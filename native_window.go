package ggui

import "github.com/ironpark/ggui/runtime"

func (a *App) nativeFilePicker() runtime.FilePicker {
	return runtime.NativeFilePickerForWindow(func() uintptr {
		if w := a.window.Load(); w != nil {
			return w.NativeHandle()
		}
		return 0
	})
}
