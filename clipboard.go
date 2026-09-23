package ggui

import "github.com/ironpark/ggui/runtime"

// CurrentClipboard is the clipboard of the App or Probe whose frame is
// running, for a widget that cuts, copies or pastes: text fields use it.
// Replace an app's with App.SetClipboard; a Probe has one of its own.
func CurrentClipboard() runtime.Clipboard { return currentClipboard() }

// systemClipboard serves a clipboard use with no frame loop running.
var systemClipboard = runtime.NativeClipboard()

func currentClipboard() runtime.Clipboard {
	if l := running.Load(); l != nil && l.clipboard != nil {
		return l.clipboard
	}
	return systemClipboard
}
