//go:build js && wasm

package devstate

import (
	"encoding/json"
	"syscall/js"
)

// Dispatch is the native channel's; in a browser the snapshot is taken in
// the page's call, while the app's goroutines are paused, so it is unused.
var Dispatch func(fn func()) bool

// connect takes what ggui dev's page hands over. The page is the channel
// here: it keeps the state in sessionStorage across the reload onto a new
// build, hands it over in globalThis.gguiDev.state, and calls
// globalThis.gguiDev.save, which this sets, just before that reload. A
// page that is not ggui dev's has no gguiDev, and the app keeps nothing.
func connect() *session {
	dev := js.Global().Get("gguiDev")
	if dev.Type() != js.TypeObject {
		return nil
	}
	var handed State
	if st := dev.Get("state"); st.Type() == js.TypeString {
		if err := json.Unmarshal([]byte(st.String()), &handed); err != nil {
			reportf("channel", "ggui: the state ggui dev's page handed over does not decode (%v); none is kept", err)
		}
	}
	s := newSession(handed)
	// JavaScript calls into Go only while the app's goroutines wait, so the
	// snapshot is taken between frames however it is reached.
	dev.Set("save", js.FuncOf(func(js.Value, []js.Value) any {
		b, err := json.Marshal(s.snapshot())
		if err != nil {
			return js.Null()
		}
		return string(b)
	}))
	return s
}
