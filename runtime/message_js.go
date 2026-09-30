//go:build js

package runtime

import "syscall/js"

// showMessage uses the browser's own boxes: alert for one button, confirm
// for two, whose OK and Cancel stand for the first and second. A browser
// offers nothing with more buttons.
func showMessage(m Message, _ uintptr) (int, error) {
	buttons := m.buttons()
	text := messageText(m)
	switch len(buttons) {
	case 1:
		js.Global().Call("alert", text)
		return 0, nil
	case 2:
		if js.Global().Call("confirm", text).Bool() {
			return 0, nil
		}
		return 1, nil
	}
	return 0, ErrUnsupported
}
