//go:build !darwin && !windows && !js

package runtime

import "github.com/ironpark/ggfx"

func nativeClipboard() Clipboard { return &x11Clipboard{} }

// x11Clipboard is the X11 CLIPBOARD selection, which ggfx's window system
// connection reads and owns. Before the app runs, what is copied is kept in
// memory.
type x11Clipboard struct {
	mem MemoryClipboard
}

func (c *x11Clipboard) Read() string {
	s, err := ggfx.ClipboardText()
	if err != nil {
		return c.mem.Read()
	}
	return s
}

func (c *x11Clipboard) Write(s string) {
	if ggfx.SetClipboardText(s) != nil {
		c.mem.Write(s)
	}
}
