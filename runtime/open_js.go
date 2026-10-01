//go:build js

package runtime

import "syscall/js"

// OpenURL opens url in a new browser tab. A browser blocks the tab unless
// the call comes from a click or a key press.
func OpenURL(url string) error {
	if js.Global().Call("open", url, "_blank").IsNull() {
		return ErrCanceled
	}
	return nil
}

// Reveal is unsupported in a browser, which has no file manager to show.
func Reveal(path string) error { return ErrUnsupported }
